package deckapi

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/deck"
	"flatmatch/api/internal/service/zones"
)

const moveInDate = "2006-01-02"

const moveInMonth = "2006-01"

const (
	whoPeople = "people"
	whoRooms  = "rooms"
)

const (
	petsNone   = "none"
	petsOKMine = "ok_mine"
)

const (
	smokingNo          = "no"
	smokingOutsideOnly = "outside_only"
)

const (
	occupationStudy   = "study"
	occupationWorking = "working"
	occupationNone    = "none"
)

type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

type chipDTO struct {
	Param   string `json:"param"`
	Verdict string `json:"verdict"`
}

type paramsDTO struct {
	PriceMin *int     `json:"price_min,omitempty"`
	PriceMax int      `json:"price_max"`
	Rooms    []int    `json:"rooms"`
	Terms    []string `json:"terms"`
	MoveIn   []string `json:"move_in"`
}

type pairDTO struct {
	Zone      zoneDTO    `json:"zone"`
	ZoneMatch bool       `json:"zone_match"`
	Section   string     `json:"section"`
	Chips     []chipDTO  `json:"chips,omitempty"`
	Params    *paramsDTO `json:"params,omitempty"`
}

type personDTO struct {
	UserID     uuid.UUID `json:"user_id"`
	Name       string    `json:"name"`
	Age        int       `json:"age"`
	Occupation string    `json:"occupation"`
	LastSeen   time.Time `json:"last_seen"`
	PhotoURLs  []string  `json:"photo_urls"`
	Schedule   string    `json:"schedule"`
	Smoking    string    `json:"smoking"`
	PetsOwn    string    `json:"pets_own"`
	PetsOK     string    `json:"pets_ok"`
	Noise      string    `json:"noise"`
	Guests     string    `json:"guests"`
	Cleaning   string    `json:"cleaning"`
	Alcohol    string    `json:"alcohol"`
	About      string    `json:"about"`
}

type authorDTO struct {
	UserID     uuid.UUID `json:"user_id"`
	Name       string    `json:"name"`
	Age        int       `json:"age"`
	Occupation string    `json:"occupation"`
	PhotoURLs  []string  `json:"photo_urls"`
	Schedule   string    `json:"schedule"`
	Smoking    string    `json:"smoking"`
	PetsOwn    string    `json:"pets_own"`
	Noise      string    `json:"noise"`
}

type roomDTO struct {
	ID           uuid.UUID `json:"id"`
	Stage        string    `json:"stage"`
	Zone         zoneDTO   `json:"zone"`
	PriceShare   *int      `json:"price_share,omitempty"`
	Utilities    string    `json:"utilities,omitempty"`
	Isolated     *bool     `json:"isolated,omitempty"`
	Rooms        *int      `json:"rooms,omitempty"`
	Bathrooms    string    `json:"bathrooms,omitempty"`
	Deposit      *int      `json:"deposit,omitempty"`
	MoveIn       string    `json:"move_in,omitempty"`
	Description  string    `json:"description,omitempty"`
	Restrictions []string  `json:"restrictions"`
	PhotoURLs    []string  `json:"photo_urls"`
	Responses    int       `json:"responses"`
	Author       authorDTO `json:"author"`
}

type cardDTO struct {
	Kind   string     `json:"kind"`
	Person *personDTO `json:"person,omitempty"`
	Room   *roomDTO   `json:"room,omitempty"`
	Pair   pairDTO    `json:"pair"`
}

type deckResponse struct {
	Cards    []cardDTO `json:"cards"`
	NoSearch bool      `json:"no_search"`
	Total    int       `json:"total"`
}

func parseQuery(v url.Values) (deck.Query, string, bool) {
	var q deck.Query

	who := listParam(v, "who")
	q.People, q.Rooms = len(who) == 0, len(who) == 0
	for _, item := range who {
		switch item {
		case whoPeople:
			q.People = true
		case whoRooms:
			q.Rooms = true
		default:
			return q, "who", false
		}
	}

	for _, age := range []struct {
		name string
		dst  **int
	}{{"age_min", &q.Filter.AgeMin}, {"age_max", &q.Filter.AgeMax}} {
		raw := strings.TrimSpace(v.Get(age.name))
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return q, age.name, false
		}
		*age.dst = pointer.To(n)
	}

	for _, item := range listParam(v, "pets") {
		switch item {
		case petsNone:
			q.Filter.PetsOwnNone = true
		case petsOKMine:
			q.Filter.PetsOKFine = true
		default:
			return q, "pets", false
		}
	}

	var codes []string
	for _, item := range listParam(v, "smoking") {
		switch item {
		case smokingNo:
			codes = append(codes, string(entity.SmokingNo))
		case smokingOutsideOnly:
			codes = append(codes, string(entity.SmokingNo), string(entity.SmokingOutside))
		default:
			return q, "smoking", false
		}
	}
	q.Filter.Smoking = sortedSet(codes)

	codes = nil
	for _, item := range listParam(v, "schedule") {
		switch entity.ParseSchedule(item) {
		case entity.ScheduleOwl, entity.ScheduleLark:
			codes = append(codes, item)
		default:
			return q, "schedule", false
		}
	}
	q.Filter.Schedule = sortedSet(codes)

	codes = nil
	for _, item := range listParam(v, "occupation") {
		switch item {
		case occupationStudy:
			codes = append(codes, string(entity.OccupationStudy))
		case occupationWorking:
			codes = append(codes, string(entity.OccupationOffice), string(entity.OccupationHybrid),
				string(entity.OccupationRemote), string(entity.OccupationShift))
		case occupationNone:
			codes = append(codes, string(entity.OccupationNone))
		default:
			return q, "occupation", false
		}
	}
	q.Filter.Occupation = sortedSet(codes)

	return q, "", true
}

