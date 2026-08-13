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

// Микрокопия отказов — та, что читает человек в форме (§6): лимит зовёт
// закрыть лишнее, дубль предлагает войти в чужой пул, пул закрыт до входа.
const (
	duplicateActiveTemplate = "Эта квартира уже есть, в пуле %d — войти?"
	duplicatePendingMessage = "Эта квартира уже добавлена и ждёт проверки"
	flatLimitMessage        = "Пять квартир уже добавлено — закрой одну в Моих поисках"
	intentHomeLimitMessage  = "Ты уже зовёшь к себе — активных точек может быть две"
	needEnterMessage        = "Пул откроется после входа"
)

// handleFormError adds to handleError то, чем отвечает только форма: дубль
// ссылки и лимит. Тип формы нужен лимиту: их два, и человеку важно, в какой он
// упёрся.
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

// handleError turns the service's sentinels into the codes of §3.
func handleError(w http.ResponseWriter, r *http.Request, err error) {
	var fields *entity.FieldsError
	switch {
	case errors.Is(err, entity.ErrNeedEnter):
		httpx.Error(w, r, http.StatusForbidden, httpx.CodeNeedEnter, needEnterMessage, err)
	case errors.Is(err, entity.ErrNotFound):
		// Точка на проверке для всех, кроме создателя, не существует (§5).
		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such point", err)
	case errors.As(err, &fields):
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "value outside the allowed set", fields.Fields, err)
	default:
		httpx.Internal(w, r, err)
	}
}

// duplicate answers a link that already lives in the product. Активная точка
// зовёт войти в свой пул, точка на проверке — ещё ничья, входить некуда: до
// аппрува её видит только тот, кто подал (§5).
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
