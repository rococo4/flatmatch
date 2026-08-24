package deckapi

import (
	"net/url"
	"reflect"
	"slices"
	"testing"

	"github.com/AlekSi/pointer"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/deck"
)

func query(t *testing.T, raw string) deck.Query {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	q, bad, ok := parseQuery(values)
	if !ok {
		t.Fatalf("parseQuery(%q) refused %q", raw, bad)
	}
	return q
}

func TestParseQueryDefaultsToEverything(t *testing.T) {
	q := query(t, "")

	if !q.People || !q.Rooms {
		t.Errorf("who = people:%v rooms:%v, want both", q.People, q.Rooms)
	}
	empty := entity.PeopleFilter{}
	if !reflect.DeepEqual(q.Filter, empty) {
		t.Errorf("filter = %+v, want an empty one", q.Filter)
	}
}

func TestParseQueryReadsEveryFilter(t *testing.T) {
	q := query(t, "who=rooms&age_min=18&age_max=30&pets=none,ok_mine"+
		"&smoking=no,outside_only&schedule=owl,lark&occupation=study,working,none")

	if q.People || !q.Rooms {
		t.Errorf("who = people:%v rooms:%v, want rooms only", q.People, q.Rooms)
	}
	if pointer.Get(q.Filter.AgeMin) != 18 || pointer.Get(q.Filter.AgeMax) != 30 {
		t.Errorf("age = %v–%v, want 18–30", q.Filter.AgeMin, q.Filter.AgeMax)
	}
	if !q.Filter.PetsOwnNone || !q.Filter.PetsOKFine {
		t.Errorf("pets = %+v, want both flags", q.Filter)
	}
	if !slices.Equal(q.Filter.Smoking, []string{"no", "outside"}) {
		t.Errorf("smoking = %v, want [no outside]", q.Filter.Smoking)
	}
	if !slices.Equal(q.Filter.Schedule, []string{"lark", "owl"}) {
		t.Errorf("schedule = %v, want [lark owl]", q.Filter.Schedule)
	}
	want := []string{"hybrid", "none", "office", "remote", "shift", "study"}
	if !slices.Equal(q.Filter.Occupation, want) {
		t.Errorf("occupation = %v, want %v — working раскрывается в четыре кода",
			q.Filter.Occupation, want)
	}
}

func TestParseQueryNamesTheBrokenParam(t *testing.T) {
	cases := map[string]string{
		"who=nobody":         "who",
		"age_min=соседи":     "age_min",
		"age_max=-1":         "age_max",
		"pets=любые":         "pets",
		"smoking=sometimes":  "smoking",
		"schedule=floating":  "schedule",
		"occupation=resting": "occupation",
	}

	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			values, err := url.ParseQuery(raw)
			if err != nil {
				t.Fatalf("parse %q: %v", raw, err)
			}
			if _, bad, ok := parseQuery(values); ok || bad != want {
				t.Errorf("parseQuery(%q) = %q, %v; want %q refused", raw, bad, ok, want)
			}
		})
	}
}
