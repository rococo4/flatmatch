package zones

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// goldenIDs is the frozen zone id list. Zone ids end up in searches.zone_id, so
// a regeneration that renames a slug orphans every saved search pointing at it —
// and hh has already renamed a station once (Коммунарка → Новомосковская).
// Nothing else in the suite would notice, hence this file.
const goldenIDs = "testdata/moscow_ids.txt"

func TestMoscowLoads(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	byKind := map[Kind]int{}
	for _, z := range d.All() {
		byKind[z.Kind]++
	}
	// Orders of magnitude of the real Moscow, not exact numbers: the file gets
	// regenerated when a line opens, and the test must not break on that.
	if byKind[KindLine] < 12 {
		t.Errorf("only %d lines", byKind[KindLine])
	}
	if byKind[KindDistrict] < 100 {
		t.Errorf("only %d districts", byKind[KindDistrict])
	}
	if byKind[KindStation] < 200 {
		t.Errorf("only %d station zones", byKind[KindStation])
	}

	// Counts alone are blind to a whole family going missing: drop all four
	// diameters and 16 lines still clear the threshold above.
	for _, id := range []string{
		"line-1", "line-11", "line-mck",
		"line-d1", "line-d2", "line-d3", "line-d4",
	} {
		z, ok := d.Get(id)
		if !ok {
			t.Errorf("line %s is missing from the directory", id)
			continue
		}
		if z.Kind != KindLine {
			t.Errorf("%s: kind %q, want line", id, z.Kind)
		}
	}
}

// TestMoscowZoneIDsAreFrozen diffs the directory against the golden id list.
// Run with UPDATE_ZONE_IDS=1 to rewrite that list — deliberately, after reading
// what changed.
func TestMoscowZoneIDsAreFrozen(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	current := make([]string, 0, len(d.All()))
	for _, z := range d.All() {
		current = append(current, z.ID)
	}
	slices.Sort(current)

	if os.Getenv("UPDATE_ZONE_IDS") != "" {
		if err := os.WriteFile(goldenIDs, []byte(strings.Join(current, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("update %s: %v", goldenIDs, err)
		}
		t.Logf("rewrote %s with %d ids", goldenIDs, len(current))
		return
	}

	raw, err := os.ReadFile(goldenIDs)
	if err != nil {
		t.Fatalf("read %s: %v", goldenIDs, err)
	}
	var want []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			want = append(want, line)
		}
	}

	have := map[string]bool{}
	for _, id := range current {
		have[id] = true
	}
	known := map[string]bool{}
	var gone []string
	for _, id := range want {
		known[id] = true
		if !have[id] {
			gone = append(gone, id)
		}
	}
	var added []string
	for _, id := range current {
		if !known[id] {
			added = append(added, id)
		}
	}

	if len(gone) > 0 {
		t.Errorf("%d zone id(s) disappeared: %s\n"+
			"Zone ids are permanent — they are stored in searches.zone_id. A rename or removal "+
			"orphans every saved search pointing at them. Either restore the id, or migrate the "+
			"affected searches first and only then update %s.",
			len(gone), strings.Join(gone, ", "), goldenIDs)
	}
	if len(added) > 0 {
		t.Errorf("%d new zone id(s): %s\n"+
			"That is fine when a line or district genuinely opened — confirm these are the ones you "+
			"expected, then rerun with UPDATE_ZONE_IDS=1 to record them in %s.",
			len(added), strings.Join(added, ", "), goldenIDs)
	}
}

func TestMoscowSpotChecks(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Exact strings, not just the preposition: a prefix check cannot tell
	// «в Строгине» from «в Строгино», and the declined literary form is the
	// whole point of hand-writing declensions.json.
	for id, want := range map[string][2]string{
		"d-khamovniki":            {"в Хамовниках", "Хамовников"},
		"d-strogino":              {"в Строгине", "Строгина"},
		"d-mitino":                {"в Митине", "Митина"},
		"d-vnukovo":               {"во Внукове", "Внукова"},
		"d-pokrovskoe-streshnevo": {"в Покровском-Стрешневе", "Покровского-Стрешнева"},
		"d-cheremushki":           {"в Черёмушках", "Черёмушек"},
		"d-arbat":                 {"на Арбате", "Арбата"},
		"d-sokol":                 {"на Соколе", "Сокола"},
	} {
		z, ok := d.Get(id)
		if !ok {
			t.Errorf("no %s", id)
			continue
		}
		if z.NameIn != want[0] || z.NameGen != want[1] {
			t.Errorf("%s: name_in %q name_gen %q, want %q / %q", id, z.NameIn, z.NameGen, want[0], want[1])
		}
	}
	// The red line runs through Khamovniki — the plain geographic fact the
	// whole intersection model rests on.
	if !slices.Contains(d.Intersecting("d-khamovniki"), "line-1") {
		t.Error("line-1 must intersect d-khamovniki")
	}

	for _, z := range d.All() {
		switch z.Kind {
		case KindStation:
			if !strings.HasPrefix(z.NameIn, "у метро ") {
				t.Errorf("station zone %s: name_in %q", z.ID, z.NameIn)
			}
		case KindDistrict:
			if !strings.HasPrefix(z.NameIn, "в ") && !strings.HasPrefix(z.NameIn, "на ") &&
				!strings.HasPrefix(z.NameIn, "во ") {
				t.Errorf("district %s: name_in %q", z.ID, z.NameIn)
			}
		}
	}
}

