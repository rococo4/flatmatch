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
		ID:          id,
		TelegramID:  42,
		Name:        "Арсений",
		Username:    pointer.To("arsenii"),
		PhotoURL:    pointer.To("https://t.me/i/userpic/320/arsenii.jpg"),
		Status:      "active",
		PhotoStatus: "ok",
		LastSeen:    now,
		CreatedAt:   now,
	}.toEntity(t.Context())

	want := entity.User{
		ID:          id,
		TelegramID:  42,
		Name:        "Арсений",
		Username:    "arsenii",
		PhotoURL:    "https://t.me/i/userpic/320/arsenii.jpg",
		Status:      entity.StatusActive,
		PhotoStatus: entity.PhotoOK,
		LastSeen:    now,
		CreatedAt:   now,
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
	}.toEntity(t.Context())

	if got.Status != entity.StatusUnknown {
		t.Errorf("Status = %q, want %q", got.Status, entity.StatusUnknown)
	}
	if got.PhotoStatus != entity.PhotoUnknown {
		t.Errorf("PhotoStatus = %q, want %q", got.PhotoStatus, entity.PhotoUnknown)
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

func TestToEntityWarnsAboutAnUnknownStatus(t *testing.T) {
	// Hiding an account without saying so turns a five-minute fix into a
	// support ticket weeks later, so the warning is part of the contract.
	var logged bytes.Buffer
	ctx := logctx.With(t.Context(), slog.New(slog.NewJSONHandler(&logged, nil)))

	userRow{Name: "Из будущего", Status: "shadow_banned", PhotoStatus: "ok"}.toEntity(ctx)

	out := logged.String()
	if !strings.Contains(out, `"level":"WARN"`) {
		t.Errorf("no warning was logged, got: %s", out)
	}
	// The stored value must be in there — it is what says which migration this
	// binary is behind.
	if !strings.Contains(out, "shadow_banned") {
		t.Errorf("the warning does not quote the stored value, got: %s", out)
	}
}
