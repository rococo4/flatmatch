// Package searchesrepo stores searches and answers pool queries (§2.1).
package searchesrepo

import "github.com/jackc/pgx/v5/pgxpool"

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}
