package zones

import (
	"os"
	"slices"
	"testing"
)

func miniDirectory(t *testing.T) *Directory {
	t.Helper()
	raw, err := os.ReadFile("testdata/mini.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	d, err := New(raw)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return d
}

func TestDirectoryGet(t *testing.T) {
	d := miniDirectory(t)

	z, ok := d.Get("d-khamovniki")
	if !ok {
		t.Fatal("d-khamovniki not found")
	}
	if z.NameIn != "в Хамовниках" || z.Kind != KindDistrict {
		t.Errorf("unexpected zone: %+v", z)
	}
	if _, ok := d.Get("d-nowhere"); ok {
		t.Error("unknown id must not be found")
	}
}

func TestDirectoryIntersecting(t *testing.T) {
	d := miniDirectory(t)

	want := map[string][]string{
		"line-1":       {"d-khamovniki", "line-1"},
		"line-2":       {"d-khamovniki", "line-2", "st-sokol"},
		"d-khamovniki": {"d-khamovniki", "line-1", "line-2", "st-sokol"},
		"st-sokol":     {"d-khamovniki", "line-2", "st-sokol"},
	}
	for id, exp := range want {
		if got := d.Intersecting(id); !slices.Equal(got, exp) {
			t.Errorf("Intersecting(%q) = %v, want %v (sorted, self included)", id, got, exp)
		}
	}

	if slices.Contains(d.Intersecting("line-1"), "line-2") {
		t.Error("line-1 and line-2 share no station, must not intersect")
	}

	for _, z := range d.All() {
		for _, other := range d.Intersecting(z.ID) {
			if !slices.Contains(d.Intersecting(other), z.ID) {
				t.Errorf("%q intersects %q, but not the other way round", z.ID, other)
			}
		}
	}
}

func TestDirectoryRejectsBrokenFiles(t *testing.T) {
	tests := []struct{ name, raw string }{
		{"unknown station ref", `{"version":1,"stations":[],"zones":[
			{"id":"z","kind":"line","name":"x","name_in":"x","name_gen":"x","stations":["ghost"]}]}`},
		{"duplicate zone id", `{"version":1,"stations":[{"id":"s","name":"S"}],"zones":[
			{"id":"z","kind":"line","name":"x","name_in":"x","name_gen":"x","stations":["s"]},
			{"id":"z","kind":"line","name":"y","name_in":"y","name_gen":"y","stations":["s"]}]}`},
		{"empty stations", `{"version":1,"stations":[{"id":"s","name":"S"}],"zones":[
			{"id":"z","kind":"line","name":"x","name_in":"x","name_gen":"x","stations":[]}]}`},
		{"missing declension", `{"version":1,"stations":[{"id":"s","name":"S"}],"zones":[
			{"id":"z","kind":"line","name":"x","name_in":"","name_gen":"x","stations":["s"]}]}`},
		{"bad kind", `{"version":1,"stations":[{"id":"s","name":"S"}],"zones":[
			{"id":"z","kind":"galaxy","name":"x","name_in":"x","name_gen":"x","stations":["s"]}]}`},
		{"zero version", `{"version":0,"stations":[],"zones":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New([]byte(tt.raw)); err == nil {
				t.Error("New() accepted a broken file")
			}
		})
	}
}
