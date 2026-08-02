package usersapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
)

var errNoUserInContext = errors.New("no authenticated user in request context")

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/me", h.me)
	return r
}

type profile struct {
	ID          string             `json:"id"`
	TelegramID  int64              `json:"tg_id"`
	Name        string             `json:"name"`
	Username    string             `json:"tg_username,omitempty"`
	PhotoURL    string             `json:"photo_url,omitempty"`
	Status      entity.Status      `json:"status"`
	PhotoStatus entity.PhotoStatus `json:"photo_status"`
	CreatedAt   time.Time          `json:"created_at"`
}

func toProfile(u entity.User) profile {
	return profile{
		ID:          u.ID.String(),
		TelegramID:  u.TelegramID,
		Name:        u.Name,
		Username:    u.Username,
		PhotoURL:    u.PhotoURL,
		Status:      u.Status,
		PhotoStatus: u.PhotoStatus,
		CreatedAt:   u.CreatedAt,
	}
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toProfile(user))
}