func toResponse(d deck.Deck, photos Photos) deckResponse {
	out := deckResponse{Cards: make([]cardDTO, 0, len(d.Cards)), NoSearch: d.NoSearch, Total: d.Total}
	for _, card := range d.Cards {
		out.Cards = append(out.Cards, toCard(card, photos))
	}
	return out
}

func toCard(c deck.Card, photos Photos) cardDTO {
	out := cardDTO{Kind: string(c.Kind), Pair: toPair(c.Pair)}
	if c.Kind == deck.KindRoom {
		out.Room = pointer.To(toRoom(c.Room, toZoneDTO(c.Pair.Zone), photos))
		return out
	}
	out.Person = pointer.To(toPerson(c.Person, photos))
	return out
}

func toPair(p deck.Pair) pairDTO {
	out := pairDTO{
		Zone:      toZoneDTO(p.Zone),
		ZoneMatch: p.ZoneMatch,
		Section:   string(p.Section),
	}
	for _, chip := range p.Chips {
		out.Chips = append(out.Chips, chipDTO{Param: string(chip.Param), Verdict: string(chip.Verdict)})
	}
	if p.Params != nil {
		out.Params = pointer.To(toParamsDTO(*p.Params))
	}
	return out
}

func toPerson(u entity.User, photos Photos) personDTO {
	out := personDTO{
		UserID:     u.ID,
		Name:       u.Name,
		Age:        u.Profile.Age,
		Occupation: string(u.Profile.Occupation),
		LastSeen:   u.LastSeen,
		PhotoURLs:  personPhotos(u, photos),
		Schedule:   string(u.Profile.Schedule),
		Smoking:    string(u.Profile.Smoking),
		PetsOwn:    string(u.Profile.PetsOwn),
		PetsOK:     string(u.Profile.PetsOK),
		Noise:      string(u.Profile.Noise),
		Guests:     string(u.Profile.Guests),
		Cleaning:   string(u.Profile.Cleaning),
		Alcohol:    string(u.Profile.Alcohol),
	}
	if u.AboutApproved() {
		out.About = u.Profile.About
	}
	return out
}

func toRoom(fp pointsrepo.FeedPoint, zone zoneDTO, photos Photos) roomDTO {
	p := fp.Point
	out := roomDTO{
		ID:           p.ID,
		Zone:         zone,
		PriceShare:   p.PriceShare,
		Isolated:     p.Isolated,
		Rooms:        p.Rooms,
		Deposit:      p.Deposit,
		Restrictions: make([]string, 0, len(p.Restrictions)),
		PhotoURLs:    photoURLs(p.PhotoKeys, photos),
		Responses:    fp.Responses,
	}
	if p.Stage != nil {
		out.Stage = string(*p.Stage)
	}
	if p.Bathrooms != nil {
		out.Bathrooms = string(*p.Bathrooms)
	}
	if p.Utilities != nil {
		out.Utilities = string(*p.Utilities)
	}
	if p.MoveIn != nil {
		out.MoveIn = p.MoveIn.Format(moveInDate)
	}
	if p.Description != nil {
		out.Description = *p.Description
	}
	for _, r := range p.Restrictions {
		out.Restrictions = append(out.Restrictions, string(r))
	}
	if fp.Author != nil {
		out.Author = toAuthor(*fp.Author, photos)
	}
	return out
}

func toAuthor(u entity.User, photos Photos) authorDTO {
	return authorDTO{
		UserID:     u.ID,
		Name:       u.Name,
		Age:        u.Profile.Age,
		Occupation: string(u.Profile.Occupation),
		PhotoURLs:  personPhotos(u, photos),
		Schedule:   string(u.Profile.Schedule),
		Smoking:    string(u.Profile.Smoking),
		PetsOwn:    string(u.Profile.PetsOwn),
		Noise:      string(u.Profile.Noise),
	}
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
	return out
}

func toZoneDTO(z zones.Zone) zoneDTO {
	return zoneDTO{ID: z.ID, Name: z.Name, NameIn: z.NameIn, NameGen: z.NameGen}
}

func personPhotos(u entity.User, photos Photos) []string {
	if !u.PhotoApproved() {
		return []string{}
	}
	return photoURLs(u.PhotoKeys, photos)
}

func photoURLs(keys []string, photos Photos) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, photos.URL(key))
	}
	return out
}

func listParam(v url.Values, name string) []string {
	raw := strings.TrimSpace(v.Get(name))
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func sortedSet(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}
