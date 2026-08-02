package httpx

import (
	"context"
	"net/http"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

func Health(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			Error(w, r, http.StatusServiceUnavailable, CodeInternal, "database is unreachable", err)
			return
		}
		JSON(w, r, http.StatusOK, map[string]string{"status": "ok", "database": "ok"})
	}
}
