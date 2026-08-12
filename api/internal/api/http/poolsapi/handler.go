// Package poolsapi serves the two screens a zone pool has: the feed of a set
// of filters (§6 — выдача с промо-блоком людей) and the pool itself, with its
// three sections and verdict chips (§2.1).
//
// Neither route saves anything: looking at a feed is not a subscription (§3),
// the search behind the promo block is created through searchesapi.
package poolsapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

type Pools interface {
	Counts(ctx context.Context, viewerID uuid.UUID, params entity.SearchParams, newSince *time.Time) (pools.Counts, error)
	ZonePool(ctx context.Context, viewer entity.User, zoneID string) (pools.Pool, error)
}

// Searches tells the promo block how many slots are taken and whether this
// zone is already one of them.
type Searches interface {
	List(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	pools    Pools
	searches Searches
	dir      ZoneDir
}

var (
	errNoUserInContext = errors.New("no authenticated user in request context")
	errBadFilters      = errors.New("feed filters are invalid")
)

func New(pools Pools, searches Searches, dir ZoneDir) *Handler {
	return &Handler{pools: pools, searches: searches, dir: dir}
}

// Feed answers the third step of the search-first feed (§6): the filters come
// in the query string and stay there — the search is saved only by the button
// inside the promo block, which posts to searchesapi.
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	// Filters are validated before anything is counted: Compare against empty
	// parameters calls the whole zone a full match, so a missing price_max
	// would quietly inflate every counter instead of failing.
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

	// The counters belong to the zone the promo block names — the one asked
	// for, not the one some saved search of the viewer happens to cover.
	counts, err := h.pools.Counts(r.Context(), user.ID, params, nil)
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
		Points: []any{},
	})
}

// ZonePool serves the pool screen. The whole viewer goes down, not just their
// id: «N совпадений анкеты» is computed against their questionnaire.
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

// parseParams reads the look-without-saving filters of the feed. Only zone_id
// and price_max are required; the rest is «неважно» when absent (§2.1). The
// returned names are the parameters that could not be read at all — semantic
// checks are entity's job.
func parseParams(q url.Values) (entity.SearchParams, []string) {
	var bad []string

	p := entity.SearchParams{ZoneID: q.Get("zone_id")}
	p.PriceMin = optInt(q, "price_min", &bad)
	if v := optInt(q, "price_max", &bad); v != nil {
		p.PriceMax = *v
	}
	p.RoomsMin = optInt(q, "rooms_min", &bad)
	p.RoomsMax = optInt(q, "rooms_max", &bad)

	if raw := q.Get("move_in"); raw != "" {
		month, err := time.Parse(moveInMonth, raw)
		if err != nil {
			bad = append(bad, "move_in")
		} else {
			p.MoveIn = &month
		}
	}
	if raw := q.Get("term"); raw != "" {
		// Parsed, not validated: an unknown value becomes the sentinel, which
		// entity.InvalidFields reports by name.
		p.Term = pointer.To(entity.ParseTerm(raw))
	}
	return p, bad
}

// slots reads the two subscription facts the promo block needs: how many of
// the three slots are taken, and whether this very zone is already one of them
// — an intersecting zone is not, the button must still offer to subscribe.
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
		// Пул без пересекающего поиска не показываем (§2.3 «мы увиделись»);
		// тизер зоны живёт в промо-блоке ленты, он не гейтится.
		httpx.BadRequest(w, r, httpx.CodeNeedSearch,
			"Сохрани поиск в этой зоне — пул откроется", nil, err)
	default:
		httpx.Internal(w, r, err)
	}
}

// optInt reads an optional integer parameter, naming one that is present but
// not a number.
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

// nonNil keeps a JSON array an array: the front-end iterates the avatar stack
// without checking it for null first.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
