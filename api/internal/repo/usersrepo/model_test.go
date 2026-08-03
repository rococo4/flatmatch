package usersrepo

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/logctx"
)

func TestToEntityMapsAKnownRow(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	got := userRow{
		ID:             id,
		TelegramID:     42,
		Name:           "Арсений",
		Username:       pointer.To("arsenii"),
		Age:            pointer.To(28),
		Gender:         pointer.To("male"),
		GenderPref:     "any",
		Schedule:       pointer.To("owl"),
		About:          pointer.To("Люблю тишину по утрам"),
		PhotoKey:       pointer.To("photos/" + id.String() + ".jpg"),
		PhotoStatus:    "ok",
		AboutStatus:    "ok",
		Status:         "active",
		OnboardingStep: 11,
		LastSeen:       now,
		CreatedAt:      now,
	}.toEntity(t.Context())

	want := entity.User{
		ID:         id,
		TelegramID: 42,
		Name:       "Арсений",
		Username:   "arsenii",
		Profile: entity.Profile{
			Age:        28,
			Gender:     entity.GenderMale,
			GenderPref: entity.GenderPrefAny,
			Schedule:   entity.ScheduleOwl,
			About:      "Люблю тишину по утрам",
		},
		PhotoKey:       "photos/" + id.String() + ".jpg",
		PhotoStatus:    entity.ModerationOK,
		AboutStatus:    entity.ModerationOK,
		Status:         entity.StatusActive,
		OnboardingStep: 11,
		LastSeen:       now,
		CreatedAt:      now,
	}
	if got != want {
		t.Errorf("toEntity() = %+v, want %+v", got, want)
	}
}

func TestToEntitySurvivesAnUnknownStatus(t *testing.T) {
	// The case this exists for: a migration added a value the deployed binary
	// does not know. The row still maps, so the endpoint keeps working…
	got := userRow{
		Name:        "Из будущего",
		Status:      "shadow_banned",
		PhotoStatus: "under_review",
		AboutStatus: "ok",
		Schedule:    pointer.To("nocturnal"),
	}.toEntity(t.Context())

	if got.Status != entity.StatusUnknown {
		t.Errorf("Status = %q, want %q", got.Status, entity.StatusUnknown)
	}
	if got.PhotoStatus != entity.ModerationUnknown {
		t.Errorf("PhotoStatus = %q, want %q", got.PhotoStatus, entity.ModerationUnknown)
	}
	if got.Profile.Schedule != entity.ScheduleUnknown {
		t.Errorf("Schedule = %q, want %q", got.Profile.Schedule, entity.ScheduleUnknown)
	}
	// …and the account stays hidden, rather than being shown with a meaning
	// nothing in this binary can act on.
	if got.Visible() {
		t.Error("an account with an uninterpretable status must not be visible")
	}
	// Everything that does not depend on the enum still arrives intact.
	if got.Name != "Из будущего" {
		t.Errorf("Name = %q, want it mapped despite the unknown status", got.Name)
	}
}

func TestToEntityWarnsAboutUnknownValues(t *testing.T) {
	// Hiding a field without saying so turns a five-minute fix into a support
	// ticket weeks later, so the warning is part of the contract.
	var logged bytes.Buffer
	ctx := logctx.With(t.Context(), slog.New(slog.NewJSONHandler(&logged, nil)))

	userRow{
		Name:        "Из будущего",
		Status:      "shadow_banned",
		PhotoStatus: "ok",
		AboutStatus: "ok",
		Cleaning:    pointer.To("robot"),
	}.toEntity(ctx)

	out := logged.String()
	if !strings.Contains(out, `"level":"WARN"`) {
		t.Errorf("no warning was logged, got: %s", out)
	}
	// Both offending columns have to be named: one warning per row is only
	// enough if it says everything that went wrong in it.
	for _, field := range []string{"status", "cleaning"} {
		if !strings.Contains(out, field) {
			t.Errorf("the warning does not name %q, got: %s", field, out)
		}
	}
}
