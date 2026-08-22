package pools

import (
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

func candidate(created time.Time, p entity.SearchParams) entity.PoolCandidate {
	return entity.PoolCandidate{
		User:   entity.User{ID: uuid.New()},
		Search: entity.Search{SearchParams: p, CreatedAt: created},
	}
}

func TestRankedSectionsCapOnlyCutsTheNoneTail(t *testing.T) {
	mine := entity.SearchParams{PriceMin: pointer.To(35000), PriceMax: 45000, MoveInMonths: []time.Time{moveSept}}
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	var cands []entity.PoolCandidate
	for i := range noneSectionCap + 1 {
		at := base.Add(time.Duration(i) * time.Minute)
		cands = append(cands,
			candidate(at, entity.SearchParams{PriceMin: pointer.To(40000), PriceMax: 55000, MoveInMonths: []time.Time{moveSept}}),
			candidate(at, entity.SearchParams{PriceMin: pointer.To(90000), PriceMax: 120000, MoveInMonths: []time.Time{moveDec}}))
	}

	sections := rankedSections(testDir(t), staticPhotos{}, entity.User{}, mine, cands)
	if got := len(sections[0].People); got != noneSectionCap+1 {
		t.Errorf("«совпали по всем» holds %d cards, want all %d — this section is never cut",
			got, noneSectionCap+1)
	}
	if got := len(sections[2].People); got != noneSectionCap {
		t.Errorf("«не совпали» holds %d cards, want the cap of %d", got, noneSectionCap)
	}
}

func TestRankedSectionsSortFreshestFirst(t *testing.T) {
	mine := entity.SearchParams{PriceMin: pointer.To(35000), PriceMax: 45000, MoveInMonths: []time.Time{moveSept}}
	theirs := entity.SearchParams{PriceMin: pointer.To(40000), PriceMax: 55000, MoveInMonths: []time.Time{moveSept}}
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	oldest := candidate(base, theirs)
	newest := candidate(base.Add(2*time.Hour), theirs)
	middle := candidate(base.Add(time.Hour), theirs)

	sections := rankedSections(testDir(t), staticPhotos{}, entity.User{}, mine,
		[]entity.PoolCandidate{oldest, newest, middle})
	want := []uuid.UUID{newest.User.ID, middle.User.ID, oldest.User.ID}
	if len(sections[0].People) != len(want) {
		t.Fatalf("got %d cards in «совпали по всем», want %d", len(sections[0].People), len(want))
	}
	for i, id := range want {
		if sections[0].People[i].User.ID != id {
			t.Errorf("card %d is %s (created %v), want %s",
				i, sections[0].People[i].User.ID, sections[0].People[i].SearchCreated, id)
		}
	}
}

func TestRankedSectionsBoostZoneMatch(t *testing.T) {
	mine := entity.SearchParams{ZoneID: "pz-district", PriceMin: pointer.To(35000), PriceMax: 45000}
	theirs := entity.SearchParams{PriceMin: pointer.To(40000), PriceMax: 55000}
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	crossZone := candidate(base.Add(3*time.Hour), withZone(theirs, "pz-line-cross"))

	subsetZone := candidate(base.Add(time.Hour), withZone(theirs, "pz-line-west"))
	sameZone := candidate(base, withZone(theirs, "pz-district"))

	sections := rankedSections(testDir(t), staticPhotos{}, entity.User{}, mine,
		[]entity.PoolCandidate{crossZone, subsetZone, sameZone})

	top := sections[0].People
	if len(top) != 3 {
		t.Fatalf("got %d cards, want 3", len(top))
	}

	wantOrder := []struct {
		id    uuid.UUID
		match bool
	}{{subsetZone.User.ID, true}, {sameZone.User.ID, true}, {crossZone.User.ID, false}}
	for i, w := range wantOrder {
		if top[i].User.ID != w.id || top[i].ZoneMatch != w.match {
			t.Errorf("card %d = %s match=%v, want %s match=%v",
				i, top[i].User.ID, top[i].ZoneMatch, w.id, w.match)
		}
	}
}

func withZone(p entity.SearchParams, zone string) entity.SearchParams {
	p.ZoneID = zone
	return p
}
