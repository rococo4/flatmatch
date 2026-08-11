// Package likesrepo stores dear likes and the matches they close (§2.3).
package likesrepo

import "github.com/jackc/pgx/v5/pgxpool"

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}
