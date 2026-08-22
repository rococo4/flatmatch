package poolsapi

import (
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/pointsapi"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

const moveInMonth = "2006-01"

type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

type promoDTO struct {
	Zone       zoneDTO    `json:"zone"`
	People     int        `json:"people"`
	FullMatch  int        `json:"full_match"`
	Avatars    []string   `json:"avatars"`
	MySearchID *uuid.UUID `json:"my_search_id,omitempty"`
	SlotsUsed  int        `json:"slots_used"`
	SlotsTotal int        `json:"slots_total"`
}

type feedResponse struct {
	Promo  promoDTO         `json:"promo"`
	Points []pointsapi.Card `json:"points"`
}

type chipDTO struct {
	Param string `json:"param"`

	Verdict string `json:"verdict"`
}

type paramsDTO struct {
	PriceMin *int     `json:"price_min,omitempty"`
	PriceMax int      `json:"price_max"`
	Rooms    []int    `json:"rooms"`
	Terms    []string `json:"terms"`
	MoveIn   []string `json:"move_in"`
	HasStuff []string `json:"has_stuff,omitempty"`
}

type personDTO struct {
	UserID        uuid.UUID `json:"user_id"`
	Name          string    `json:"name"`
	Age           int       `json:"age"`
	PhotoURL      string    `json:"photo_url"`
	Schedule      string    `json:"schedule"`
	Smoking       string    `json:"smoking"`
	PetsOwn       string    `json:"pets_own"`
	SharedAnswers int       `json:"shared_answers"`
	Chips         []chipDTO `json:"chips,omitempty"`
	Params        paramsDTO `json:"params"`

	Zone zoneDTO `json:"zone"`

	ZoneMatch bool `json:"zone_match"`
}

type sectionDTO struct {
	ID     string      `json:"id"`
	People []personDTO `json:"people"`
}

type poolResponse struct {
	Zone zoneDTO `json:"zone"`

	MySearchID *uuid.UUID `json:"my_search_id,omitempty"`
	Ranked     bool       `json:"ranked"`

	Sections []sectionDTO `json:"sections"`
	People   []personDTO  `json:"people"`
}

func toPoolResponse(p pools.Pool) poolResponse {
	out := poolResponse{
		Zone:     toZoneDTO(p.Zone),
		Ranked:   p.Ranked,
		Sections: make([]sectionDTO, 0, len(p.Sections)),
		People:   toPeople(p.People),
	}
	if p.MySearch != nil {
		out.MySearchID = &p.MySearch.ID
	}
	for _, s := range p.Sections {
		out.Sections = append(out.Sections, sectionDTO{
			ID:     string(s.ID),
			People: toPeople(s.People),
		})
	}
	return out
}

func toZoneDTO(z zones.Zone) zoneDTO {
	return zoneDTO{ID: z.ID, Name: z.Name, NameIn: z.NameIn, NameGen: z.NameGen}
}

func toPeople(people []pools.Person) []personDTO {
	out := make([]personDTO, 0, len(people))
	for _, p := range people {
		out = append(out, toPerson(p))
	}
	return out
}

func toPerson(p pools.Person) personDTO {
	out := personDTO{
		UserID:        p.User.ID,
		Name:          p.User.Name,
		Age:           p.User.Profile.Age,
		PhotoURL:      p.PhotoURL,
		Schedule:      string(p.User.Profile.Schedule),
		Smoking:       string(p.User.Profile.Smoking),
		PetsOwn:       string(p.User.Profile.PetsOwn),
		SharedAnswers: p.SharedAnswers,
		Params:        toParamsDTO(p.Params),
		Zone:          toZoneDTO(p.Zone),
		ZoneMatch:     p.ZoneMatch,
	}
	for _, c := range p.Chips {
		out.Chips = append(out.Chips, chipDTO{Param: string(c.Param), Verdict: string(c.Verdict)})
	}
	return out
}

func toParamsDTO(p entity.SearchParams) paramsDTO {
	out := paramsDTO{
		PriceMin: p.PriceMin,
		PriceMax: p.PriceMax,
		Rooms:    append(make([]int, 0, len(p.Rooms)), p.Rooms...),
		Terms:    make([]string, 0, len(p.Terms)),
		MoveIn:   make([]string, 0, len(p.MoveInMonths)),
	}
	for _, term := range p.Terms {
		out.Terms = append(out.Terms, string(term))
	}
	for _, month := range p.MoveInMonths {
		out.MoveIn = append(out.MoveIn, month.Format(moveInMonth))
	}
	for _, chip := range p.HasStuff {
		out.HasStuff = append(out.HasStuff, string(chip))
	}
	return out
}
