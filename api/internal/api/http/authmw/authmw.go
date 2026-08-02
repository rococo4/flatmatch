package authmw

import (
	"context"
	"errors"
	"net/http"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/logctx"
	"flatmatch/api/internal/service/users"
	"flatmatch/api/internal/tma"
)

const scheme = "tma"

type Validator interface {
	Validate(raw string) (tma.InitData, error)
}

type Accounts interface {
	EnsureFromTelegram(ctx context.Context, tg entity.TelegramIdentity) (entity.User, error)
}

func New(validator Validator, accounts Accounts) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := initDataFromHeader(r)
			if !ok {
				httpx.Unauthorized(w, r, "missing Telegram credentials", nil)
				return
			}

			data, err := validator.Validate(raw)
			if err != nil {
				httpx.Unauthorized(w, r, "invalid Telegram credentials", err)
				return
			}

			user, err := accounts.EnsureFromTelegram(r.Context(), entity.TelegramIdentity{
				ID:       data.User.ID,
				Name:     entity.DisplayName(data.User.FirstName, data.User.LastName),
				Username: data.User.Username,
				PhotoURL: data.User.PhotoURL,
			})
			switch {
			case errors.Is(err, users.ErrBanned):
				httpx.Error(w, r, http.StatusForbidden, httpx.CodeForbidden, "account is blocked", err)
				return
			case err != nil:
				httpx.Internal(w, r, err)
				return
			}

			ctx := newContext(r.Context(), user)

			ctx = logctx.With(ctx, logctx.From(ctx).With("user_id", user.ID, "tg_id", user.TelegramID))

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