// TestMoscowInvariants guards what zone ids mean rather than what they contain:
// the slugs are frozen into searches.zone_id forever, so a regeneration that
// renamed or duplicated one would orphan saved searches.
func TestMoscowInvariants(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	seenID := map[string]bool{}
	// All three names are checked, not just Name: the picker reads Name, but
	// every pool headline and «пул {зоны}» is built from the declensions, so
	// two zones sharing NameIn are indistinguishable exactly where it matters.
	// line-8 and line-8a were that case — both «на жёлтой ветке».
	seen := map[string]map[Kind]map[string]bool{
		"name": {}, "name_in": {}, "name_gen": {},
	}
	for _, z := range d.All() {
		if seenID[z.ID] {
			t.Fatalf("duplicate zone id %q", z.ID)
		}
		seenID[z.ID] = true

		for field, value := range map[string]string{
			"name": z.Name, "name_in": z.NameIn, "name_gen": z.NameGen,
		} {
			if seen[field][z.Kind] == nil {
				seen[field][z.Kind] = map[string]bool{}
			}
			if seen[field][z.Kind][value] {
				t.Errorf("two %s zones with %s %q — nothing in the UI could tell them apart",
					z.Kind, field, value)
			}
			seen[field][z.Kind][value] = true
		}

		if len(z.Stations) == 0 {
			t.Errorf("zone %s: no stations", z.ID)
		}
		if z.NameIn == "" || z.NameGen == "" {
			t.Errorf("zone %s: empty declension", z.ID)
		}
		// The engine tolerates a repeated station; the data file must not have
		// one, or «N станций» in the UI would lie.
		inZone := map[string]bool{}
		for _, st := range z.Stations {
			if inZone[st] {
				t.Errorf("zone %s: station %q listed twice", z.ID, st)
			}
			inZone[st] = true
		}

		var wantPrefix string
		switch z.Kind {
		case KindDistrict:
			wantPrefix = "d-"
		case KindStation:
			wantPrefix = "st-"
		case KindLine:
			wantPrefix = "line-"
		}
		if !strings.HasPrefix(z.ID, wantPrefix) {
			t.Errorf("zone %s of kind %s: id must start with %q", z.ID, z.Kind, wantPrefix)
		}
	}
}

// TestMoscowGeography pins a handful of facts a broken generator would get
// wrong silently: a bad point-in-polygon still produces a valid file.
func TestMoscowGeography(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	stationNames := func(zoneID string) []string {
		z, ok := d.Get(zoneID)
		if !ok {
			t.Fatalf("no zone %q", zoneID)
		}
		var out []string
		for _, st := range z.Stations {
			out = append(out, st)
		}
		return out
	}
	contains := func(zoneID, stationID string) bool {
		return slices.Contains(stationNames(zoneID), stationID)
	}

	for _, st := range []string{"st:1:park-kultury", "st:1:frunzenskaya", "st:1:sportivnaya"} {
		if !contains("d-khamovniki", st) {
			t.Errorf("d-khamovniki must contain %s", st)
		}
	}
	// Сокол is a Замоскворецкая station standing in the district of the same
	// name — the case that proves station zones and district zones are built
	// from the same station ids.
	if !contains("line-2", "st:2:sokol") {
		t.Error("line-2 must contain st:2:sokol")
	}
	if !contains("st-sokol", "st:2:sokol") {
		t.Error("st-sokol must contain st:2:sokol")
	}
	if !contains("d-sokol", "st:2:sokol") {
		t.Error("d-sokol must contain st:2:sokol")
	}
	// The БКЛ is the biggest line in the network; a truncated source would show
	// up here first.
	if n := len(stationNames("line-11")); n < 25 {
		t.Errorf("line-11 (БКЛ) has only %d stations", n)
	}
	// Central districts must never end up station-less through a polygon bug.
	for _, id := range []string{"d-arbat", "d-yakimanka", "d-tverskoi", "d-basmannyi"} {
		if n := len(stationNames(id)); n == 0 {
			t.Errorf("%s has no stations", id)
		}
	}
	// Бутово is served by its own light metro and by nothing else — the check
	// that outlying districts are not quietly attached to the centre.
	if !slices.Contains(d.Intersecting("d-yuzhnoe-butovo"), "line-12") {
		t.Error("line-12 must intersect d-yuzhnoe-butovo")
	}
}
