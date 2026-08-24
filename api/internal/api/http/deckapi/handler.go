package deckapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/service/deck"
)

type Deck interface {
	Deck(ctx context.Context, viewerID uuid.UUID, q deck.Query) (deck.Deck, error)
}

type Photos interface {
	URL(key string) string
}

type Handler struct {
	deck   Deck
	photos Photos
}

var (
	errNoUserInContext = errors.New("no authenticated user in request context")
	errUnknownValue    = errors.New("filter value is outside the allowed set")
)

func New(d Deck, photos Photos) *Handler {
	return &Handler{deck: d, photos: photos}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.cards)
	return r
}

func (h *Handler) cards(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	query, bad, ok := parseQuery(r.URL.Query())
	if !ok {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "unknown value of "+bad,
			[]string{bad}, errUnknownValue)
		return
	}

	got, err := h.deck.Deck(r.Context(), user.ID, query)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toResponse(got, h.photos))
}
