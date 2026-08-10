package searchesapi

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/searches"
	"flatmatch/api/internal/service/zones"
)

// moveInMonth is the wire format of a move-in date. Only the month is a
// product concept (§2.1 — «месяц заезда»), so the day never travels.
const moveInMonth = "2006-01"

type paramsRequest struct {
	ZoneID   string   `json:"zone_id"`
	PriceMin *int     `json:"price_min"`
	PriceMax int      `json:"price_max"`
	RoomsMin *int     `json:"rooms_min"`
	RoomsMax *int     `json:"rooms_max"`
	MoveIn   *string  `json:"move_in"` // «2026-09»
	Term     *string  `json:"term"`
	HasStuff []string `json:"has_stuff"`
}

type closeRequest struct {
	Reason string `json:"reason"`
}

// zoneDTO carries the declensions the front-end needs to write «в Хамовниках»
// or «пул Хамовников» without a morphology library of its own.
type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

type searchResponse struct {
	ID          uuid.UUID `json:"id"`
	Zone        zoneDTO   `json:"zone"`
	PriceMin    *int      `json:"price_min,omitempty"`
	PriceMax    int       `json:"price_max"`
	RoomsMin    *int      `json:"rooms_min,omitempty"`
	RoomsMax    *int      `json:"rooms_max,omitempty"`
	MoveIn      *string   `json:"move_in,omitempty"` // «2026-09»
	Term        *string   `json:"term,omitempty"`
	HasStuff    []string  `json:"has_stuff"`
	Status      string    `json:"status"`
	CloseReason *string   `json:"close_reason,omitempty"`
	CreatedAt   time.Time `json:"created_at"`

	// Pool counters are filled in by the list handler only. In the answers to
	// mutations the zeroes do not lie, they are simply not counted: the
	// front-end refetches the list after every mutation.
	People      int `json:"people"`
	FullMatch   int `json:"full_match"`
	NewCount    int `json:"new_count"`
	PointsCount int `json:"points_count"` // always 0 until §12 step 5
}

func toSearchResponse(s entity.Search, zone zoneDTO) searchResponse {
	out := searchResponse{
		ID:        s.ID,
		Zone:      zone,
		PriceMin:  s.PriceMin,
		PriceMax:  s.PriceMax,
		RoomsMin:  s.RoomsMin,
		RoomsMax:  s.RoomsMax,
		HasStuff:  make([]string, 0, len(s.HasStuff)),
		Status:    string(s.Status),
		CreatedAt: s.CreatedAt,
	}
	if s.MoveIn != nil {
		out.MoveIn = pointer.To(s.MoveIn.Format(moveInMonth))
	}
	if s.Term != nil {
		out.Term = pointer.To(string(*s.Term))
	}
	for _, chip := range s.HasStuff {
		out.HasStuff = append(out.HasStuff, string(chip))
	}
	if s.CloseReason != "" {
		out.CloseReason = pointer.To(string(s.CloseReason))
	}
	return out
}

func toZoneDTO(z zones.Zone) zoneDTO {
	return zoneDTO{ID: z.ID, Name: z.Name, NameIn: z.NameIn, NameGen: z.NameGen}
}

// toParams turns the request into domain parameters. Enum values are parsed,
// not validated: an unrecognised one becomes the package's sentinel, which
// entity.InvalidFields then reports by name. Only the date has to be checked
// here — a string that is not a month cannot be represented at all.
func (b paramsRequest) toParams() (entity.SearchParams, error) {
	p := entity.SearchParams{
		ZoneID:   b.ZoneID,
		PriceMin: b.PriceMin,
		PriceMax: b.PriceMax,
		RoomsMin: b.RoomsMin,
		RoomsMax: b.RoomsMax,
		HasStuff: make([]entity.StuffChip, 0, len(b.HasStuff)),
	}

	// An empty string means «неважно», the same as an absent field: the promo
	// block posts the feed's filters back verbatim, and there `move_in=` is
	// exactly how «месяц не выбран» travels. Rejecting it here would turn the
	// one button that saves a search into a 400. A string that is present and
	// malformed («2026-13», «sept») is still an error.
	if b.MoveIn != nil && *b.MoveIn != "" {
		month, err := time.Parse(moveInMonth, *b.MoveIn)
		if err != nil {
			return entity.SearchParams{},
				&entity.FieldsError{Err: searches.ErrInvalidParams, Fields: []string{"move_in"}}
		}
		p.MoveIn = &month
	}
	// Same rule for the term, and here it is not only about symmetry: an empty
	// string parses to an empty Term, which InvalidFields does not call unknown
	// and the CHECK on searches.term then rejects — a 500 for a filter the user
	// simply did not fill in.
	if b.Term != nil && *b.Term != "" {
		p.Term = pointer.To(entity.ParseTerm(*b.Term))
	}
	for _, raw := range b.HasStuff {
		p.HasStuff = append(p.HasStuff, entity.ParseStuffChip(raw))
	}
	return p, nil
}
