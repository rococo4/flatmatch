package usersapi

import (
	"errors"
	"net/http"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/service/users"
)

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, entity.ErrNotFound) {
		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such user", err)
		return
	}

	if errors.Is(err, users.ErrUnsupportedImage) {
		httpx.BadRequest(w, r, httpx.CodeUnsupportedMedia, "photo must be jpeg, png or webp", nil, err)
		return
	}

	var fields *entity.FieldsError
	if !errors.As(err, &fields) {
		httpx.Internal(w, r, err)
		return
	}

	switch {
	case errors.Is(err, users.ErrPhotoLimit):
		httpx.BadRequest(w, r, httpx.CodePhotoLimit, "profile holds five photos already", fields.Fields, err)
	case errors.Is(err, users.ErrLastPhoto):
		httpx.BadRequest(w, r, httpx.CodeLastPhoto, "a profile cannot be left without a photo", fields.Fields, err)
	case errors.Is(err, users.ErrInvalidPhotoIndex):
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "no photo under that index", fields.Fields, err)
	case errors.Is(err, users.ErrInvalidPhotoOrder):
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "order must be a permutation of the photo set", fields.Fields, err)
	case errors.Is(err, users.ErrInvalidPatch):
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "value outside the allowed set", fields.Fields, err)
	case errors.Is(err, users.ErrInvalidSource):
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "source must match ^[a-z0-9_]{1,32}$", fields.Fields, err)
	case errors.Is(err, users.ErrIncomplete):
		httpx.BadRequest(w, r, httpx.CodeProfileIncomplete, "profile is not filled in", fields.Fields, err)
	default:
		httpx.Internal(w, r, err)
	}
}
