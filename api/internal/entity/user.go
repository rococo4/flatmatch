package entity

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

const OnboardingSteps = 12

type Status string

const (
	StatusUnknown Status = unknown

	StatusDraft Status = "draft"

	StatusPending Status = "pending"

	StatusActive Status = "active"

	StatusBanned Status = "banned"
)

type ModerationStatus string

const (
	ModerationUnknown ModerationStatus = unknown

	ModerationPending  ModerationStatus = "pending"
	ModerationOK       ModerationStatus = "ok"
	ModerationRejected ModerationStatus = "rejected"
)

type TelegramIdentity struct {
	ID       int64
	Name     string
	Username string
}

type User struct {
	ID         uuid.UUID
	TelegramID int64

	Name     string
	Username string

	Profile Profile

	PhotoKey    string
	PhotoStatus ModerationStatus
	AboutStatus ModerationStatus

	Status         Status
	OnboardingStep int

	LastSeen  time.Time
	CreatedAt time.Time
}

func ParseStatus(raw string) Status {
	return parseEnum(raw, StatusDraft, StatusPending, StatusActive, StatusBanned)
}

func ParseModerationStatus(raw string) ModerationStatus {
	return parseEnum(raw, ModerationPending, ModerationOK, ModerationRejected)
}

func DisplayName(firstName, lastName string) string {
	return strings.TrimSpace(firstName + " " + lastName)
}

func (u User) MissingFields() []string {
	missing := u.Profile.MissingFields()
	if u.PhotoKey == "" {
		missing = append(missing, "photo")
	}
	return missing
}

func (u User) Visible() bool {
	return u.Status == StatusActive
}

func (u User) PhotoApproved() bool {
	return u.PhotoStatus == ModerationOK && u.PhotoKey != ""
}

func (u User) AboutApproved() bool {
	return u.AboutStatus == ModerationOK && u.Profile.About != ""
}
