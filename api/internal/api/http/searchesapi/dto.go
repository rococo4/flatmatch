package searchesapi

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/searches"
	"flatmatch/api/internal/service/zones"
)

const moveInMonth = "2006-01"

type paramsRequest struct {
	ZoneID   string   `json:"zone_id"`
	PriceMin *int     `json:"price_min"`
	PriceMax int      `json:"price_max"`
	RoomsMin *int     `json:"rooms_min"`
	RoomsMax *int     `json:"rooms_max"`
	MoveIn   *string  `json:"move_in"`
	Term     *string  `json:"term"`
	HasStuff []string `json:"has_stuff"`
}

type closeRequest struct {
	Reason string `json:"reason"`
}

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
	MoveIn      *string   `json:"move_in,omitempty"`
	Term        *string   `json:"term,omitempty"`
	HasStuff    []string  `json:"has_stuff"`
	Status      string    `json:"status"`
	CloseReason *string   `json:"close_reason,omitempty"`
	CreatedAt   time.Time `json:"created_at"`

	People    int `json:"people"`
	FullMatch int `json:"full_match"`
	NewCount  int `json:"new_count"`

	PointsCount int `json:"points_count"`
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

func (b paramsRequest) toParams() (entity.SearchParams, error) {
	p := entity.SearchParams{
		ZoneID:   b.ZoneID,
		PriceMin: b.PriceMin,
		PriceMax: b.PriceMax,
		RoomsMin: b.RoomsMin,
		RoomsMax: b.RoomsMax,
		HasStuff: make([]entity.StuffChip, 0, len(b.HasStuff)),
	}

	if b.MoveIn != nil && *b.MoveIn != "" {
		month, err := time.Parse(moveInMonth, *b.MoveIn)
		if err != nil {
			return entity.SearchParams{},
				&entity.FieldsError{Err: searches.ErrInvalidParams, Fields: []string{"move_in"}}
		}
		p.MoveIn = &month
	}

	if b.Term != nil && *b.Term != "" {
		p.Term = pointer.To(entity.ParseTerm(*b.Term))
	}
	for _, raw := range b.HasStuff {
		p.HasStuff = append(p.HasStuff, entity.ParseStuffChip(raw))
	}
	return p, nil
}
