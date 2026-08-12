package pointsrepo

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const pointColumns = `id, type, stage, status, source, created_by, zone_id,
	price_total, price_share, rooms, isolated, bathrooms, utilities,
	restrictions, photo_key, url_norm, move_in, deposit, close_reason,
	created_at, edited_at`

const livePoint = `status IN ('pending', 'active')`

type pointRow struct {
	ID           uuid.UUID  `db:"id"`
	Type         string     `db:"type"`
	Stage        *string    `db:"stage"`
	Status       string     `db:"status"`
	Source       string     `db:"source"`
	CreatedBy    *uuid.UUID `db:"created_by"`
	ZoneID       string     `db:"zone_id"`
	PriceTotal   *int       `db:"price_total"`
	PriceShare   *int       `db:"price_share"`
	Rooms        *int       `db:"rooms"`
	Isolated     *bool      `db:"isolated"`
	Bathrooms    *string    `db:"bathrooms"`
	Utilities    *string    `db:"utilities"`
	Restrictions []string   `db:"restrictions"`
	PhotoKey     *string    `db:"photo_key"`
	URLNorm      *string    `db:"url_norm"`
	MoveIn       *time.Time `db:"move_in"`
	Deposit      *int       `db:"deposit"`
	CloseReason  *string    `db:"close_reason"`
	CreatedAt    time.Time  `db:"created_at"`
	EditedAt     time.Time  `db:"edited_at"`
}

func (r pointRow) toEntity() entity.Point {
	p := entity.Point{
		ID:          r.ID,
		Type:        entity.ParsePointType(r.Type),
		Status:      entity.ParsePointStatus(r.Status),
		Source:      entity.ParsePointSource(r.Source),
		CreatedBy:   r.CreatedBy,
		ZoneID:      r.ZoneID,
		PriceTotal:  r.PriceTotal,
		PriceShare:  r.PriceShare,
		Rooms:       r.Rooms,
		Isolated:    r.Isolated,
		PhotoKey:    pointer.Get(r.PhotoKey),
		URLNorm:     r.URLNorm,
		MoveIn:      r.MoveIn,
		Deposit:     r.Deposit,
		CloseReason: r.CloseReason,
		CreatedAt:   r.CreatedAt,
		EditedAt:    r.EditedAt,
	}
	if r.Stage != nil {
		p.Stage = pointer.To(entity.ParsePointStage(*r.Stage))
	}
	if r.Bathrooms != nil {
		p.Bathrooms = pointer.To(entity.ParseBathrooms(*r.Bathrooms))
	}
	if r.Utilities != nil {
		p.Utilities = pointer.To(entity.ParseUtilities(*r.Utilities))
	}
	for _, code := range r.Restrictions {
		p.Restrictions = append(p.Restrictions, entity.ParseRestriction(code))
	}
	return p
}

func enumArg[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	return pointer.To(string(*v))
}

func restrictionsArg(codes []entity.Restriction) []string {
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		out = append(out, string(c))
	}
	return out
}
