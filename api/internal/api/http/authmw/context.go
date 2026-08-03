package authmw

import (
	"context"

	"flatmatch/api/internal/entity"
)

type contextKey int

const currentUserKey contextKey = iota

func FromContext(ctx context.Context) (entity.User, bool) {
	u, ok := ctx.Value(currentUserKey).(entity.User)
	return u, ok
}

func newContext(ctx context.Context, u entity.User) context.Context {
	return context.WithValue(ctx, currentUserKey, u)
}
