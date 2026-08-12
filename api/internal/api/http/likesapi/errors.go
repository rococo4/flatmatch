package likesapi

import (
	"errors"
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
	nextFreeAtTemplate = " — следующий лайк освободится в "
	needSearchMessage  = "Укажи, где ищешь — и лайк улетит"
	notApprovedMessage = "Анкета ещё на проверке — лайки откроются после одобрения"
)

const clockLayout = "15:04"

// moscow, not time.LoadLocation("Europe/Moscow"): the product is Moscow-only
// (§1), the zone has had no DST and no offset change since 2014, and a fixed
// zone cannot fail on a container that ships without tzdata.
var moscow = time.FixedZone("MSK", 3*60*60)

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

// limitMessage names the hour the next slot comes back. The 400 envelope has
// no field for it: the message is what the user reads, and a second field
// saying the same thing would be one more thing to keep in step.
func (h *Handler) limitMessage(r *http.Request, userID uuid.UUID) string {
	budget, err := h.likes.Budget(r.Context(), userID)
	if err != nil {
		// The refusal itself is correct and already decided, so a budget that
		// cannot be read costs the hour, not the answer.
		logctx.From(r.Context()).Error("read like budget for the limit message", "error", err)
		return likeLimitMessage
	}
	if budget.NextFreeAt == nil {
		return likeLimitMessage
	}
	return likeLimitMessage + nextFreeAtTemplate + budget.NextFreeAt.In(moscow).Format(clockLayout)
}
