package reportsrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"flatmatch/api/internal/entity"
)

func (r *Repo) Insert(ctx context.Context, report entity.Report) (entity.Report, bool, error) {
	const query = `
		INSERT INTO reports (reporter_id, target_id, reason, context_key)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (reporter_id, target_id) DO NOTHING
		RETURNING ` + columns

	stored, err := r.one(ctx, query, report.ReporterID, report.TargetID,
		string(report.Reason), pointer.ToOrNil(report.ContextKey))
	switch {
	case errors.Is(err, entity.ErrNotFound):
		return entity.Report{}, false, nil
	case err != nil:
		return entity.Report{}, false, fmt.Errorf("file a report of %s by %s: %w",
			report.TargetID, report.ReporterID, err)
	}
	return stored, true, nil
}

func (r *Repo) ByID(ctx context.Context, id uuid.UUID) (entity.Report, error) {
	report, err := r.one(ctx, `SELECT `+columns+` FROM reports WHERE id = $1`, id)
	if err != nil {
		return entity.Report{}, fmt.Errorf("get report %s: %w", id, err)
	}
	return report, nil
}

func (r *Repo) Resolve(ctx context.Context, id uuid.UUID, verdict entity.ReportVerdict) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE reports SET verdict = $2, resolved_at = now()
		WHERE id = $1 AND resolved_at IS NULL`, id, string(verdict))
	if err != nil {
		return false, fmt.Errorf("resolve report %s as %s: %w", id, verdict, err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repo) Unresolved(ctx context.Context) ([]entity.Report, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+columns+` FROM reports WHERE resolved_at IS NULL ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list unresolved reports: %w", err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[reportRow])
	if err != nil {
		return nil, fmt.Errorf("list unresolved reports: %w", err)
	}

	out := make([]entity.Report, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

func (r *Repo) one(ctx context.Context, query string, args ...any) (entity.Report, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return entity.Report{}, err
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[reportRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Report{}, entity.ErrNotFound
		}
		return entity.Report{}, err
	}
	return row.toEntity(), nil
}
