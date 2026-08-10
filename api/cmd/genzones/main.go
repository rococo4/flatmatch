// Command genzones builds internal/service/zones/moscow.json — the finite,
// versioned directory of Moscow districts, metro stations and metro lines (§8).
//
// It is a one-shot tool, not part of the service: the product reads the
// committed JSON, and this exists so the file can be regenerated when a new
// line opens. Only the standard library is used.
//
// Sources (both checked live on 2026-08-07):
//
//   - Stations and lines: https://api.hh.ru/metro/1 — `{lines: [{id, name,
//     hex_color, stations: [{id, name, lat, lng, order, line}]}]}`, 21 lines /
//     452 platforms, WGS84. Includes МЦК (name «МЦК») and МЦД-1…МЦД-4, plus a
//     stale «Каховская» line absorbed into the БКЛ in 2023 (see skippedLines).
//     Line names in the payload are the keys of linesByOfficialName; an
//     unmapped one fails generation instead of silently dropping stations.
//   - District boundaries: https://gis-lab.info/data/mos-adm/mo.geojson —
//     FeatureCollection, CRS84, 146 features, geometry Polygon (141) or
//     MultiPolygon (5). Properties: NAME («Хамовники»), NAME_AO / ABBREV_AO
//     (okrug, e.g. «Троицкий»), TYPE_MO, OKATO, OKTMO. gis-lab answered, so the
//     Overpass fallback was not needed.
//
// Usage:
//
//	cd api && go run ./cmd/genzones -out internal/service/zones/moscow.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"flatmatch/api/internal/service/zones"
)

const (
	// boundaryBufferM: a station just outside a district line still serves it —
	// «Парк культуры» is a Khamovniki station whichever side of the street the
	// vestibule sits on.
	boundaryBufferM = 400.0
	// nearestStationsForMetroless: districts with no metro of their own get the
	// closest few, so that no district is an empty zone the engine would reject.
	nearestStationsForMetroless = 3

	// Новая Москва is not a zone: its settlements are villages an hour out, and
	// pooling flatmates there with Moscow ones would be noise (§1 — the market
	// is Moscow). Their stations still reach users through their lines.
	okrugTroitsky      = "Троицкий"
	okrugNovomoskovsky = "Новомосковский"

	metersPerDegreeLat = 110540.0
	metersPerDegreeLng = 111320.0

	// stationSplitM breaks up a same-named group whose platforms are not one
	// place: «у метро Лианозово» must not span a 40-minute walk between the
	// metro and the МЦД platform.
	stationSplitM = 1500.0

	// dirVersion: 2 — МЦД split-offs, Новомосковская merge, район Аэропорт.
	dirVersion = 2

	fetchTimeout = 60 * time.Second
)

// hhMetro is the slice of api.hh.ru/metro/1 this tool needs.
type hhMetro struct {
	Lines []hhLine `json:"lines"`
}

type hhLine struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Stations []hhStation `json:"stations"`
}

type hhStation struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

type geoFile struct {
	Features []geoFeature `json:"features"`
}

type geoFeature struct {
	Properties struct {
		Name   string `json:"NAME"`
		NameAO string `json:"NAME_AO"`
	} `json:"properties"`
	Geometry struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
}

// point is lng, lat — GeoJSON order, kept as-is to avoid a swap per vertex.
type point [2]float64

// district is one boundary polygon set. Interior holes are dropped: for
// deciding which station serves a district they are noise.
type district struct {
	Name    string
	OkrugAO string
	Rings   [][]point
	// Bounding box of all rings, for a cheap reject before the ray cast.
	MinLng, MinLat, MaxLng, MaxLat float64
}

// declension is one hand-written entry of declensions.json.
type declension struct {
	In  string `json:"in"`
	Gen string `json:"gen"`
}

// outFile mirrors the zones package file format; it is written, then read back
// through zones.New before the tool exits successfully.
type outFile struct {
	Version  int             `json:"version"`
	Stations []zones.Station `json:"stations"`
	Zones    []zones.Zone    `json:"zones"`
}

// translitTable is a deliberately dumb 1:1 map. Zone slugs land in
// searches.zone_id and are therefore FOREVER — a clever scheme that changes its
// mind on the next Go version would orphan every saved search.
var translitTable = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

