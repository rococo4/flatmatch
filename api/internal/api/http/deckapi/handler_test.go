package deckapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/deck"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

type fakeDeck struct {
	got   deck.Query
	calls int
	deck  deck.Deck
}

type fakePhotos struct{}

type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

func testZone() zones.Zone {
	return zones.Zone{ID: "d-khamovniki", Kind: zones.KindDistrict,
		Name: "Хамовники", NameIn: "в Хамовниках", NameGen: "Хамовников"}
}

func get(t *testing.T, d *fakeDeck, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusActive},
	}, nil))
	r.Mount("/deck", New(d, fakePhotos{}).Routes())

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "tma stub")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestDeckCardsCarryPeopleAndRooms(t *testing.T) {
	seen := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	mate := entity.User{
		ID: uuid.New(), Name: "Катя", LastSeen: seen,
		Profile: entity.Profile{
			Age: 23, Occupation: entity.OccupationStudy, Schedule: entity.ScheduleOwl,
			Smoking: entity.SmokingNo, PetsOwn: entity.PetsOwnCat, PetsOK: entity.PetsOKFine,
			Noise: entity.NoiseNormal, Guests: entity.GuestsRare,
			Cleaning: entity.CleaningSchedule, Alcohol: entity.AlcoholNone,
			About: "люблю тишину",
		},
		PhotoKeys: []string{"photos/kate.jpg"}, PhotoStatus: entity.ModerationOK,
		AboutStatus: entity.ModerationOK,
	}
	host := entity.User{
		ID: uuid.New(), Name: "Оля",
		Profile:   entity.Profile{Age: 27, Occupation: entity.OccupationRemote},
		PhotoKeys: []string{"photos/olya.jpg"}, PhotoStatus: entity.ModerationOK,
	}
	room := pointsrepo.FeedPoint{
		Point: entity.Point{
			ID: uuid.New(), Type: entity.PointIntentHome, Stage: pointer.To(entity.StageLive),
			ZoneID: "d-khamovniki", PriceShare: pointer.To(42000),
			Utilities: pointer.To(entity.UtilitiesMeters), Isolated: pointer.To(true),
			Rooms: pointer.To(2), Description: pointer.To("светлая комната"),
			Restrictions: []entity.Restriction{entity.RestrictionNoSmoking},
			PhotoKeys:    []string{"points/room.jpg"},
		},
		Author: &host, Responses: 3,
	}

	d := &fakeDeck{deck: deck.Deck{Total: 2, Cards: []deck.Card{
		{
			Kind:   deck.KindPerson,
			Person: mate,
			Pair: deck.Pair{
				Zone: testZone(), ZoneMatch: true, Section: pools.SectionAll,
				Chips:  []pools.Chip{{Param: pools.ParamPrice, Verdict: pools.VerdictGreen}},
				Params: &entity.SearchParams{PriceMin: pointer.To(35000), PriceMax: 45000},
			},
		},
		{
			Kind: deck.KindRoom,
			Room: room,
			Pair: deck.Pair{Zone: testZone(), Section: pools.SectionPartial},
		},
	}}}

	rec := get(t, d, "/deck")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body)
	}

	var got deckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v (%s)", err, rec.Body)
	}
	if got.Total != 2 || len(got.Cards) != 2 || got.NoSearch {
		t.Fatalf("response = %+v, want two cards", got)
	}

	person := got.Cards[0]
	if person.Kind != string(deck.KindPerson) || person.Person == nil || person.Room != nil {
		t.Fatalf("card 0 = %+v, want a person", person)
	}
	if person.Person.About != "люблю тишину" || person.Person.Occupation != "study" {
		t.Errorf("person = %+v, want the approved about and occupation", person.Person)
	}
	if len(person.Person.PhotoURLs) != 1 || person.Person.PhotoURLs[0] != "/media/photos/kate.jpg" {
		t.Errorf("photos = %v, want the approved set", person.Person.PhotoURLs)
	}
	if person.Pair.Section != "all" || !person.Pair.ZoneMatch ||
		person.Pair.Zone.ID != "d-khamovniki" || len(person.Pair.Chips) != 1 {
		t.Errorf("pair = %+v, want the full match in Хамовники", person.Pair)
	}
	if person.Pair.Params == nil || person.Pair.Params.PriceMax != 45000 {
		t.Errorf("pair params = %+v, want their search", person.Pair.Params)
	}

	card := got.Cards[1]
	if card.Kind != string(deck.KindRoom) || card.Room == nil || card.Person != nil {
		t.Fatalf("card 1 = %+v, want a room", card)
	}
	if card.Room.ID != room.Point.ID || card.Room.Stage != "live" || card.Room.Responses != 3 {
		t.Errorf("room = %+v, want the live room with three responses", card.Room)
	}
	if card.Room.Zone.ID != "d-khamovniki" || pointer.Get(card.Room.PriceShare) != 42000 ||
		card.Room.Description != "светлая комната" || card.Room.Utilities != "meters" {
		t.Errorf("room facts = %+v", card.Room)
	}
	if len(card.Room.Restrictions) != 1 || card.Room.Restrictions[0] != "no_smoking" {
		t.Errorf("restrictions = %v, want [no_smoking]", card.Room.Restrictions)
	}
	if card.Room.Author.UserID != host.ID || len(card.Room.Author.PhotoURLs) != 1 {
		t.Errorf("author = %+v, want %s with a photo", card.Room.Author, host.ID)
	}
	if card.Pair.Params != nil || len(card.Pair.Chips) != 0 {
		t.Errorf("room pair = %+v, want no chips and no params", card.Pair)
	}
}

