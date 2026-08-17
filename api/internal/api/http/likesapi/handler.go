// Package likesapi serves the dear like (§2.3) — «Хочу жить вместе» — and the
// «Тебя лайкнули» list it lands in.
//
// The gates behind POST / are the transaction's, not this handler's: approval,
// visibility, the budget and the match are all decided in one place, and the
// handler only turns their sentinels into the codes of §3.
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
	"flatmatch/api/internal/service/zones"
)

const maxBodyBytes = 64 << 10

type Likes interface {
	Like(ctx context.Context, from entity.User, toID uuid.UUID, zoneID string, pointID *uuid.UUID) (entity.LikeResult, error)
	Incoming(ctx context.Context, userID uuid.UUID) ([]likesrepo.IncomingLike, error)
	Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error)
}

// Photos is the §5 rule about an unapproved photo, and it lives in one place —
// the users service — because every screen has to hide the same thing.
type Photos interface {
	PhotoURL(u entity.User) string
}

// Points names the point half of a like's context. Статус не проверяется:
// лайк не сгорает ни от чего (§2.3), и его контекст переживает и закрытие
// точки — карточка «Тебя лайкнули» обязана остаться читаемой.
type Points interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.Point, error)
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	likes  Likes
	photos Photos
	points Points
	dir    ZoneDir
}

var (
	errNoUserInContext = errors.New("no authenticated user in request context")
	errSelfLike        = errors.New("a like to oneself")
)

func New(likes Likes, photos Photos, points Points, dir ZoneDir) *Handler {
	return &Handler{likes: likes, photos: photos, points: points, dir: dir}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.create)
	r.Get("/incoming", h.incoming)
	return r
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
	// Without this the answer would be 404: visible_people never returns the
	// viewer himself. 400 invalid_field труднее спутать с «нет такого
	// человека» — сам себе это ошибка запроса, а не отсутствие адресата.
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

	// Два контекста разом — ошибка запроса, и называет её сервис: там же, где
	// живёт правило «лайк несёт один контекст» (§2.3).
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

	likes, err := h.likes.Incoming(r.Context(), user.ID)
	if err != nil {
		h.handleError(w, r, user.ID, err)
		return
	}

	out := make([]incomingDTO, 0, len(likes))
	for _, like := range likes {
		point, err := h.point(r.Context(), like.PointID)
		if err != nil {
			h.handleError(w, r, user.ID, err)
			return
		}
		out = append(out, toIncoming(like, h.photos.PhotoURL(like.From), h.zone(like.ZoneID), point))
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// point names the point a like came from. Точка, которой больше нет, отнимает
// у карточки мини-карту контекста, а не саму карточку: лайк переживает всё
// (§2.3).
func (h *Handler) point(ctx context.Context, id *uuid.UUID) (*pointDTO, error) {
	if id == nil {
		return nil, nil
	}
	p, err := h.points.ByID(ctx, *id)
	switch {
	case errors.Is(err, entity.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	}
	station, _ := h.dir.Get(p.ZoneID)
	return &pointDTO{ID: p.ID, Kind: string(p.Type), Label: pointLabel(p.Type, station.Name)}, nil
}

// zone names the pool a like came from. An id the directory no longer knows is
// dropped: the like itself never expires (§2.3), so its context may well
// outlive the zone, and the card simply loses its mini-card of context.
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
