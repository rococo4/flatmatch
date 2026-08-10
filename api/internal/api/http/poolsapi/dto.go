package poolsapi

import (
	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

// moveInMonth is the wire format of a move-in date, the same one searchesapi
// reads: only the month is a product concept (§2.1).
const moveInMonth = "2006-01"

// zoneDTO is the outbound twin of searchesapi's: same fields, same
// declensions. Copied rather than shared because a DTO belongs to the handler
// that serves it — the two may diverge the day one of them grows a field.
type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

// promoDTO backs the pink block of the feed (§6): «N человек ищут в {зоне}»,
// «с K полное совпадение», the avatar stack and the state of the button —
// which is decided by my_search_id (already subscribed here) and slots_used
// against slots_total (all three slots taken).
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
	Promo  promoDTO `json:"promo"`
	Points []any    `json:"points"` // always [] until §12 step 5
}

type chipDTO struct {
	Param string `json:"param"`
	// Verdict is the backend's own four-value verdict: green / edge / grey /
	// none. «Не указана» (none) is deliberately not merged into grey — the
	// card labels the two differently.
	Verdict string `json:"verdict"`
}

// paramsDTO is the outbound twin of searchesapi's paramsRequest: same fields
// and encodings (move_in «2026-09»), zone omitted — the pool's zone is on top.
type paramsDTO struct {
	PriceMin *int     `json:"price_min,omitempty"`
	PriceMax int      `json:"price_max"`
	RoomsMin *int     `json:"rooms_min,omitempty"`
	RoomsMax *int     `json:"rooms_max,omitempty"`
	MoveIn   *string  `json:"move_in,omitempty"`
	Term     *string  `json:"term,omitempty"`
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
	Chips         []chipDTO `json:"chips,omitempty"` // ranked pools only
	Params        paramsDTO `json:"params"`
	// Zone of the person's search: «ищет на красной ветке» in the card.
	Zone zoneDTO `json:"zone"`
	// ZoneMatch: полная территориальная совместимость с поиском смотрящего.
	ZoneMatch bool `json:"zone_match"`
}

type sectionDTO struct {
	ID     string      `json:"id"` // all | partial | none
	People []personDTO `json:"people"`
}

type poolResponse struct {
	Zone zoneDTO `json:"zone"`
	// MySearchID is the subscription to this very zone, what the «Ты в пуле
	// {зоны} · поиск сохранён» banner needs. Ranking is happy with a search of
	// an intersecting zone, so ranked may be true while this is absent.
	MySearchID *uuid.UUID `json:"my_search_id,omitempty"`
	Ranked     bool       `json:"ranked"`
	// Both lists are always present and always arrays; `ranked` says which one
	// carries the pool — sections when true, people when false, the other one
	// empty. No omitempty: it drops an empty slice as readily as a nil one, so
	// an empty pool would arrive without the key the front-end iterates, and
	// «пул пуст» would have to be told apart from «поле забыли».
	Sections []sectionDTO `json:"sections"` // ranked
	People   []personDTO  `json:"people"`   // unranked
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

// toPerson serialises the mini-card. PhotoURL comes from the pool, never from
// the user's PhotoKey: the pool hides a photo moderation has not approved
// (§5), and Person.User is a partial record that must not be asked anything
// else.
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
		RoomsMin: p.RoomsMin,
		RoomsMax: p.RoomsMax,
	}
	if p.MoveIn != nil {
		out.MoveIn = pointer.To(p.MoveIn.Format(moveInMonth))
	}
	if p.Term != nil {
		out.Term = pointer.To(string(*p.Term))
	}
	for _, chip := range p.HasStuff {
		out.HasStuff = append(out.HasStuff, string(chip))
	}
	return out
}