func main() {
	var (
		metroSrc  = flag.String("metro", "https://api.hh.ru/metro/1", "hh metro payload: URL or file path")
		districts = flag.String("districts", "/tmp/mo.geojson", "Moscow districts GeoJSON: URL or file path")
		declSrc   = flag.String("declensions", "cmd/genzones/declensions.json", "hand-written district morphology")
		out       = flag.String("out", "internal/service/zones/moscow.json", "output file")
	)
	flag.Parse()

	if err := run(*metroSrc, *districts, *declSrc, *out); err != nil {
		fmt.Fprintf(os.Stderr, "genzones: %v\n", err)
		os.Exit(1)
	}
}

func run(metroSrc, districtsSrc, declSrc, out string) error {
	metro, err := loadMetro(metroSrc)
	if err != nil {
		return err
	}
	all, zoneDistricts, err := loadDistricts(districtsSrc)
	if err != nil {
		return err
	}
	decls, err := loadDeclensions(declSrc)
	if err != nil {
		return err
	}

	fmt.Printf("== genzones ==\nmetro: %s\ndistricts: %s (%d features, %d zone districts)\n\n",
		metroSrc, districtsSrc, len(all), len(zoneDistricts))

	lineZones, stations, err := buildLines(metro, all)
	if err != nil {
		return err
	}
	districtZones, err := buildDistricts(zoneDistricts, stations, decls)
	if err != nil {
		return err
	}
	stationZones, err := buildStationZones(stations)
	if err != nil {
		return err
	}

	// Bump on every meaningful data change: the version is the /zones ETag, and
	// a stale one would 304 old data into caches for a day.
	f := outFile{Version: dirVersion, Stations: stations}
	f.Zones = append(f.Zones, lineZones...)
	f.Zones = append(f.Zones, districtZones...)
	f.Zones = append(f.Zones, stationZones...)
	if err := finalize(&f); err != nil {
		return err
	}
	reportOrphanStations(f.Zones, stations)

	raw, err := marshal(f)
	if err != nil {
		return err
	}
	// Never ship a file the service would refuse to boot on: validate with the
	// very code that will load it in production.
	if _, err := zones.New(raw); err != nil {
		return fmt.Errorf("generated file fails zones.New: %w", err)
	}
	if err := os.WriteFile(out, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}

	fmt.Printf("\nwrote %s: %d stations, %d zones, %d bytes\n", out, len(f.Stations), len(f.Zones), len(raw))
	return nil
}

