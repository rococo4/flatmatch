package main

import (
	"log/slog"
	"net/http"
	"slices"
	"testing"

	"github.com/go-chi/chi/v5"

	"flatmatch/api/internal/photostore"
)

func routePaths(t *testing.T) []string {
	t.Helper()

	photos, err := photostore.NewDir(t.TempDir(), mediaPrefix)
	if err != nil {
		t.Fatalf("photo directory: %v", err)
	}

	var paths []string
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		paths = append(paths, method+" "+route)
		return nil
	}
	handler := router(slog.New(slog.DiscardHandler), nil, nil, deps{photos: photos})
	if err := chi.Walk(handler.(*chi.Mux), walk); err != nil {
		t.Fatalf("walk the router: %v", err)
	}
	return paths
}

func TestMarketFlatRoutesAreGone(t *testing.T) {
	paths := routePaths(t)

	gone := []string{
		"GET /api/v1/feed",
		"GET /api/v1/pools/zone/{zoneID}",
		"POST /api/v1/points/{id}/enter",
		"DELETE /api/v1/points/{id}/enter",
		"POST /api/v1/points/{id}/vote",
		"POST /api/v1/points/{id}/restore",
		"GET /api/v1/points/{id}/pool",
	}
	for _, route := range gone {
		if slices.Contains(paths, route) {
			t.Errorf("router still serves %s", route)
		}
	}
}

func TestRoomAndDeckRoutesStay(t *testing.T) {
	paths := routePaths(t)

	kept := []string{
		"GET /api/v1/deck/",
		"GET /api/v1/zones/top",
		"POST /api/v1/points/",
		"GET /api/v1/points/mine",
		"GET /api/v1/points/{id}",
		"POST /api/v1/points/{id}/close",
		"POST /api/v1/points/{id}/pass",
	}
	for _, route := range kept {
		if !slices.Contains(paths, route) {
			t.Errorf("router lost %s; it serves %v", route, paths)
		}
	}
}
