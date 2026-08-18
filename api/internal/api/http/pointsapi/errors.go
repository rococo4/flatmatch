package pointsapi

import (
	"errors"
	"fmt"
	"net/http"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/logctx"
	"flatmatch/api/internal/service/points"
)

const (
	duplicateActiveTemplate = "Эта квартира уже есть, в пуле %d — войти?"
	duplicatePendingMessage = "Эта квартира уже добавлена и ждёт проверки"
	flatLimitMessage        = "Пять квартир уже добавлено — закрой одну в Моих поисках"
	intentHomeLimitMessage  = "Ты уже зовёшь к себе — активных точек может быть две"
	needEnterMessage        = "Пул откроется после входа"
)

func handleFormError(w http.ResponseWriter, r *http.Request, kind entity.PointType, err error) {
	var duplicate *points.ErrDuplicateURL
	switch {
	case errors.As(err, &duplicate):
		duplicateFound(w, r, duplicate)
	case errors.Is(err, entity.ErrPointLimit):
		httpx.BadRequest(w, r, httpx.CodePointLimit, limitMessage(kind), nil, err)
	default:
		handleError(w, r, err)
	}
}

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	var fields *entity.FieldsError
	switch {
	case errors.Is(err, entity.ErrNeedEnter):
		httpx.Error(w, r, http.StatusForbidden, httpx.CodeNeedEnter, needEnterMessage, err)
	case errors.Is(err, entity.ErrNotFound):

		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such point", err)
	case errors.As(err, &fields):
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "value outside the allowed set", fields.Fields, err)
	default:
		httpx.Internal(w, r, err)
	}
}

func duplicateFound(w http.ResponseWriter, r *http.Request, dup *points.ErrDuplicateURL) {
	message := duplicatePendingMessage
	body := duplicateResponse{Status: string(dup.Existing.Status)}
	if dup.Existing.Status == entity.PointActive {
		message = fmt.Sprintf(duplicateActiveTemplate, dup.PoolCount)
		body.PointID = &dup.Existing.ID
		body.PoolCount = dup.PoolCount
	}
	body.Error = httpx.ErrorDetail{Code: httpx.CodeDuplicateURL, Message: message}

	logctx.From(r.Context()).Info(message,
		"code", httpx.CodeDuplicateURL, "status", http.StatusConflict, "error", dup)
	httpx.JSON(w, r, http.StatusConflict, body)
}

func limitMessage(kind entity.PointType) string {
	if kind == entity.PointIntentHome {
		return intentHomeLimitMessage
	}
	return flatLimitMessage
}
