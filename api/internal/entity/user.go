package entity

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

const OnboardingSteps = 12

const MaxUserPhotos = 5

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

	PhotoKeys   []string
	PhotoStatus ModerationStatus
	AboutStatus ModerationStatus

	Status         Status
	OnboardingStep int
	Source         string

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

func ReorderPhotos(keys []string, order []int) ([]string, bool) {
	if len(order) != len(keys) {
		return nil, false
	}

	seen := make([]bool, len(keys))
	out := make([]string, 0, len(keys))
	for _, i := range order {
		if i < 0 || i >= len(keys) || seen[i] {
			return nil, false
		}
		seen[i] = true
		out = append(out, keys[i])
	}
	return out, true
}

func RemovePhoto(keys []string, index int) ([]string, bool) {
	if index < 0 || index >= len(keys) {
		return nil, false
	}

	out := make([]string, 0, len(keys)-1)
	out = append(out, keys[:index]...)
	return append(out, keys[index+1:]...), true
}

func (u User) MissingFields() []string {
	missing := u.Profile.MissingFields()
	if len(u.PhotoKeys) == 0 {
		missing = append(missing, "photo")
	}
	return missing
}

func (u User) Visible() bool {
	return u.Status == StatusActive
}

func (u User) PhotoApproved() bool {
	return u.PhotoStatus == ModerationOK && len(u.PhotoKeys) > 0
}

func (u User) AboutApproved() bool {
	return u.AboutStatus == ModerationOK && u.Profile.About != ""
}