// buildLines turns source lines into line zones and the global station list.
// МЦД stations outside the city are dropped here: the diameters run to Lobnya
// and Ramenskoye, and a Moscow flatmate pool has no business there (§1).
func buildLines(metro *hhMetro, moscow []district) ([]zones.Zone, []zones.Station, error) {
	sorted := append([]hhLine(nil), metro.Lines...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var (
		unmapped   []string
		lineZones  []zones.Zone
		stations   []zones.Station
		byID       = map[string]bool{}
		dropped    []string
		dupSkipped []string
		fixed      []string
		unlinked   []string
	)
	for _, l := range sorted {
		if why, skip := skippedLines[l.Name]; skip {
			fmt.Printf("line skipped: %-28s %s\n", l.Name, why)
			continue
		}
		meta, ok := linesByOfficialName[l.Name]
		if !ok {
			unmapped = append(unmapped, fmt.Sprintf("%q (hh id %s, %d stations)", l.Name, l.ID, len(l.Stations)))
			continue
		}
		prefix := strings.TrimPrefix(meta.Slug, "line-")
		filterOutside := mcdLineSlugs[meta.Slug]

		var ids []string
		for _, s := range l.Stations {
			// The feed has shipped «Бутырская » with a trailing space; that
			// would reach the user as «у метро Бутырская ».
			s.Name = strings.TrimSpace(s.Name)
			if canonical, ok := stationAliases[s.Name]; ok {
				s.Name = canonical
			}
			key := stationKey{Line: l.Name, Station: s.Name}

			if why, skip := stationSkips[key]; skip {
				unlinked = append(unlinked, fmt.Sprintf("%s · %s — %s", meta.Name, s.Name, why))
				continue
			}
			if fix, ok := stationOverrides[key]; ok {
				fixed = append(fixed, fmt.Sprintf("%s · %s: %.6f,%.6f → %.6f,%.6f (%s)",
					meta.Name, s.Name, s.Lat, s.Lng, fix.Lat, fix.Lng, fix.Why))
				s.Lat, s.Lng = fix.Lat, fix.Lng
			}
			if filterOutside && !insideAny(moscow, point{s.Lng, s.Lat}) {
				dropped = append(dropped, fmt.Sprintf("%s · %s", meta.Name, s.Name))
				continue
			}
			id := fmt.Sprintf("st:%s:%s", prefix, translit(s.Name))
			if byID[id] {
				dupSkipped = append(dupSkipped, id)
				continue
			}
			byID[id] = true
			stations = append(stations, zones.Station{ID: id, Name: s.Name, Lat: s.Lat, Lng: s.Lng})
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			fmt.Printf("line skipped: %-28s no stations left inside Moscow\n", l.Name)
			continue
		}
		lineZones = append(lineZones, zones.Zone{
			ID: meta.Slug, Kind: zones.KindLine, Name: meta.Name,
			NameIn: meta.NameIn, NameGen: meta.NameGen, Stations: ids,
		})
	}
	if len(unmapped) > 0 {
		return nil, nil, fmt.Errorf("unmapped lines — add them to linesByOfficialName or skippedLines: %s",
			strings.Join(unmapped, ", "))
	}
	if err := checkCoordinateCollisions(stations); err != nil {
		return nil, nil, err
	}

	fmt.Printf("\nlines: %d zones from %d source lines\n", len(lineZones), len(metro.Lines))
	for _, f := range fixed {
		fmt.Printf("  coordinates corrected: %s\n", f)
	}
	for _, u := range unlinked {
		fmt.Printf("  station dropped from line: %s\n", u)
	}
	fmt.Printf("stations: %d kept, %d МЦД platforms dropped as outside Moscow\n", len(stations), len(dropped))
	for _, d := range dropped {
		fmt.Printf("  outside Moscow: %s\n", d)
	}
	for _, id := range dupSkipped {
		fmt.Printf("  duplicate station id on one line, second ignored: %s\n", id)
	}
	return lineZones, stations, nil
}

// checkCoordinateCollisions is the guard that would have caught hh shipping
// «Покровское» at the coordinates of «Покровское-Стрешнево». Two platforms of
// the SAME station legitimately share a point — two different stations never do,
// and such a pair means the feed copied a row.
func checkCoordinateCollisions(stations []zones.Station) error {
	atPoint := map[string][]zones.Station{}
	for _, s := range stations {
		atPoint[fmt.Sprintf("%.6f,%.6f", s.Lat, s.Lng)] = append(atPoint[fmt.Sprintf("%.6f,%.6f", s.Lat, s.Lng)], s)
	}
	var bad []string
	for pt, group := range atPoint {
		names := map[string]bool{}
		for _, s := range group {
			names[normalizeName(s.Name)] = true
		}
		if len(names) < 2 {
			continue
		}
		var list []string
		for _, s := range group {
			list = append(list, fmt.Sprintf("%s (%s)", s.Name, s.ID))
		}
		sort.Strings(list)
		bad = append(bad, fmt.Sprintf("%s: %s", pt, strings.Join(list, " / ")))
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("different stations share exact coordinates — the source copied a row, "+
		"add the real position to stationOverrides:\n  %s", strings.Join(bad, "\n  "))
}

// buildDistricts assigns stations by geometry: inside the polygon, or within
// boundaryBufferM of its border. Districts the metro missed get the nearest
// few — an empty zone would be rejected by the engine and would be a dead end
// in the UI besides.
func buildDistricts(ds []district, stations []zones.Station, decls map[string]declension) ([]zones.Zone, error) {
	var missing []string
	for _, d := range ds {
		if _, ok := decls[d.Name]; !ok {
			missing = append(missing, d.Name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("declensions.json is missing %d districts — add them by hand: %s",
			len(missing), strings.Join(missing, ", "))
	}

	var (
		out       []zones.Zone
		metroless [][2]string // district name, assigned stations
		slugs     = map[string]string{}
	)
	for _, d := range ds {
		ids := stationsForDistrict(d, stations)
		note := ""
		if len(ids) == 0 {
			ids = nearestStations(d, stations, nearestStationsForMetroless)
			note = strings.Join(stationNames(ids, stations), ", ")
			metroless = append(metroless, [2]string{d.Name, note})
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("district %q got no stations at all", d.Name)
		}
		id := "d-" + translit(d.Name)
		if prev, dup := slugs[id]; dup {
			return nil, fmt.Errorf("district slug collision %q: %q and %q", id, prev, d.Name)
		}
		slugs[id] = d.Name
		dec := decls[d.Name]
		out = append(out, zones.Zone{
			ID: id, Kind: zones.KindDistrict, Name: d.Name,
			NameIn: dec.In, NameGen: dec.Gen, Stations: ids,
		})
	}

	fmt.Printf("\ndistricts: %d zones, %d of them without their own metro\n", len(out), len(metroless))
	for _, m := range metroless {
		fmt.Printf("  metro-less: %-26s → %s\n", m[0], m[1])
	}
	return out, nil
}

// buildStationZones groups same-named platforms — «Арбатская» on the blue line
// and «Арбатская» on the light-blue one are one place to a person looking for a
// flat, so they are one zone (§2.1).
func buildStationZones(stations []zones.Station) ([]zones.Zone, error) {
	groups := map[string][]zones.Station{}
	var order []string
	for _, s := range stations {
		key := normalizeName(s.Name)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], s)
	}
	sort.Strings(order)

	var (
		out   []zones.Zone
		slugs = map[string]string{}
	)
	var split []string
	for _, key := range order {
		g := groups[key]
		id := "st-" + translit(key)
		if prev, dup := slugs[id]; dup {
			return nil, fmt.Errorf("station slug collision %q: %q and %q", id, prev, key)
		}
		slugs[id] = key

		if maxSpreadM(g) > stationSplitM {
			base, rail := splitByRail(g)
			if len(base) == 0 || len(rail) == 0 ||
				maxSpreadM(base) > stationSplitM || maxSpreadM(rail) > stationSplitM {
				return nil, fmt.Errorf("station group %q spreads %.0f m and МЦД does not explain it — decide by hand",
					key, maxSpreadM(g))
			}
			out = append(out, stationZone(id, displayName(base), "у метро ", "метро ", base))
			mcdName := displayName(rail) + " (МЦД)"
			out = append(out, stationZone(id+"-mcd", mcdName, "у станции ", "станции ", rail))
			split = append(split, fmt.Sprintf("%s (%.0f m apart)", key, maxSpreadM(g)))
			continue
		}

		out = append(out, stationZone(id, displayName(g), "у метро ", "метро ", g))
	}
	fmt.Printf("stations: %d station zones from %d platforms\n", len(out), len(stations))
	if len(split) > 0 {
		fmt.Printf("stations: split off МЦД zones: %s\n", strings.Join(split, "; "))
	}
	return out, nil
}

func stationZone(id, name, in, gen string, g []zones.Station) zones.Zone {
	ids := make([]string, 0, len(g))
	for _, s := range g {
		ids = append(ids, s.ID)
	}
	return zones.Zone{
		ID: id, Kind: zones.KindStation, Name: name,
		NameIn: in + name, NameGen: gen + name, Stations: ids,
	}
}

func maxSpreadM(g []zones.Station) float64 {
	spread := 0.0
	for i := range g {
		for j := i + 1; j < len(g); j++ {
			d := metersBetween(point{g[i].Lng, g[i].Lat}, point{g[j].Lng, g[j].Lat})
			if d > spread {
				spread = d
			}
		}
	}
	return spread
}

// splitByRail parts a group into metro platforms and МЦД ones — the only kind
// of same-name pair that sits kilometres apart in practice.
func splitByRail(g []zones.Station) (base, rail []zones.Station) {
	for _, s := range g {
		parts := strings.SplitN(s.ID, ":", 3)
		if len(parts) == 3 && strings.HasPrefix(parts[1], "d") {
			rail = append(rail, s)
		} else {
			base = append(base, s)
		}
	}
	return base, rail
}

// finalize makes the file byte-stable across runs and removes the one thing the
// engine tolerates but should never be shipped: a station listed twice in a zone.
func finalize(f *outFile) error {
	sort.Slice(f.Stations, func(i, j int) bool { return f.Stations[i].ID < f.Stations[j].ID })
	sort.SliceStable(f.Zones, func(i, j int) bool {
		if f.Zones[i].Kind != f.Zones[j].Kind {
			return f.Zones[i].Kind < f.Zones[j].Kind
		}
		if f.Zones[i].Name != f.Zones[j].Name {
			return f.Zones[i].Name < f.Zones[j].Name
		}
		return f.Zones[i].ID < f.Zones[j].ID
	})

	byKind := map[zones.Kind]int{}
	seen := map[string]bool{}
	for i := range f.Zones {
		z := &f.Zones[i]
		if seen[z.ID] {
			return fmt.Errorf("duplicate zone id %q", z.ID)
		}
		seen[z.ID] = true
		byKind[z.Kind]++

		uniq := make([]string, 0, len(z.Stations))
		in := map[string]bool{}
		for _, id := range z.Stations {
			if in[id] {
				continue
			}
			in[id] = true
			uniq = append(uniq, id)
		}
		sort.Strings(uniq)
		z.Stations = uniq
	}
	fmt.Printf("\nzones: %d district, %d line, %d station (total %d)\n",
		byKind[zones.KindDistrict], byKind[zones.KindLine], byKind[zones.KindStation], len(f.Zones))
	return nil
}

func reportOrphanStations(zs []zones.Zone, stations []zones.Station) {
	inDistrict := map[string]bool{}
	for _, z := range zs {
		if z.Kind != zones.KindDistrict {
			continue
		}
		for _, id := range z.Stations {
			inDistrict[id] = true
		}
	}
	var orphans []string
	for _, s := range stations {
		if !inDistrict[s.ID] {
			orphans = append(orphans, fmt.Sprintf("%s (%s)", s.Name, s.ID))
		}
	}
	sort.Strings(orphans)
	fmt.Printf("stations in no district: %d\n", len(orphans))
	for _, o := range orphans {
		fmt.Printf("  orphan: %s\n", o)
	}
}

func loadMetro(src string) (*hhMetro, error) {
	raw, err := read(src)
	if err != nil {
		return nil, fmt.Errorf("metro source: %w", err)
	}
	var m hhMetro
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse metro source: %w", err)
	}
	if len(m.Lines) == 0 {
		return nil, fmt.Errorf("metro source has no lines")
	}
	// Trim at the door: a stray space in a line name would silently miss the
	// linesByOfficialName lookup and fail generation for the wrong reason.
	for i := range m.Lines {
		m.Lines[i].Name = strings.TrimSpace(m.Lines[i].Name)
	}
	return &m, nil
}

// loadDistricts returns every feature (the union of which is the city boundary,
// used to keep МЦД honest) and, separately, the ones that become zones.
func loadDistricts(src string) (all, zoneDistricts []district, err error) {
	raw, err := read(src)
	if err != nil {
		return nil, nil, fmt.Errorf("districts source: %w (download it first: curl -sL https://gis-lab.info/data/mos-adm/mo.geojson -o /tmp/mo.geojson)", err)
	}
	var g geoFile
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, nil, fmt.Errorf("parse districts source: %w", err)
	}
	if len(g.Features) == 0 {
		return nil, nil, fmt.Errorf("districts source has no features")
	}

	for _, feat := range g.Features {
		rings, err := outerRings(feat.Geometry.Type, feat.Geometry.Coordinates)
		if err != nil {
			return nil, nil, fmt.Errorf("district %q: %w", feat.Properties.Name, err)
		}
		if len(rings) == 0 {
			return nil, nil, fmt.Errorf("district %q: no rings", feat.Properties.Name)
		}
		d := district{Name: feat.Properties.Name, OkrugAO: feat.Properties.NameAO, Rings: rings}
		d.MinLng, d.MinLat = math.Inf(1), math.Inf(1)
		d.MaxLng, d.MaxLat = math.Inf(-1), math.Inf(-1)
		for _, r := range rings {
			for _, p := range r {
				d.MinLng, d.MaxLng = math.Min(d.MinLng, p[0]), math.Max(d.MaxLng, p[0])
				d.MinLat, d.MaxLat = math.Min(d.MinLat, p[1]), math.Max(d.MaxLat, p[1])
			}
		}
		all = append(all, d)
		if d.OkrugAO != okrugTroitsky && d.OkrugAO != okrugNovomoskovsky {
			zoneDistricts = append(zoneDistricts, d)
		}
	}
	sort.Slice(zoneDistricts, func(i, j int) bool { return zoneDistricts[i].Name < zoneDistricts[j].Name })
	return all, zoneDistricts, nil
}

