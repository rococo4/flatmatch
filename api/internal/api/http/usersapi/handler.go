package usersapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
)

const maxPhotoBytes = 5 << 20

type Accounts interface {
	UpdateProfile(ctx context.Context, id uuid.UUID, p entity.ProfilePatch) (entity.User, error)
	AddPhoto(ctx context.Context, id uuid.UUID, file io.ReadSeeker) (entity.User, error)
	RemovePhoto(ctx context.Context, id uuid.UUID, index int) (entity.User, error)
	ReorderPhotos(ctx context.Context, id uuid.UUID, order []int) (entity.User, error)
	Submit(ctx context.Context, id uuid.UUID) (entity.User, error)
	SetSource(ctx context.Context, id uuid.UUID, raw string) error
	OwnPhotoURLs(u entity.User) []string
}

type Bot interface {
	Username() string
}

type Handler struct {
	accounts Accounts
	bot      Bot
}

var errNoUserInContext = errors.New("no authenticated user in request context")

func New(accounts Accounts, bot Bot) *Handler {
	return &Handler{accounts: accounts, bot: bot}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/me", h.me)
	r.Patch("/me/profile", h.updateProfile)
	r.Post("/me/photos", h.addPhoto)
	r.Delete("/me/photos/{index}", h.removePhoto)
	r.Put("/me/photos/order", h.reorderPhotos)
	r.Post("/me/submit", h.submit)
	r.Post("/me/source", h.setSource)
	return r
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	h.respondSelf(w, r, user)
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	var body patchRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))

	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		httpx.BadRequest(w, r, httpx.CodeBadRequest, "cannot read the request body", nil, err)
		return
	}

	updated, err := h.accounts.UpdateProfile(r.Context(), user.ID, body.toPatch())
	if err != nil {
		handleError(w, r, err)
		return
	}
	h.respondSelf(w, r, updated)
}

func (h *Handler) addPhoto(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+64<<10)

	file, _, err := r.FormFile("photo")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.BadRequest(w, r, httpx.CodeTooLarge, "photo is larger than 5 MB", nil, err)
			return
		}
		httpx.BadRequest(w, r, httpx.CodeBadRequest, "no file in the photo field", nil, err)
		return
	}
	defer file.Close()

	updated, err := h.accounts.AddPhoto(r.Context(), user.ID, file)
	if err != nil {
		handleError(w, r, err)
		return
	}
	h.respondSelf(w, r, updated)
}

func (h *Handler) removePhoto(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	index, err := strconv.Atoi(chi.URLParam(r, "index"))
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "index must be a number",
			[]string{"index"}, err)
		return
	}

	updated, err := h.accounts.RemovePhoto(r.Context(), user.ID, index)
	if err != nil {
		handleError(w, r, err)
		return
	}
	h.respondSelf(w, r, updated)
}

func (h *Handler) reorderPhotos(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	var body orderRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))

	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		httpx.BadRequest(w, r, httpx.CodeBadRequest, "cannot read the request body", nil, err)
		return
	}

	updated, err := h.accounts.ReorderPhotos(r.Context(), user.ID, body.Order)
	if err != nil {
		handleError(w, r, err)
		return
	}
	h.respondSelf(w, r, updated)
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	updated, err := h.accounts.Submit(r.Context(), user.ID)
	if err != nil {
		handleError(w, r, err)
		return
	}
	h.respondSelf(w, r, updated)
}

func (h *Handler) setSource(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	var body sourceRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))

	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		httpx.BadRequest(w, r, httpx.CodeBadRequest, "cannot read the request body", nil, err)
		return
	}

	if err := h.accounts.SetSource(r.Context(), user.ID, body.Source); err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusNoContent, nil)
}

func (h *Handler) respondSelf(w http.ResponseWriter, r *http.Request, u entity.User) {
	httpx.JSON(w, r, http.StatusOK, toOwnProfile(u, h.accounts.OwnPhotoURLs(u), h.bot.Username()))
}
