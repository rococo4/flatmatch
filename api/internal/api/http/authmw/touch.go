package authmw

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/logctx"
)

const touchEvery = 10 * time.Minute

type Touches interface {
	Touch(ctx context.Context, userID uuid.UUID) error
}

type throttle struct {
	every time.Duration
	at    sync.Map
}

func touch(ctx context.Context, touches Touches, recent *throttle, userID uuid.UUID) {
	if touches == nil || !recent.allow(userID, time.Now()) {
		return
	}
	if err := touches.Touch(ctx, userID); err != nil {
		logctx.From(ctx).Warn("touch the searches of a user", "error", err)
	}
}

func (t *throttle) allow(userID uuid.UUID, now time.Time) bool {
	if last, ok := t.at.Load(userID); ok && now.Sub(last.(time.Time)) < t.every {
		return false
	}
	t.at.Store(userID, now)
	return true
}
