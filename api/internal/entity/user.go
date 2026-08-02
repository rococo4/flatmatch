package entity

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusUnknown Status = "unknown"

	StatusDraft Status = "draft"

	StatusPending Status = "pending"

	StatusActive Status = "active"

	StatusBanned Status = "banned"
)

// ParseStatus converts a stored value into a Status, resolving anything it does
// not recognise to StatusUnknown.
//
// It cannot fail, so callers never have to decide what to do about a value they
// have no way to interpret. StatusUnknown is itself the signal — nothing writes
// it to the database, so seeing it means the schema is ahead of this binary,
// and it is not Visible, so such an account is hidden rather than shown with a
// meaning nobody can act on.
func ParseStatus(raw string) Status {
	switch s := Status(raw); s {
	case StatusDraft, StatusPending, StatusActive, StatusBanned:
		return s
	default:
		return StatusUnknown
	}
}

type PhotoStatus string

const (
	PhotoUnknown PhotoStatus = "unknown"

	PhotoPending PhotoStatus = "pending"
	PhotoOK      PhotoStatus = "ok"

	PhotoRejected PhotoStatus = "rejected"
)

// ParsePhotoStatus converts a stored value into a PhotoStatus, with the same
// contract as ParseStatus: it cannot fail, and PhotoUnknown is the signal.
func ParsePhotoStatus(raw string) PhotoStatus {
	switch s := PhotoStatus(raw); s {
	case PhotoPending, PhotoOK, PhotoRejected:
		return s
	default:
		return PhotoUnknown
	}
}

type TelegramIdentity struct {
	ID       int64
	Name     string
	Username string
	PhotoURL string
}

func DisplayName(firstName, lastName string) string {
	return strings.TrimSpace(firstName + " " + lastName)
}

type User struct {
	ID         uuid.UUID
	TelegramID int64

	Name string

	Username    string
	PhotoURL    string
	Status      Status
	PhotoStatus PhotoStatus
	LastSeen    time.Time
	CreatedAt   time.Time
}

func (u User) Visible() bool {
	return u.Status == StatusActive
}
