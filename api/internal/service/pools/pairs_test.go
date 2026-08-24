package pools

import (
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

func search(zone string, min, max int, created time.Time, months ...time.Time) entity.Search {
	return entity.Search{
		ID:     uuid.New(),
		Status: entity.SearchActive,
		SearchParams: entity.SearchParams{
			ZoneID:       zone,
			PriceMin:     pointer.To(min),
			PriceMax:     max,
			MoveInMonths: months,
		},
		CreatedAt: created,
	}
}

func TestBestPairTakesTheHighestSection(t *testing.T) {
	dir := testDir(t)
	old := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	fresh := old.Add(time.Hour)

	mine := []entity.Search{
		search("pz-district", 35000, 45000, fresh, moveSept),
		search("pz-line-cross", 35000, 45000, old, moveSept),
	}
	theirs := []entity.Search{
		search("pz-line-east", 90000, 120000, fresh, moveDec),
		search("pz-station-west", 40000, 55000, old, moveSept),
	}

	got, ok := BestPair(dir, mine, theirs)
	if !ok {
		t.Fatal("BestPair() found nothing, want the cross/west pair")
	}
	if got.Section != SectionAll {
		t.Errorf("section = %q, want all — the full match must win over the fresher search", got.Section)
	}
	if got.Mine.ZoneID != "pz-line-cross" || got.Theirs.ZoneID != "pz-station-west" {
		t.Errorf("pair = %s × %s, want pz-line-cross × pz-station-west",
			got.Mine.ZoneID, got.Theirs.ZoneID)
	}
	if got.Zone.ID != "pz-station-west" {
		t.Errorf("zone = %q, want the zone of their search", got.Zone.ID)
	}
}

func TestBestPairCountsGreenChipsThenFreshness(t *testing.T) {
	dir := testDir(t)
	old := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	fresh := old.Add(time.Hour)

	mine := []entity.Search{
		search("pz-district", 35000, 45000, old, moveSept),
		search("pz-line-west", 35000, 45000, fresh, moveSept),
	}

	greener := search("pz-station-east", 40000, 55000, old, moveSept)
	greener.Rooms = []int{2}
	paler := search("pz-line-west", 40000, 55000, fresh, moveSept)

	got, ok := BestPair(dir, mine, []entity.Search{paler, greener})
	if !ok {
		t.Fatal("BestPair() found nothing")
	}
	if got.Theirs.ZoneID != "pz-station-east" {
		t.Errorf("their search = %q, want pz-station-east — больше зелёных чипов", got.Theirs.ZoneID)
	}

	sameChips := search("pz-line-west", 40000, 55000, old, moveSept)
	got, ok = BestPair(dir, mine, []entity.Search{sameChips})
	if !ok {
		t.Fatal("BestPair() found nothing")
	}
	if got.Mine.ZoneID != "pz-line-west" {
		t.Errorf("my search = %q, want the fresher pz-line-west", got.Mine.ZoneID)
	}
}

func TestBestPairNeedsIntersectingZones(t *testing.T) {
	dir := testDir(t)
	now := time.Now()

	mine := []entity.Search{search("pz-line-west", 35000, 45000, now, moveSept)}
	theirs := []entity.Search{search("pz-station-east", 35000, 45000, now, moveSept)}

	if _, ok := BestPair(dir, mine, theirs); ok {
		t.Error("BestPair() paired zones that do not intersect")
	}
}

func TestBestPairSkipsInactiveSearches(t *testing.T) {
	dir := testDir(t)
	now := time.Now()

	closed := search("pz-district", 35000, 45000, now, moveSept)
	closed.Status = entity.SearchClosed
	paused := search("pz-district", 35000, 45000, now, moveSept)
	paused.Status = entity.SearchPaused
	live := search("pz-district", 40000, 55000, now, moveSept)

	if _, ok := BestPair(dir, []entity.Search{closed}, []entity.Search{live}); ok {
		t.Error("a closed search of mine must not pair")
	}
	if _, ok := BestPair(dir, []entity.Search{live}, []entity.Search{paused}); ok {
		t.Error("a paused search of theirs must not pair")
	}
}

func TestBestPairMarksTheSameZone(t *testing.T) {
	dir := testDir(t)
	now := time.Now()

	cases := map[string]struct {
		mineZone, theirsZone string
		want                 bool
	}{
		"та же зона":            {"pz-district", "pz-district", true},
		"станция внутри района": {"pz-district", "pz-station-east", true},
		"ветка внутри района":   {"pz-line-west", "pz-district", true},
		"пересеклись краем":     {"pz-line-cross", "pz-district", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := BestPair(dir,
				[]entity.Search{search(tc.mineZone, 35000, 45000, now, moveSept)},
				[]entity.Search{search(tc.theirsZone, 40000, 55000, now, moveSept)})
			if !ok {
				t.Fatal("BestPair() found nothing")
			}
			if got.ZoneMatch != tc.want {
				t.Errorf("ZoneMatch = %v, want %v", got.ZoneMatch, tc.want)
			}
		})
	}
}

func TestGreenChips(t *testing.T) {
	chips := []Chip{
		{ParamPrice, VerdictGreen},
		{ParamRooms, VerdictEdge},
		{ParamMoveIn, VerdictGreen},
		{ParamTerm, VerdictGrey},
	}
	if got := GreenChips(chips); got != 2 {
		t.Errorf("GreenChips() = %d, want 2", got)
	}
}
