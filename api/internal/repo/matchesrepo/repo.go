// Package matchesrepo reads the matches a person is in and the peer behind
// each of them (§2.3).
package matchesrepo

import "github.com/jackc/pgx/v5/pgxpool"

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}
