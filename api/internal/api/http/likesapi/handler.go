package likesapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/AlekSi/pointer"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/service/deck"
	"flatmatch/api/internal/service/zones"
)

const maxBodyBytes = 64 << 10

type Likes interface {
	Like(ctx context.Context, from entity.User, toID uuid.UUID, zoneID string, pointID *uuid.UUID) (entity.LikeResult, error)
	Incoming(ctx context.Context, userID uuid.UUID) ([]likesrepo.IncomingLike, error)
	Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error)
}

type Photos interface {
	PhotoURLs(u entity.User) []string
}

type Points interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.Point, error)
}

type Pairs interface {
	RankIncoming(ctx context.Context, viewerID uuid.UUID,
		list []likesrepo.IncomingLike) ([]deck.RankedLike, error)
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	likes  Likes
	photos Photos
	points Points
	pairs  Pairs
	dir    ZoneDir
}

var (
	errNoUserInContext = errors.New("no authenticated user in request context")
	errSelfLike        = errors.New("a like to oneself")
)

func New(likes Likes, photos Photos, points Points, pairs Pairs, dir ZoneDir) *Handler {
	return &Handler{likes: likes, photos: photos, points: points, pairs: pairs, dir: dir}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.create)
	r.Get("/budget", h.budget)
	r.Get("/incoming", h.incoming)
	return r
}

func (h *Handler) budget(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	budget, err := h.likes.Budget(r.Context(), user.ID)
	if err != nil {
		h.handleError(w, r, user.ID, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toBudget(budget))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	var body likeRequest
	if !decode(w, r, &body) {
		return
	}
	toID, err := uuid.Parse(body.ToID)
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "to_id is not a uuid", []string{"to_id"}, err)
		return
	}

	if toID == user.ID {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "cannot like yourself", []string{"to_id"}, errSelfLike)
		return
	}

	var pointID *uuid.UUID
	if body.PointID != "" {
		id, err := uuid.Parse(body.PointID)
		if err != nil {
			httpx.BadRequest(w, r, httpx.CodeInvalidField, "point_id is not a uuid", []string{"point_id"}, err)
			return
		}
		pointID = &id
	}

	result, err := h.likes.Like(r.Context(), user, toID, body.ZoneID, pointID)
	if err != nil {
		h.handleError(w, r, user.ID, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toLikeResponse(result))
}

func (h *Handler) incoming(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	incoming, err := h.likes.Incoming(r.Context(), user.ID)
	if err != nil {
		h.handleError(w, r, user.ID, err)
		return
	}
	ranked, err := h.pairs.RankIncoming(r.Context(), user.ID, incoming)
	if err != nil {
		h.handleError(w, r, user.ID, err)
		return
	}

	out := make([]incomingDTO, 0, len(ranked))
	for _, item := range ranked {
		point, roomZone, err := h.point(r.Context(), item.Like.PointID)
		if err != nil {
			h.handleError(w, r, user.ID, err)
			return
		}
		out = append(out, toIncoming(item.Like, h.photos.PhotoURLs(item.Like.From),
			h.zone(item.Like.ZoneID), point, toPairDTO(item.Pair, point, roomZone)))
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) point(ctx context.Context, id *uuid.UUID) (*pointDTO, *zoneDTO, error) {
	if id == nil {
		return nil, nil, nil
	}
	p, err := h.points.ByID(ctx, *id)
	switch {
	case errors.Is(err, entity.ErrNotFound):
		return nil, nil, nil
	case err != nil:
		return nil, nil, err
	}
	station, _ := h.dir.Get(p.ZoneID)
	return &pointDTO{ID: p.ID, Kind: string(p.Type), Label: pointLabel(p.Type, station.Name)},
		h.zone(p.ZoneID), nil
}

func (h *Handler) zone(id string) *zoneDTO {
	zone, ok := h.dir.Get(id)
	if !ok {
		return nil
	}
	return pointer.To(toZoneDTO(zone))
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
