package zonesapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/service/pools"
)

const (
	defaultTopLimit = 5
	maxTopLimit     = 20
)

type ZoneDir interface {
	Raw() []byte
	Version() int
}

type Tops interface {
	Top(ctx context.Context, viewerID uuid.UUID, limit int) ([]pools.ZoneTop, error)
}

type Handler struct {
	dir  ZoneDir
	tops Tops
}

var (
	errNoUserInContext = errors.New("no authenticated user in request context")
	errBadLimit        = errors.New("limit is not a positive number")
)

func New(dir ZoneDir, tops Tops) *Handler {
	return &Handler{dir: dir, tops: tops}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.directory)
	r.Get("/top", h.top)
	return r
}

func (h *Handler) directory(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) top(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "value outside the allowed set",
			[]string{"limit"}, err)
		return
	}

	top, err := h.tops.Top(r.Context(), user.ID, limit)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, topResponse{Zones: topZones(top)})
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return defaultTopLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%w: %q", errBadLimit, raw)
	}
	return min(n, maxTopLimit), nil
}
