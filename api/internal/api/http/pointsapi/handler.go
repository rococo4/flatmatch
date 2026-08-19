package pointsapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/imagetype"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/points"
	"flatmatch/api/internal/service/zones"
)

const maxBodyBytes = 64 << 10

const maxPhotoBytes = 5 << 20

type Points interface {
	CreateFlat(ctx context.Context, userID uuid.UUID, in points.CreateFlat) (entity.Point, error)
	CreateIntentHome(ctx context.Context, userID uuid.UUID, in points.CreateIntentHome) (entity.Point, error)
	SavePhoto(ctx context.Context, file io.ReadSeeker) (string, error)
	Point(ctx context.Context, viewerID, pointID uuid.UUID) (pointsrepo.FeedPoint, error)
	Mine(ctx context.Context, viewerID uuid.UUID) (created, entered []pointsrepo.FeedPoint, err error)
	Enter(ctx context.Context, userID, pointID uuid.UUID) error
	Leave(ctx context.Context, userID, pointID uuid.UUID) error
	Close(ctx context.Context, userID, pointID uuid.UUID) error
	Vote(ctx context.Context, viewerID, pointID uuid.UUID, alive bool) (pointsrepo.FeedPoint, error)
	Restore(ctx context.Context, userID, pointID uuid.UUID) (pointsrepo.FeedPoint, error)
	Pool(ctx context.Context, viewer entity.User, pointID uuid.UUID) (points.PoolView, error)
}

type Photos interface {
	URL(key string) string
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	points Points
	photos Photos
	dir    ZoneDir
}

var errNoUserInContext = errors.New("no authenticated user in request context")

var errNoAliveField = errors.New("vote body carries no alive field")

func New(points Points, photos Photos, dir ZoneDir) *Handler {
	return &Handler{points: points, photos: photos, dir: dir}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.create)

	r.Post("/photo", h.uploadPhoto)

	r.Get("/mine", h.mine)
	r.Get("/{id}", h.card)
	r.Post("/{id}/enter", h.enter)
	r.Delete("/{id}/enter", h.leave)
	r.Post("/{id}/close", h.close)
	r.Post("/{id}/vote", h.vote)
	r.Post("/{id}/restore", h.restore)
	r.Get("/{id}/pool", h.pool)
	return r
}

func Cards(list []pointsrepo.FeedPoint, photos Photos, dir ZoneDir, viewerID uuid.UUID) []Card {
	out := make([]Card, 0, len(list))
	for _, fp := range list {
		zone, _ := dir.Get(fp.Point.ZoneID)
		out = append(out, toCard(fp, photos, toZoneDTO(zone), viewerID))
	}
	return out
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	var body createRequest
	if !decode(w, r, &body) {
		return
	}

	kind := entity.ParsePointType(body.Type)
	var created entity.Point
	var err error
	switch kind {
	case entity.PointFlat:
		created, err = h.points.CreateFlat(r.Context(), user.ID, body.toFlat())
	case entity.PointIntentHome:
		var in points.CreateIntentHome
		if in, err = body.toIntentHome(); err == nil {
			created, err = h.points.CreateIntentHome(r.Context(), user.ID, in)
		}
	default:
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "unknown point type",
			[]string{"type"}, errors.New("unknown point type "+body.Type))
		return
	}
	if err != nil {
		handleFormError(w, r, kind, err)
		return
	}

	card := pointsrepo.FeedPoint{Point: created, InPool: created.Type == entity.PointFlat}
	httpx.JSON(w, r, http.StatusCreated, h.toCard(card, user.ID))
}

func (h *Handler) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+64<<10)

	file, _, err := r.FormFile("photo")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.BadRequest(w, r, httpx.CodeTooLarge, "photo is larger than 5 MB", nil, err)
			return
		}
		httpx.BadRequest(w, r, httpx.CodeBadRequest, "no file in the photo field", nil, err)
		return
	}
	defer file.Close()

	key, err := h.points.SavePhoto(r.Context(), file)
	if err != nil {
		if errors.Is(err, imagetype.ErrUnsupported) {
			httpx.BadRequest(w, r, httpx.CodeUnsupportedMedia, "unsupported image type", nil, err)
			return
		}
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, photoResponse{PhotoKey: key})
}

func (h *Handler) card(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	card, err := h.points.Point(r.Context(), user.ID, id)
	if err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.toCard(card, user.ID))
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	created, entered, err := h.points.Mine(r.Context(), user.ID)
	if err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, mineResponse{
		Created: Cards(created, h.photos, h.dir, user.ID),
		Entered: Cards(entered, h.photos, h.dir, user.ID),
	})
}

func (h *Handler) enter(w http.ResponseWriter, r *http.Request) {
	h.membership(w, r, h.points.Enter)
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	h.membership(w, r, h.points.Leave)
}

func (h *Handler) close(w http.ResponseWriter, r *http.Request) {
	h.membership(w, r, h.points.Close)
}

func (h *Handler) vote(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	var body voteRequest
	if !decode(w, r, &body) {
		return
	}
	if body.Alive == nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "vote carries no alive field",
			[]string{"alive"}, errNoAliveField)
		return
	}

	card, err := h.points.Vote(r.Context(), user.ID, id, *body.Alive)
	if err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.toCard(card, user.ID))
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	card, err := h.points.Restore(r.Context(), user.ID, id)
	if err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.toCard(card, user.ID))
}

func (h *Handler) pool(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	view, err := h.points.Pool(r.Context(), user, id)
	if err != nil {
		handleError(w, r, err)
		return
	}

	members := make([]memberDTO, 0, len(view.Members))
	for _, m := range view.Members {
		members = append(members, toMember(m, h.photos, h.searchZone(m)))
	}
	httpx.JSON(w, r, http.StatusOK, poolResponse{Members: members})
}

func (h *Handler) membership(w http.ResponseWriter, r *http.Request, act func(context.Context, uuid.UUID, uuid.UUID) error) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	if err := act(r.Context(), user.ID, id); err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusNoContent, nil)
}

func (h *Handler) toCard(fp pointsrepo.FeedPoint, viewerID uuid.UUID) Card {
	zone, _ := h.dir.Get(fp.Point.ZoneID)
	return toCard(fp, h.photos, toZoneDTO(zone), viewerID)
}

func (h *Handler) searchZone(m points.MemberView) *zoneDTO {
	if m.Search == nil {
		return nil
	}
	zone, ok := h.dir.Get(m.Search.ZoneID)
	if !ok {
		return nil
	}
	dto := toZoneDTO(zone)
	return &dto
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

func pointID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "point id is not a uuid", []string{"id"}, err)
		return uuid.Nil, false
	}
	return id, true
}
