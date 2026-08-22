package poolsapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/api/http/pointsapi"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

type Pools interface {
	Counts(ctx context.Context, viewerID uuid.UUID, params entity.SearchParams, newSince *time.Time) (pools.Counts, error)
	ZonePool(ctx context.Context, viewer entity.User, zoneID string) (pools.Pool, error)
}

type Searches interface {
	List(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
}

type Points interface {
	Feed(ctx context.Context, viewerID uuid.UUID, zoneID string) ([]pointsrepo.FeedPoint, error)
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	pools    Pools
	searches Searches
	points   Points
	photos   pointsapi.Photos
	dir      ZoneDir
}

var (
	errNoUserInContext = errors.New("no authenticated user in request context")
	errBadFilters      = errors.New("feed filters are invalid")
)

func New(pools Pools, searches Searches, points Points, photos pointsapi.Photos, dir ZoneDir) *Handler {
	return &Handler{pools: pools, searches: searches, points: points, photos: photos, dir: dir}
}

func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	params, bad := parseParams(r.URL.Query())
	zone, known := h.dir.Get(params.ZoneID)
	if !known {
		bad = append(bad, "zone_id")
	}
	bad = append(bad, params.InvalidFields()...)
	if len(bad) > 0 {
		slices.Sort(bad)
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "value outside the allowed set",
			slices.Compact(bad), errBadFilters)
		return
	}

	mine, err := h.searches.List(r.Context(), user.ID)
	if err != nil {
		handleError(w, r, err)
		return
	}
	used, mySearchID := slots(mine, params.ZoneID)

	counts, err := h.pools.Counts(r.Context(), user.ID, params, nil)
	if err != nil {
		handleError(w, r, err)
		return
	}

	points, err := h.points.Feed(r.Context(), user.ID, params.ZoneID)
	if err != nil {
		handleError(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, feedResponse{
		Promo: promoDTO{
			Zone:       toZoneDTO(zone),
			People:     counts.People,
			FullMatch:  counts.FullMatch,
			Avatars:    nonNil(counts.Avatars),
			MySearchID: mySearchID,
			SlotsUsed:  used,
			SlotsTotal: entity.MaxActiveSearches,
		},
		Points: pointsapi.Cards(points, h.photos, h.dir, user.ID),
	})
}

func (h *Handler) ZonePool(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	pool, err := h.pools.ZonePool(r.Context(), user, chi.URLParam(r, "zoneID"))
	if err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toPoolResponse(pool))
}

func parseParams(q url.Values) (entity.SearchParams, []string) {
	var bad []string

	p := entity.SearchParams{ZoneID: q.Get("zone_id")}
	p.PriceMin = optInt(q, "price_min", &bad)
	if v := optInt(q, "price_max", &bad); v != nil {
		p.PriceMax = *v
	}
	for _, raw := range queryList(q, "rooms") {
		n, err := strconv.Atoi(raw)
		if err != nil {
			bad = append(bad, "rooms")
			continue
		}
		p.Rooms = append(p.Rooms, n)
	}
	for _, raw := range queryList(q, "terms") {
		p.Terms = append(p.Terms, entity.ParseTerm(raw))
	}
	for _, raw := range queryList(q, "move_in") {
		month, err := time.Parse(moveInMonth, raw)
		if err != nil {
			bad = append(bad, "move_in")
			continue
		}
		p.MoveInMonths = append(p.MoveInMonths, month)
	}
	return p.Normalized(), bad
}

func slots(mine []entity.Search, zoneID string) (used int, mySearchID *uuid.UUID) {
	for i, s := range mine {
		if s.Status != entity.SearchActive {
			continue
		}
		used++
		if s.ZoneID == zoneID && mySearchID == nil {
			mySearchID = &mine[i].ID
		}
	}
	return used, mySearchID
}

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, entity.ErrNotFound):
		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such zone", err)
	case errors.Is(err, entity.ErrNeedSearch):

		httpx.BadRequest(w, r, httpx.CodeNeedSearch,
			"Сохрани поиск в этой зоне — пул откроется", nil, err)
	default:
		httpx.Internal(w, r, err)
	}
}

func queryList(q url.Values, name string) []string {
	var out []string
	for _, raw := range q[name] {
		for _, part := range strings.Split(raw, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func optInt(q url.Values, name string, bad *[]string) *int {
	raw := q.Get(name)
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		*bad = append(*bad, name)
		return nil
	}
	return &v
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
