package pools

import (
	"slices"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
	Intersecting(id string) []string
}

type Pair struct {
	Mine   entity.Search
	Theirs entity.Search

	Zone      zones.Zone
	ZoneMatch bool
	Chips     []Chip
	Section   SectionID
}

func BestPair(dir ZoneDir, mine, theirs []entity.Search) (Pair, bool) {
	var best Pair
	var found bool

	for _, m := range mine {
		if m.Status != entity.SearchActive {
			continue
		}
		intersecting := dir.Intersecting(m.ZoneID)
		for _, t := range theirs {
			if t.Status != entity.SearchActive || !slices.Contains(intersecting, t.ZoneID) {
				continue
			}

			chips, section := Compare(m.SearchParams, t.SearchParams)
			zone, _ := dir.Get(t.ZoneID)
			next := Pair{
				Mine: m, Theirs: t,
				Zone:      zone,
				ZoneMatch: ZoneMatch(dir, m.ZoneID, t.ZoneID),
				Chips:     chips,
				Section:   section,
			}
			if !found || betterPair(next, best) {
				best, found = next, true
			}
		}
	}
	return best, found
}

func GreenChips(chips []Chip) int {
	var n int
	for _, c := range chips {
		if c.Verdict == VerdictGreen {
			n++
		}
	}
	return n
}

func ZoneMatch(dir ZoneDir, mineID, theirsID string) bool {
	if mineID == theirsID {
		return true
	}
	mine, ok1 := dir.Get(mineID)
	theirs, ok2 := dir.Get(theirsID)
	if !ok1 || !ok2 {
		return false
	}
	return subset(mine.Stations, theirs.Stations) || subset(theirs.Stations, mine.Stations)
}

func subset(inner, outer []string) bool {
	set := make(map[string]bool, len(outer))
	for _, s := range outer {
		set[s] = true
	}
	for _, s := range inner {
		if !set[s] {
			return false
		}
	}
	return true
}

func betterPair(a, b Pair) bool {
	if a.Section != b.Section {
		return BetterSection(a.Section, b.Section)
	}
	if green, was := GreenChips(a.Chips), GreenChips(b.Chips); green != was {
		return green > was
	}
	return a.Mine.CreatedAt.After(b.Mine.CreatedAt)
}
