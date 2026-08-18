package searchesapi

import (
	"errors"
	"testing"

	"github.com/AlekSi/pointer"

	"flatmatch/api/internal/entity"
)

func TestToParamsEmptyOptionalIsAny(t *testing.T) {
	b := paramsRequest{
		ZoneID:   "d-khamovniki",
		PriceMax: 45000,
		MoveIn:   pointer.To(""),
		Term:     pointer.To(""),
	}
	p, err := b.toParams()
	if err != nil {
		t.Fatalf("toParams() = %v, want no error", err)
	}
	if p.MoveIn != nil {
		t.Errorf("MoveIn = %v, want nil", p.MoveIn)
	}
	if p.Term != nil {
		t.Errorf("Term = %v, want nil", *p.Term)
	}
	if bad := p.InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

func TestToParamsMalformedMoveIn(t *testing.T) {
	for _, raw := range []string{"2026-13", "sept", "2026-09-01"} {
		t.Run(raw, func(t *testing.T) {
			b := paramsRequest{ZoneID: "d-khamovniki", PriceMax: 45000, MoveIn: pointer.To(raw)}
			_, err := b.toParams()
			var fields *entity.FieldsError
			if !errors.As(err, &fields) {
				t.Fatalf("toParams() = %v, want a FieldsError", err)
			}
			if len(fields.Fields) != 1 || fields.Fields[0] != "move_in" {
				t.Errorf("Fields = %v, want [move_in]", fields.Fields)
			}
		})
	}
}
