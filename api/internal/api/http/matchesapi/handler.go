package matchesapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/AlekSi/pointer"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/service/matches"
	"flatmatch/api/internal/service/zones"
)

type Matches interface {
	List(ctx context.Context, userID uuid.UUID) ([]matchesrepo.MatchWithPeer, error)
	Detail(ctx context.Context, userID, id uuid.UUID) (matchesrepo.MatchWithPeer, matches.Summary, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
}

type Photos interface {
	PhotoURL(u entity.User) string
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	matches Matches
	photos  Photos
	dir     ZoneDir
}

var errNoUserInContext = errors.New("no authenticated user in request context")

func New(matches Matches, photos Photos, dir ZoneDir) *Handler {
	return &Handler{matches: matches, photos: photos, dir: dir}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Get("/{id}", h.detail)
	r.Delete("/{id}", h.remove)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	all, err := h.matches.List(r.Context(), user.ID)
	if err != nil {
		handleError(w, r, err)
		return
	}

	out := make([]matchDTO, 0, len(all))
	for _, m := range all {
		out = append(out, toMatchDTO(m, h.photos.PhotoURL(m.Peer), h.zone(m.Match.ContextZone)))
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "match id is not a uuid", []string{"id"}, err)
		return
	}

	match, summary, err := h.matches.Detail(r.Context(), user.ID, id)
	if err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toMatchDetail(match, summary, h.photos.PhotoURL(match.Peer)))
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "match id is not a uuid", []string{"id"}, err)
		return
	}

	if err := h.matches.Delete(r.Context(), user.ID, id); err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusNoContent, nil)
}

func (h *Handler) zone(id string) *zoneDTO {
	zone, ok := h.dir.Get(id)
	if !ok {
		return nil
	}
	return pointer.To(toZoneDTO(zone))
}

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, entity.ErrNotFound) {
		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such match", err)
		return
	}
	httpx.Internal(w, r, err)
}
