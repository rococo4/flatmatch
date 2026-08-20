package reportsrepo

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const columns = `id, reporter_id, target_id, reason, context_key, created_at, resolved_at, verdict`

type reportRow struct {
	ID         uuid.UUID `db:"id"`
	ReporterID uuid.UUID `db:"reporter_id"`
	TargetID   uuid.UUID `db:"target_id"`

	Reason     string  `db:"reason"`
	ContextKey *string `db:"context_key"`

	CreatedAt  time.Time  `db:"created_at"`
	ResolvedAt *time.Time `db:"resolved_at"`
	Verdict    *string    `db:"verdict"`
}

func (r reportRow) toEntity() entity.Report {
	out := entity.Report{
		ID:         r.ID,
		ReporterID: r.ReporterID,
		TargetID:   r.TargetID,
		Reason:     entity.ParseReportReason(r.Reason),
		ContextKey: pointer.Get(r.ContextKey),
		CreatedAt:  r.CreatedAt,
		ResolvedAt: r.ResolvedAt,
	}
	if r.Verdict != nil {
		out.Verdict = pointer.To(entity.ParseReportVerdict(*r.Verdict))
	}
	return out
}