func loadDeclensions(path string) (map[string]declension, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("declensions: %w", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse declensions: %w", err)
	}
	out := make(map[string]declension, len(m))
	for k, v := range m {
		if strings.HasPrefix(k, "_") {
			continue // comment key
		}
		var d declension
		if err := json.Unmarshal(v, &d); err != nil {
			return nil, fmt.Errorf("declensions[%q]: %w", k, err)
		}
		if d.In == "" || d.Gen == "" {
			return nil, fmt.Errorf("declensions[%q]: empty in/gen", k)
		}
		out[k] = d
	}
	return out, nil
}

func read(src string) ([]byte, error) {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return os.ReadFile(src)
	}
	client := &http.Client{Timeout: fetchTimeout}
	resp, err := client.Get(src)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", src, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// outerRings keeps only the exterior ring of each polygon; holes are irrelevant
// for "which district is this station in".
func outerRings(kind string, coords json.RawMessage) ([][]point, error) {
	switch kind {
	case "Polygon":
		var poly [][]point
		if err := json.Unmarshal(coords, &poly); err != nil {
			return nil, err
		}
		if len(poly) == 0 {
			return nil, nil
		}
		return [][]point{poly[0]}, nil
	case "MultiPolygon":
		var multi [][][]point
		if err := json.Unmarshal(coords, &multi); err != nil {
			return nil, err
		}
		var out [][]point
		for _, poly := range multi {
			if len(poly) > 0 {
				out = append(out, poly[0])
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported geometry %q", kind)
	}
}

func stationsForDistrict(d district, stations []zones.Station) []string {
	var out []string
	for _, s := range stations {
		p := point{s.Lng, s.Lat}
		if !inBBox(d, p, boundaryBufferM) {
			continue
		}
		if insideRings(d.Rings, p) || distanceToBoundaryM(d.Rings, p) <= boundaryBufferM {
			out = append(out, s.ID)
		}
	}
	return out
}

// nearestStations picks the n closest station *places* to the district centre,
// not platforms: counting «Новогиреево» twice because the metro and the МЦД
// stop there would spend a slot on nothing. Every platform of a chosen place is
// taken, so the district intersects each line running through it.
func nearestStations(d district, stations []zones.Station, n int) []string {
	c := centroid(d.Rings)
	idx := make([]int, len(stations))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		da := metersBetween(c, point{stations[idx[a]].Lng, stations[idx[a]].Lat})
		db := metersBetween(c, point{stations[idx[b]].Lng, stations[idx[b]].Lat})
		if da != db {
			return da < db
		}
		return stations[idx[a]].ID < stations[idx[b]].ID
	})

	picked := map[string]bool{}
	for _, i := range idx {
		if len(picked) == n {
			break
		}
		picked[normalizeName(stations[i].Name)] = true
	}
	var out []string
	for _, s := range stations {
		if picked[normalizeName(s.Name)] {
			out = append(out, s.ID)
		}
	}
	return out
}

