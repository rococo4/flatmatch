package usersrepo

import "github.com/jackc/pgx/v5/pgxpool"

// Repo is the users repository backed by Postgres.
type Repo struct {
	pool *pgxpool.Pool
}

// New wires the repository to a connection pool.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}
