package pointsrepo

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrURLTaken = errors.New("url already taken by a live point")

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}
