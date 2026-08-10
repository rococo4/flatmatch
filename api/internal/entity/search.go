package entity

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

const MaxActiveSearches = 3

const PriceMaxCap = 1_000_000

type Term string

const (
	TermUnknown Term = unknown

	TermHalfYear Term = "half_year"
	TermYear     Term = "year"
	TermOpen     Term = "open"
)

type SearchStatus string

const (
	SearchStatusUnknown SearchStatus = unknown

	SearchActive SearchStatus = "active"
	SearchPaused SearchStatus = "paused"
	SearchClosed SearchStatus = "closed"
)

type CloseReason string

const (
	CloseReasonUnknown CloseReason = unknown

	CloseFoundHere      CloseReason = "found_here" // ГЛАВНАЯ МЕТРИКА (§3)
	CloseFoundElsewhere CloseReason = "found_elsewhere"
	CloseChangedMind    CloseReason = "changed_mind"
)

type StuffChip string

const (
	StuffChipUnknown StuffChip = unknown

	StuffFurniture  StuffChip = "furniture"
	StuffAppliances StuffChip = "appliances"
)

type SearchParams struct {
	ZoneID   string
	PriceMin *int
	PriceMax int
	RoomsMin *int
	RoomsMax *int
	MoveIn   *time.Time // first day of the move-in month
	Term     *Term
	HasStuff []StuffChip
}

type Search struct {
	ID     uuid.UUID
	UserID uuid.UUID
	SearchParams
	Status      SearchStatus
	CloseReason CloseReason // "" unless Status is closed
	PoolSeenAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type PoolCandidate struct {
	User   User
	Search Search
}

func ParseTerm(raw string) Term {
	return parseEnum(raw, TermHalfYear, TermYear, TermOpen)
}

func ParseSearchStatus(raw string) SearchStatus {
	return parseEnum(raw, SearchActive, SearchPaused, SearchClosed)
}

func ParseCloseReason(raw string) CloseReason {
	return parseEnum(raw, CloseFoundHere, CloseFoundElsewhere, CloseChangedMind)
}

func ParseStuffChip(raw string) StuffChip {
	return parseEnum(raw, StuffFurniture, StuffAppliances)
}

func (p SearchParams) InvalidFields() []string {
	checks := []struct {
		field string
		bad   bool
	}{
		{"zone_id", p.ZoneID == ""},
		{"price_max", p.PriceMax <= 0 || p.PriceMax > PriceMaxCap},
		{"price_min", p.PriceMin != nil && (*p.PriceMin < 0 || *p.PriceMin > p.PriceMax)},
		{"rooms_min", p.RoomsMin != nil &&
			(*p.RoomsMin < 1 || (p.RoomsMax != nil && *p.RoomsMin > *p.RoomsMax))},
		{"rooms_max", p.RoomsMax != nil && *p.RoomsMax < 1},
		{"term", p.Term != nil && *p.Term == TermUnknown},
		{"has_stuff", slices.ContainsFunc(p.HasStuff, func(c StuffChip) bool {
			return c == "" || c == StuffChipUnknown || ParseStuffChip(string(c)) != c
		})},
	}

	var bad []string
	for _, c := range checks {
		if c.bad {
			bad = append(bad, c.field)
		}
	}
	return bad
}
