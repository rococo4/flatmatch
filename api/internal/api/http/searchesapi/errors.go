package searchesapi

import (
	"errors"
	"net/http"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
)

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	var fields *entity.FieldsError
	switch {
	case errors.Is(err, entity.ErrNotFound):
		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such search", err)
	case errors.Is(err, entity.ErrSearchLimit):

		httpx.BadRequest(w, r, httpx.CodeSearchLimit, "поиски заняты 3 из 3", nil, err)
	case errors.Is(err, entity.ErrSearchClosed):
		httpx.BadRequest(w, r, httpx.CodeSearchClosed, "search is closed, reactivate it first", nil, err)
	case errors.As(err, &fields):
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "value outside the allowed set", fields.Fields, err)
	default:
		httpx.Internal(w, r, err)
	}
}
