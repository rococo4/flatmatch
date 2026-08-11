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
	Like(ctx context.Context, fromID, toID uuid.UUID, zoneID string) (entity.LikeResult, error)
	Incoming(ctx context.Context, userID uuid.UUID) ([]likesrepo.IncomingLike, error)
	Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error)
}

type Photos interface {
	URL(key string) string
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	likes  Likes
	photos Photos
	dir    ZoneDir
}

var (
	errNoUserInContext = errors.New("no authenticated user in request context")
	errSelfLike        = errors.New("a like to oneself")
)

func New(likes Likes, photos Photos, dir ZoneDir) *Handler {
	return &Handler{likes: likes, photos: photos, dir: dir}
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
	// The table's CHECK (from_id <> to_id) would turn this into a 500; naming
	// the field costs one comparison.
	if toID == user.ID {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "cannot like yourself", []string{"to_id"}, errSelfLike)
		return
	}

	result, err := h.likes.Like(r.Context(), user.ID, toID, body.ZoneID)
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
		out = append(out, toIncoming(like, h.photoURL(like.From), h.zone(like.ZoneID)))
	}
	httpx.JSON(w, r, http.StatusOK, out)
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

// photoURL hides a photo moderation has not approved (§5).
func (h *Handler) photoURL(u entity.User) string {
	if !u.PhotoApproved() {
		return ""
	}
	return h.photos.URL(u.PhotoKey)
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
