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

type People interface {
	VisibleByID(ctx context.Context, viewerID, id uuid.UUID) (entity.User, error)
}

type Likes interface {
	Between(ctx context.Context, a, b uuid.UUID) (aLikedB, bLikedA bool, err error)
	Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error)
}

type Photos interface {
	PhotoURL(u entity.User) string
}

type Searches interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
	HasActiveIn(ctx context.Context, userID uuid.UUID, zoneIDs []string) (bool, error)
}

type Points interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.Point, error)
	InPool(ctx context.Context, userID, pointID uuid.UUID) (bool, error)
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
	points   Points
	dir      ZoneDir
}

var errNoUserInContext = errors.New("no authenticated user in request context")

func New(people People, likes Likes, photos Photos, searches Searches, points Points, dir ZoneDir) *Handler {
	return &Handler{
		people: people, likes: likes, photos: photos,
		searches: searches, points: points, dir: dir,
	}
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

	point, metAtPoint, err := h.contextPoint(r.Context(), viewer.ID, target, r.URL.Query().Get("point"))
	if err != nil {
		handleError(w, r, err)
		return
	}

	if !myLike && !likedMe && !metAtPoint {
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
		h.photos.PhotoURL(target), h.contextZone(r.URL.Query().Get("zone")), point,
		myLike, likedMe, budget))
}

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

func (h *Handler) contextPoint(
	ctx context.Context, viewerID uuid.UUID, target entity.User, raw string,
) (*pointDTO, bool, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, false, nil
	}
	point, err := h.points.ByID(ctx, id)
	switch {
	case errors.Is(err, entity.ErrNotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}
	if point.Status != entity.PointActive {
		return nil, false, nil
	}

	switch point.Type {
	case entity.PointIntentHome:
		if point.CreatedBy == nil || *point.CreatedBy != target.ID {
			return nil, false, nil
		}
		return pointer.To(h.toPointDTO(point, target.Name)), true, nil
	case entity.PointFlat:
		mine, err := h.points.InPool(ctx, viewerID, point.ID)
		if err != nil {
			return nil, false, err
		}
		theirs, err := h.points.InPool(ctx, target.ID, point.ID)
		if err != nil {
			return nil, false, err
		}
		if !mine || !theirs {
			return nil, false, nil
		}
		return pointer.To(h.toPointDTO(point, "")), true, nil
	default:
		return nil, false, nil
	}
}

func (h *Handler) toPointDTO(p entity.Point, author string) pointDTO {
	station, _ := h.dir.Get(p.ZoneID)
	return pointDTO{ID: p.ID, Kind: string(p.Type), Label: pointLabel(p.Type, author, station.Name)}
}

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
