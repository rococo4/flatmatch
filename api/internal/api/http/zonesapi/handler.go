// Package zonesapi serves the zone directory: the embedded file, byte for
// byte. The picker searches client-side (§6 — справочник конечный и
// версионируемый, ~450 зон грузятся один раз).
package zonesapi

import (
	"fmt"
	"net/http"
)

// ZoneDir is the directory as this handler sees it: bytes and a version to
// hang an ETag on.
type ZoneDir interface {
	Raw() []byte
	Version() int
}

type Handler struct {
	dir ZoneDir
}

func New(dir ZoneDir) *Handler {
	return &Handler{dir: dir}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The directory is versioned in the repository, so the version alone
	// identifies the payload: a new file means a new version means a new ETag.
	etag := fmt.Sprintf(`"zones-v%d"`, h.dir.Version())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=86400")

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(h.dir.Raw())
}
