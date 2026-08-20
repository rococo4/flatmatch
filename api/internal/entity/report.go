package entity

import (
	"time"

	"github.com/google/uuid"
)

const (
	ReportContextZone  = "zone"
	ReportContextPoint = "point"
	ReportContextMatch = "match"
)

type ReportReason string

const (
	ReportReasonUnknown ReportReason = unknown

	ReasonSpam  ReportReason = "spam"
	ReasonScam  ReportReason = "scam"
	ReasonAbuse ReportReason = "abuse"
	ReasonOther ReportReason = "other"
)

type ReportVerdict string

const (
	ReportVerdictUnknown ReportVerdict = unknown

	ReportBanned  ReportVerdict = "banned"
	ReportSkipped ReportVerdict = "skipped"
)

type Report struct {
	ID         uuid.UUID
	ReporterID uuid.UUID
	TargetID   uuid.UUID

	Reason     ReportReason
	ContextKey string

	CreatedAt  time.Time
	ResolvedAt *time.Time
	Verdict    *ReportVerdict
}

func ParseReportReason(raw string) ReportReason {
	return parseEnum(raw, ReasonSpam, ReasonScam, ReasonAbuse, ReasonOther)
}

func ParseReportVerdict(raw string) ReportVerdict {
	return parseEnum(raw, ReportBanned, ReportSkipped)
}

func ReportReasonLabel(reason ReportReason) string {
	switch reason {
	case ReasonSpam:
		return "Спам/реклама"
	case ReasonScam:
		return "Мошенничество"
	case ReasonAbuse:
		return "Оскорбления/угрозы"
	case ReasonOther:
		return "Другое"
	default:
		return "Другое"
	}
}

func (r ReportReason) Known() bool {
	return r != "" && r != ReportReasonUnknown
}

func (r Report) Resolved() bool { return r.ResolvedAt != nil }