func stationNames(ids []string, stations []zones.Station) []string {
	byID := make(map[string]string, len(stations))
	for _, s := range stations {
		byID[s.ID] = s.Name
	}
	var (
		out  []string
		seen = map[string]bool{}
	)
	for _, id := range ids {
		key := normalizeName(byID[id])
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, byID[id])
	}
	return out
}

// displayName picks the spelling to show for a group of same-named platforms,
// preferring the one that keeps ё («Тёплый Стан», not «Теплый Стан»).
func displayName(g []zones.Station) string {
	names := make([]string, 0, len(g))
	for _, s := range g {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	for _, n := range names {
		if strings.ContainsRune(n, 'ё') || strings.ContainsRune(n, 'Ё') {
			return n
		}
	}
	return names[0]
}

func insideAny(ds []district, p point) bool {
	for _, d := range ds {
		if inBBox(d, p, 0) && insideRings(d.Rings, p) {
			return true
		}
	}
	return false
}

func inBBox(d district, p point, bufferM float64) bool {
	dLat := bufferM / metersPerDegreeLat
	dLng := bufferM / (metersPerDegreeLng * math.Cos(p[1]*math.Pi/180))
	return p[0] >= d.MinLng-dLng && p[0] <= d.MaxLng+dLng &&
		p[1] >= d.MinLat-dLat && p[1] <= d.MaxLat+dLat
}

func insideRings(rings [][]point, p point) bool {
	for _, r := range rings {
		if rayCast(r, p) {
			return true
		}
	}
	return false
}

// rayCast is the standard even-odd test: count crossings of a ray going east.
func rayCast(ring []point, p point) bool {
	inside := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		yi, yj := ring[i][1], ring[j][1]
		if (yi > p[1]) == (yj > p[1]) {
			continue
		}
		x := ring[j][0] + (p[1]-yj)/(yi-yj)*(ring[i][0]-ring[j][0])
		if p[0] < x {
			inside = !inside
		}
	}
	return inside
}

