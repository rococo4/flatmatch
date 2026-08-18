package zonesapi

import (
	"fmt"
	"net/http"
)

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
