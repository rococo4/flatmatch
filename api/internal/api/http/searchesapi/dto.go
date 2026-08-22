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
	Rooms    []int    `json:"rooms"`
	Terms    []string `json:"terms"`
	MoveIn   []string `json:"move_in"`
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
	Rooms       []int     `json:"rooms"`
	Terms       []string  `json:"terms"`
	MoveIn      []string  `json:"move_in"`
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
		Rooms:     append(make([]int, 0, len(s.Rooms)), s.Rooms...),
		Terms:     make([]string, 0, len(s.Terms)),
		MoveIn:    make([]string, 0, len(s.MoveInMonths)),
		HasStuff:  make([]string, 0, len(s.HasStuff)),
		Status:    string(s.Status),
		CreatedAt: s.CreatedAt,
	}
	for _, term := range s.Terms {
		out.Terms = append(out.Terms, string(term))
	}
	for _, month := range s.MoveInMonths {
		out.MoveIn = append(out.MoveIn, month.Format(moveInMonth))
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
		ZoneID:       b.ZoneID,
		PriceMin:     b.PriceMin,
		PriceMax:     b.PriceMax,
		Rooms:        append(make([]int, 0, len(b.Rooms)), b.Rooms...),
		Terms:        make([]entity.Term, 0, len(b.Terms)),
		MoveInMonths: make([]time.Time, 0, len(b.MoveIn)),
		HasStuff:     make([]entity.StuffChip, 0, len(b.HasStuff)),
	}

	for _, raw := range b.Terms {
		p.Terms = append(p.Terms, entity.ParseTerm(raw))
	}
	for _, raw := range b.MoveIn {
		month, err := time.Parse(moveInMonth, raw)
		if err != nil {
			return entity.SearchParams{},
				&entity.FieldsError{Err: searches.ErrInvalidParams, Fields: []string{"move_in"}}
		}
		p.MoveInMonths = append(p.MoveInMonths, month)
	}
	for _, raw := range b.HasStuff {
		p.HasStuff = append(p.HasStuff, entity.ParseStuffChip(raw))
	}
	return p.Normalized(), nil
}