func distanceToBoundaryM(rings [][]point, p point) float64 {
	best := math.Inf(1)
	for _, r := range rings {
		for i, j := 0, len(r)-1; i < len(r); j, i = i, i+1 {
			if d := pointSegmentM(p, r[j], r[i]); d < best {
				best = d
			}
		}
	}
	return best
}

// pointSegmentM measures in metres through a local equirectangular projection —
// exact enough at Moscow's scale, and it keeps the tool dependency-free.
func pointSegmentM(p, a, b point) float64 {
	k := math.Cos(p[1] * math.Pi / 180)
	px, py := 0.0, 0.0
	ax, ay := (a[0]-p[0])*metersPerDegreeLng*k, (a[1]-p[1])*metersPerDegreeLat
	bx, by := (b[0]-p[0])*metersPerDegreeLng*k, (b[1]-p[1])*metersPerDegreeLat

	dx, dy := bx-ax, by-ay
	if dx == 0 && dy == 0 {
		return math.Hypot(ax-px, ay-py)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(ax+t*dx-px, ay+t*dy-py)
}

func metersBetween(a, b point) float64 {
	k := math.Cos((a[1] + b[1]) / 2 * math.Pi / 180)
	return math.Hypot((b[0]-a[0])*metersPerDegreeLng*k, (b[1]-a[1])*metersPerDegreeLat)
}

// centroid is the area centroid of the largest ring, so a district with a
// detached sliver is measured from its actual body.
func centroid(rings [][]point) point {
	var (
		best     []point
		bestArea float64
	)
	for _, r := range rings {
		if a := math.Abs(ringArea(r)); a > bestArea {
			best, bestArea = r, a
		}
	}
	if len(best) == 0 {
		return point{}
	}
	area := ringArea(best)
	if area == 0 {
		var sx, sy float64
		for _, p := range best {
			sx, sy = sx+p[0], sy+p[1]
		}
		return point{sx / float64(len(best)), sy / float64(len(best))}
	}
	var cx, cy float64
	for i, j := 0, len(best)-1; i < len(best); j, i = i, i+1 {
		cross := best[j][0]*best[i][1] - best[i][0]*best[j][1]
		cx += (best[j][0] + best[i][0]) * cross
		cy += (best[j][1] + best[i][1]) * cross
	}
	return point{cx / (6 * area), cy / (6 * area)}
}

func ringArea(r []point) float64 {
	var a float64
	for i, j := 0, len(r)-1; i < len(r); j, i = i, i+1 {
		a += r[j][0]*r[i][1] - r[i][0]*r[j][1]
	}
	return a / 2
}

// normalizeName folds the two spellings of the same place into one key: case
// and ё, nothing else.
func normalizeName(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "ё", "е")
}

func translit(s string) string {
	var b strings.Builder
	prevDash := true // leading dashes are trimmed by never emitting one first
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if v, ok := translitTable[r]; ok {
				if v != "" {
					b.WriteString(v)
					prevDash = false
				}
				continue
			}
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func marshal(f outFile) ([]byte, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}
