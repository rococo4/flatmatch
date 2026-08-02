package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

func InTx(ctx context.Context, db Beginner, fn func(pgx.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
