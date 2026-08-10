package zones

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
)

type Kind string

const (
	KindDistrict Kind = "district"
	KindStation  Kind = "station"
	KindLine     Kind = "line"
)

type Zone struct {
	ID       string   `json:"id"`
	Kind     Kind     `json:"kind"`
	Name     string   `json:"name"`
	NameIn   string   `json:"name_in"`  // «в Хамовниках» / «на красной ветке» / «у метро Сокол»
	NameGen  string   `json:"name_gen"` // «Хамовников» — for «пул Хамовников»
	Stations []string `json:"stations"`
}

type Station struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

type Directory struct {
	version      int
	raw          []byte
	zones        map[string]Zone
	order        []string            // stable All() order: file order
	intersecting map[string][]string // zone id → ids sharing ≥1 station, self included
}

type file struct {
	Version  int       `json:"version"`
	Stations []Station `json:"stations"`
	Zones    []Zone    `json:"zones"`
}

//go:embed moscow.json
var moscowJSON []byte

func Load() (*Directory, error) {
	return New(moscowJSON)
}

func New(raw []byte) (*Directory, error) {
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse zones file: %w", err)
	}
	if f.Version < 1 {
		return nil, fmt.Errorf("zones file version %d, want >= 1", f.Version)
	}

	stations := make(map[string]bool, len(f.Stations))
	for _, s := range f.Stations {
		if s.ID == "" || s.Name == "" {
			return nil, fmt.Errorf("station %q: empty id or name", s.ID)
		}
		if stations[s.ID] {
			return nil, fmt.Errorf("duplicate station id %q", s.ID)
		}
		stations[s.ID] = true
	}

	d := &Directory{
		version:      f.Version,
		raw:          raw,
		zones:        make(map[string]Zone, len(f.Zones)),
		intersecting: make(map[string][]string, len(f.Zones)),
	}
	byStation := make(map[string][]string)
	for _, z := range f.Zones {
		if err := validateZone(z, stations); err != nil {
			return nil, err
		}
		if _, dup := d.zones[z.ID]; dup {
			return nil, fmt.Errorf("duplicate zone id %q", z.ID)
		}
		d.zones[z.ID] = z
		d.order = append(d.order, z.ID)
		for _, st := range z.Stations {
			byStation[st] = append(byStation[st], z.ID)
		}
	}

	for id, z := range d.zones {
		seen := map[string]bool{}
		for _, st := range z.Stations {
			for _, other := range byStation[st] {
				seen[other] = true
			}
		}
		ids := make([]string, 0, len(seen))
		for other := range seen {
			ids = append(ids, other)
		}
		slices.Sort(ids)
		d.intersecting[id] = ids
	}

	return d, nil
}

// Get returns the zone by id. The Zone is a copy, but its Stations slice is
// NOT: it aliases the directory's own array, which is shared by every caller
// and never rebuilt after startup. Read it, never write to it — appending to
// or sorting it corrupts the directory for the whole process.
func (d *Directory) Get(id string) (Zone, bool) {
	z, ok := d.zones[id]
	return z, ok
}

// Intersecting answers the pool boundary (§2.1): every zone sharing at least
// one station with the given one, itself included. The returned slice is the
// directory's own, precomputed at startup and shared by every caller: it is
// read-only for the same reason Get's Stations are. Callers that need to
// modify it must copy first (slices.Clone).
func (d *Directory) Intersecting(id string) []string {
	return d.intersecting[id]
}

func (d *Directory) All() []Zone {
	out := make([]Zone, 0, len(d.order))
	for _, id := range d.order {
		out = append(out, d.zones[id])
	}
	return out
}

func (d *Directory) Version() int { return d.version }

func (d *Directory) Raw() []byte { return d.raw }

func validateZone(z Zone, stations map[string]bool) error {
	if z.ID == "" || z.Name == "" || z.NameIn == "" || z.NameGen == "" {
		return fmt.Errorf("zone %q: empty id, name or declension", z.ID)
	}
	if z.Kind != KindDistrict && z.Kind != KindStation && z.Kind != KindLine {
		return fmt.Errorf("zone %q: unknown kind %q", z.ID, z.Kind)
	}
	if len(z.Stations) == 0 {
		return fmt.Errorf("zone %q: no stations — it would intersect nothing", z.ID)
	}
	for _, st := range z.Stations {
		if !stations[st] {
			return fmt.Errorf("zone %q: unknown station %q", z.ID, st)
		}
	}
	return nil
}
