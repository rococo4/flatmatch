package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newTxTestPool connects to TEST_DATABASE_URL and creates a scratch table.
//
// The table is temporary in name only — it is a real one, dropped on cleanup —
// because a TEMP table lives per connection, and a pool hands out a different
// connection to almost every statement here.
func newTxTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set — see README, «Тесты, которым нужна база»")
	}

	pool, err := NewPool(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx := t.Context()
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_probe (n int)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP TABLE IF EXISTS tx_probe`); err != nil {
			t.Errorf("drop probe table: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, `TRUNCATE tx_probe`); err != nil {
		t.Fatalf("truncate probe table: %v", err)
	}

	return pool
}

func countProbeRows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()

	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM tx_probe`).Scan(&n); err != nil {
		t.Fatalf("count probe rows: %v", err)
	}
	return n
}

func TestInTxCommitsWhenTheFunctionSucceeds(t *testing.T) {
	pool := newTxTestPool(t)

	err := InTx(t.Context(), pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO tx_probe (n) VALUES (1), (2)`)
		return err
	})
	if err != nil {
		t.Fatalf("InTx() failed: %v", err)
	}

	if got := countProbeRows(t, pool); got != 2 {
		t.Errorf("found %d rows after a committed transaction, want 2", got)
	}
}

func TestInTxRollsBackOnError(t *testing.T) {
	pool := newTxTestPool(t)
	wantErr := errors.New("nope")

	err := InTx(t.Context(), pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO tx_probe (n) VALUES (1)`); err != nil {
			return err
		}
		// The write above is real inside the transaction; returning an error
		// has to undo it.
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("InTx() error = %v, want it to pass through %v", err, wantErr)
	}

	if got := countProbeRows(t, pool); got != 0 {
		t.Errorf("found %d rows after a rolled back transaction, want 0", got)
	}
}

func TestInTxRollsBackOnPanic(t *testing.T) {
	// This is the case the deferred rollback exists for: without it the
	// connection would go back to the pool with an open transaction.
	pool := newTxTestPool(t)

	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic did not propagate out of InTx")
			}
		}()

		_ = InTx(t.Context(), pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `INSERT INTO tx_probe (n) VALUES (1)`); err != nil {
				t.Errorf("insert inside transaction: %v", err)
			}
			panic("boom")
		})
	}()

	if got := countProbeRows(t, pool); got != 0 {
		t.Errorf("found %d rows after a panicking transaction, want 0", got)
	}
}

func TestInTxNestsThroughSavepoints(t *testing.T) {
	// A nested Begin on a pgx.Tx opens a savepoint, so a repository method that
	// opens its own transaction still composes inside a wider one. §12 step 4
	// depends on this: a like, its budget row and the match it produces must
	// land together (§2.3).
	pool := newTxTestPool(t)

	err := InTx(t.Context(), pool, func(outer pgx.Tx) error {
		if _, err := outer.Exec(t.Context(), `INSERT INTO tx_probe (n) VALUES (1)`); err != nil {
			return err
		}

		// An inner failure rolls back to the savepoint without killing the
		// outer transaction.
		inner := InTx(t.Context(), outer, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `INSERT INTO tx_probe (n) VALUES (2)`); err != nil {
				return err
			}
			return errors.New("inner fails")
		})
		if inner == nil {
			t.Error("the inner transaction was expected to fail")
		}

		_, err := outer.Exec(t.Context(), `INSERT INTO tx_probe (n) VALUES (3)`)
		return err
	})
	if err != nil {
		t.Fatalf("outer InTx() failed: %v", err)
	}

	// Rows 1 and 3 survive; row 2 was undone by the savepoint rollback.
	if got := countProbeRows(t, pool); got != 2 {
		t.Errorf("found %d rows, want 2 — the savepoint should have removed only the inner write", got)
	}
}
