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
	CreateIntentHome(ctx context.Context, userID uuid.UUID, in points.CreateIntentHome) (entity.Point, error)
	SavePhoto(ctx context.Context, file io.ReadSeeker) (string, error)
	Point(ctx context.Context, viewerID, pointID uuid.UUID) (pointsrepo.FeedPoint, error)
	Mine(ctx context.Context, viewerID uuid.UUID) (created, entered []pointsrepo.FeedPoint, err error)
	Close(ctx context.Context, userID, pointID uuid.UUID) error
}

type Passes interface {
	PassPoint(ctx context.Context, viewerID, pointID uuid.UUID) error
}

type Photos interface {
	URL(key string) string
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	points Points
	passes Passes
	photos Photos
	dir    ZoneDir
}

var errNoUserInContext = errors.New("no authenticated user in request context")

func New(points Points, passes Passes, photos Photos, dir ZoneDir) *Handler {
	return &Handler{points: points, passes: passes, photos: photos, dir: dir}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.create)

	r.Post("/photo", h.uploadPhoto)

	r.Get("/mine", h.mine)
	r.Get("/{id}", h.card)
	r.Post("/{id}/close", h.close)
	r.Post("/{id}/pass", h.pass)
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
	if kind != entity.PointIntentHome {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "point type is not open for creation",
			[]string{"type"}, errors.New("point type "+body.Type+" is not open for creation"))
		return
	}

	in, err := body.toIntentHome()
	if err != nil {
		handleFormError(w, r, kind, err)
		return
	}
	created, err := h.points.CreateIntentHome(r.Context(), user.ID, in)
	if err != nil {
		handleFormError(w, r, kind, err)
		return
	}

	httpx.JSON(w, r, http.StatusCreated, h.toCard(pointsrepo.FeedPoint{Point: created}, user.ID))
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

func (h *Handler) close(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	if err := h.points.Close(r.Context(), user.ID, id); err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusNoContent, nil)
}

func (h *Handler) pass(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	if err := h.passes.PassPoint(r.Context(), user.ID, id); err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusNoContent, nil)
}

func (h *Handler) toCard(fp pointsrepo.FeedPoint, viewerID uuid.UUID) Card {
	zone, _ := h.dir.Get(fp.Point.ZoneID)
	return toCard(fp, h.photos, toZoneDTO(zone), viewerID)
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