func TestDeckHidesUnapprovedPhotoAndAbout(t *testing.T) {
	mate := entity.User{
		ID: uuid.New(), Name: "Катя",
		Profile:     entity.Profile{About: "текст на проверке"},
		PhotoKeys:   []string{"photos/kate.jpg"},
		PhotoStatus: entity.ModerationPending,
		AboutStatus: entity.ModerationPending,
	}
	d := &fakeDeck{deck: deck.Deck{Total: 1, Cards: []deck.Card{
		{Kind: deck.KindPerson, Person: mate, Pair: deck.Pair{Zone: testZone(), Section: pools.SectionNone}},
	}}}

	rec := get(t, d, "/deck")

	var got deckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v (%s)", err, rec.Body)
	}
	if len(got.Cards[0].Person.PhotoURLs) != 0 || got.Cards[0].Person.About != "" {
		t.Errorf("card = %+v, want no unapproved photo and no unapproved about", got.Cards[0].Person)
	}
}

func TestDeckWithoutSearchAnswersEmpty(t *testing.T) {
	d := &fakeDeck{deck: deck.Deck{NoSearch: true, Cards: []deck.Card{}}}

	rec := get(t, d, "/deck")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{`"cards":[]`, `"no_search":true`, `"total":0`} {
		if !strings.Contains(body, want) {
			t.Errorf("body %s does not carry %s", body, want)
		}
	}
}

func TestDeckPassesTheQueryToTheService(t *testing.T) {
	d := &fakeDeck{}

	rec := get(t, d, "/deck?who=rooms&age_min=18&smoking=outside_only")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	if d.calls != 1 || d.got.People || !d.got.Rooms {
		t.Errorf("service saw %d call(s) with %+v, want one for rooms", d.calls, d.got)
	}
	if pointer.Get(d.got.Filter.AgeMin) != 18 || len(d.got.Filter.Smoking) != 2 {
		t.Errorf("filter = %+v, want age 18+ and two smoking codes", d.got.Filter)
	}
}

func TestDeckRefusesAnUnknownFilterValue(t *testing.T) {
	d := &fakeDeck{}

	rec := get(t, d, "/deck?pets=любые")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body)
	}
	for _, want := range []string{`"invalid_field"`, `"pets"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body %s does not carry %s", rec.Body, want)
		}
	}
	if d.calls != 0 {
		t.Errorf("a broken filter reached the service %d time(s)", d.calls)
	}
}

func (f *fakeDeck) Deck(_ context.Context, _ uuid.UUID, q deck.Query) (deck.Deck, error) {
	f.calls++
	f.got = q
	return f.deck, nil
}

func (fakePhotos) URL(key string) string { return "/media/" + key }

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 777000001, FirstName: "Дев"}}, nil
}

func (f fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}
