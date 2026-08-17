// Package notificationsrepo stores the push log (§7): the queue the worker
// drains and the sent marks its thresholds are counted from.
package notificationsrepo

import "github.com/jackc/pgx/v5/pgxpool"

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}
