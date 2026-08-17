package authmw

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/logctx"
)

// touchEvery — как часто активность одного человека вообще доезжает до базы.
// SQL-гейт Touch и так пишет не чаще раза в час, но ходить за этим отказом в
// базу на каждый GET ленты незачем.
const touchEvery = 10 * time.Minute

// Touches двигает «последнюю активность» поисков: открытое приложение — это
// молчаливый ответ на «ещё ищешь?» (§3). nil — молча ничего.
type Touches interface {
	Touch(ctx context.Context, userID uuid.UUID) error
}

// throttle помнит, когда активность каждого человека последний раз доехала до
// базы. Карта не чистится: строка на человека — цена, которую один VPS платит
// спокойно, а перезапуск обнуляет её сам.
type throttle struct {
	every time.Duration
	at    sync.Map // uuid.UUID → time.Time
}

// touch is best-effort: незаписанная активность стоит лишнего вопроса «ещё
// ищешь?», а упавший на ней запрос стоил бы экрана.
func touch(ctx context.Context, touches Touches, recent *throttle, userID uuid.UUID) {
	if touches == nil || !recent.allow(userID, time.Now()) {
		return
	}
	if err := touches.Touch(ctx, userID); err != nil {
		logctx.From(ctx).Warn("touch the searches of a user", "error", err)
	}
}

// allow: гонка двух одновременных запросов одного человека даёт лишний UPDATE,
// а не потерянную активность — цена верная.
func (t *throttle) allow(userID uuid.UUID, now time.Time) bool {
	if last, ok := t.at.Load(userID); ok && now.Sub(last.(time.Time)) < t.every {
		return false
	}
	t.at.Store(userID, now)
	return true
}
