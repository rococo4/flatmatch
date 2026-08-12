package likesapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/logctx"
)

// The three messages the front-end shows as they come: the limit, the missing
// search (the mini-form of §2.3) and the unapproved questionnaire (§5).
const (
	likeLimitMessage   = "Лимит исчерпан"
	nextFreeAtTemplate = " — следующий лайк освободится через "
	needSearchMessage  = "Укажи, где ищешь — и лайк улетит"
	notApprovedMessage = "Анкета ещё на проверке — лайки откроются после одобрения"
)

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, userID uuid.UUID, err error) {
	var fields *entity.FieldsError
	switch {
	case errors.Is(err, entity.ErrNotFound):
		// Invisible and non-existent are the same answer (§4).
		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such person", err)
	case errors.Is(err, entity.ErrNeedSearch):
		httpx.BadRequest(w, r, httpx.CodeNeedSearch, needSearchMessage, nil, err)
	case errors.Is(err, entity.ErrLikeLimit):
		httpx.BadRequest(w, r, httpx.CodeLikeLimit, h.limitMessage(r, userID), nil, err)
	case errors.Is(err, entity.ErrNotApproved):
		httpx.Error(w, r, http.StatusForbidden, httpx.CodeNotApproved, notApprovedMessage, err)
	case errors.As(err, &fields):
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "value outside the allowed set", fields.Fields, err)
	default:
		httpx.Internal(w, r, err)
	}
}

// limitMessage says how long until the next slot. Relative, not a clock time:
// «в 15:00» spent at 15:00 reads as right now when it means tomorrow. The 400
// envelope has no field for it: the message is what the user reads.
func (h *Handler) limitMessage(r *http.Request, userID uuid.UUID) string {
	budget, err := h.likes.Budget(r.Context(), userID)
	if err != nil {
		// The refusal itself is correct and already decided, so a budget that
		// cannot be read costs the countdown, not the answer.
		logctx.From(r.Context()).Error("read like budget for the limit message", "error", err)
		return likeLimitMessage
	}
	if budget.NextFreeAt == nil {
		return likeLimitMessage
	}
	return likeLimitMessage + nextFreeAtTemplate + untilText(time.Until(*budget.NextFreeAt))
}

func untilText(d time.Duration) string {
	if d < time.Minute {
		return "минуту"
	}
	h := int(d / time.Hour)
	m := int(d % time.Hour / time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%d мин", m)
	case m == 0:
		return fmt.Sprintf("%d ч", h)
	default:
		return fmt.Sprintf("%d ч %d мин", h, m)
	}
}
