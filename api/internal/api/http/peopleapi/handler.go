// Package peopleapi serves the profile screen of another person (§6): the only
// place a dear like is placed from, and the only screen that has to say what
// the two have already done to each other.
package peopleapi

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
	"flatmatch/api/internal/service/zones"
)

// People is the single visibility rule of §4: a person hidden from this viewer
// comes back as ErrNotFound, the same as one who never existed.
type People interface {
	VisibleByID(ctx context.Context, viewerID, id uuid.UUID) (entity.User, error)
}

// Likes answers the two halves of the like state a profile shows: what the
// pair has already done to each other and how many slots the viewer has left.
type Likes interface {
	Between(ctx context.Context, a, b uuid.UUID) (aLikedB, bLikedA bool, err error)
	Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error)
}

// Photos is the §5 rule about an unapproved photo, and it lives in one place —
// the users service — because every screen has to hide the same thing.
type Photos interface {
	PhotoURL(u entity.User) string
}

// Searches answers the pool half of «мы увиделись» (§2.3): whether the pair's
// active searches intersect. The viewer side arrives as a zone list, the
// target side is asked with it.
type Searches interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
	HasActiveIn(ctx context.Context, userID uuid.UUID, zoneIDs []string) (bool, error)
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
	Intersecting(id string) []string
}

type Handler struct {
	people   People
	likes    Likes
	photos   Photos
	searches Searches
	dir      ZoneDir
}

var errNoUserInContext = errors.New("no authenticated user in request context")

func New(people People, likes Likes, photos Photos, searches Searches, dir ZoneDir) *Handler {
	return &Handler{people: people, likes: likes, photos: photos, searches: searches, dir: dir}
}

func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	viewer, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "user id is not a uuid", []string{"id"}, err)
		return
	}

	// The visibility gate goes first and everything else waits for its answer:
	// Between has no gate of its own, so asking it about an invisible person
	// would leak «он тебя лайкнул» about someone who, for this viewer, does not
	// exist at all (§4).
	target, err := h.people.VisibleByID(r.Context(), viewer.ID, id)
	if err != nil {
		handleError(w, r, err)
		return
	}

	myLike, likedMe, err := h.likes.Between(r.Context(), viewer.ID, target.ID)
	if err != nil {
		handleError(w, r, err)
		return
	}

	// Отношение «мы увиделись» (§2.3): без пары лайков профиль открыт только
	// со-пульнику — активные поиски двоих должны пересекаться. Чужой человек
	// из другого конца города получает тот же 404, что и невидимый.
	if !myLike && !likedMe {
		met, err := h.metInPool(r.Context(), viewer.ID, target.ID)
		if err != nil {
			handleError(w, r, err)
			return
		}
		if !met {
			handleError(w, r, entity.ErrNotFound)
			return
		}
	}

	budget, err := h.likes.Budget(r.Context(), viewer.ID)
	if err != nil {
		handleError(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, toProfile(viewer, target,
		h.photos.PhotoURL(target), h.contextZone(r.URL.Query().Get("zone")),
		myLike, likedMe, budget))
}

// metInPool: активные поиски двоих пересекаются зонами — цель состоит в пуле
// одного из поисков смотрящего. Пересечение считает справочник: объединяем
// Intersecting по зонам смотрящего и спрашиваем у базы, есть ли у цели
// активный поиск в этих зонах.
func (h *Handler) metInPool(ctx context.Context, viewerID, targetID uuid.UUID) (bool, error) {
	mine, err := h.searches.ListByUser(ctx, viewerID)
	if err != nil {
		return false, err
	}
	seen := make(map[string]bool)
	var union []string
	for _, s := range mine {
		if s.Status != entity.SearchActive {
			continue
		}
		for _, z := range h.dir.Intersecting(s.ZoneID) {
			if !seen[z] {
				seen[z] = true
				union = append(union, z)
			}
		}
	}
	if len(union) == 0 {
		return false, nil
	}
	return h.searches.HasActiveIn(ctx, targetID, union)
}

// contextZone names the pool the profile was opened from. An id the directory
// no longer knows is dropped rather than refused: a like keeps its context
// forever (§2.3), so profiles do open from zones we have since retired, and
// that is no reason to refuse the screen.
func (h *Handler) contextZone(id string) *zoneDTO {
	zone, ok := h.dir.Get(id)
	if !ok {
		return nil
	}
	return pointer.To(toZoneDTO(zone))
}

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, entity.ErrNotFound) {
		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such person", err)
		return
	}
	httpx.Internal(w, r, err)
}
