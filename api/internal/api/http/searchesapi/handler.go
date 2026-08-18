package searchesapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

const maxBodyBytes = 64 << 10

type Searches interface {
	Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error)
	List(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
	Update(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error)
	Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error)
	Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
}

type Pools interface {
	Counts(ctx context.Context, viewerID uuid.UUID, params entity.SearchParams, newSince *time.Time) (pools.Counts, error)
}

type Points interface {
	CountIn(ctx context.Context, zoneID string) (int, error)
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	searches Searches
	pools    Pools
	points   Points
	dir      ZoneDir
}

var errNoUserInContext = errors.New("no authenticated user in request context")

func New(searches Searches, pools Pools, points Points, dir ZoneDir) *Handler {
	return &Handler{searches: searches, pools: pools, points: points, dir: dir}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Patch("/{id}", h.update)
	r.Post("/{id}/close", h.close)
	r.Post("/{id}/reactivate", h.reactivate)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	all, err := h.searches.List(r.Context(), user.ID)
	if err != nil {
		handleError(w, r, err)
		return
	}

	out := make([]searchResponse, 0, len(all))
	for _, s := range all {
		item := toSearchResponse(s, h.zone(s.ZoneID))

		if s.Status == entity.SearchActive {
			counts, err := h.pools.Counts(r.Context(), user.ID, s.SearchParams, &s.PoolSeenAt)
			switch {
			case errors.Is(err, entity.ErrNotFound):

			case err != nil:
				handleError(w, r, err)
				return
			default:
				item.People = counts.People
				item.FullMatch = counts.FullMatch
				item.NewCount = counts.NewCount
			}

			points, err := h.points.CountIn(r.Context(), s.ZoneID)
			if err != nil {
				handleError(w, r, err)
				return
			}
			item.PointsCount = points
		}
		out = append(out, item)
	}

	httpx.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	params, ok := h.readParams(w, r)
	if !ok {
		return
	}

	created, err := h.searches.Create(r.Context(), user.ID, params)
	if err != nil {
		handleError(w, r, err)
		return
	}
	h.respond(w, r, http.StatusCreated, created)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	id, ok := searchID(w, r)
	if !ok {
		return
	}
	params, ok := h.readParams(w, r)
	if !ok {
		return
	}

	updated, err := h.searches.Update(r.Context(), user.ID, id, params)
	if err != nil {
		handleError(w, r, err)
		return
	}
	h.respond(w, r, http.StatusOK, updated)
}

func (h *Handler) close(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	id, ok := searchID(w, r)
	if !ok {
		return
	}

	var body closeRequest
	if !decode(w, r, &body) {
		return
	}

	closed, err := h.searches.Close(r.Context(), user.ID, id, entity.ParseCloseReason(body.Reason))
	if err != nil {
		handleError(w, r, err)
		return
	}
	h.respond(w, r, http.StatusOK, closed)
}

func (h *Handler) reactivate(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	id, ok := searchID(w, r)
	if !ok {
		return
	}

	revived, err := h.searches.Reactivate(r.Context(), user.ID, id)
	if err != nil {
		handleError(w, r, err)
		return
	}
	h.respond(w, r, http.StatusOK, revived)
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, status int, s entity.Search) {
	httpx.JSON(w, r, status, toSearchResponse(s, h.zone(s.ZoneID)))
}

func (h *Handler) readParams(w http.ResponseWriter, r *http.Request) (entity.SearchParams, bool) {
	var body paramsRequest
	if !decode(w, r, &body) {
		return entity.SearchParams{}, false
	}

	params, err := body.toParams()
	if err != nil {
		handleError(w, r, err)
		return entity.SearchParams{}, false
	}
	return params, true
}

func (h *Handler) zone(id string) zoneDTO {
	z, ok := h.dir.Get(id)
	if !ok {
		return zoneDTO{ID: id, Name: id, NameIn: id, NameGen: id}
	}
	return toZoneDTO(z)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))

	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.BadRequest(w, r, httpx.CodeBadRequest, "cannot read the request body", nil, err)
		return false
	}
	return true
}

func searchID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "search id is not a uuid", []string{"id"}, err)
		return uuid.Nil, false
	}
	return id, true
}
