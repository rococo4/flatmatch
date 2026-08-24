package entity

import "testing"

func TestParseOccupationKnowsTheJoblessAnswer(t *testing.T) {
	if got := ParseOccupation("none"); got != OccupationNone {
		t.Errorf("ParseOccupation(none) = %q, want %q", got, OccupationNone)
	}
	if got := ParseOccupation("freelance"); got != OccupationUnknown {
		t.Errorf("ParseOccupation(freelance) = %q, want %q", got, OccupationUnknown)
	}
}
