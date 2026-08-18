package entity

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

const MaxPersonalPushesPerDay = 10

const AggregateWindow = 24 * time.Hour

const (
	TTLQuietDays      = 7
	TTLAfterMatchDays = 3
	TTLAnswerDays     = 3
)

const PostMatchDays = 5

type NotificationType string

const (
	NotifTypeUnknown NotificationType = unknown

	NotifZoneNew NotificationType = "zone_new"
	NotifPoolNew NotificationType = "pool_new"

	NotifLiked   NotificationType = "liked"
	NotifMatched NotificationType = "matched"

	NotifProfileOK      NotificationType = "profile_ok"
	NotifPhotoRejected  NotificationType = "photo_rejected"
	NotifAboutRejected  NotificationType = "about_rejected"
	NotifPointOK        NotificationType = "point_ok"
	NotifPointRejected  NotificationType = "point_rejected"
	NotifStillSearching NotificationType = "still_searching"
	NotifPostMatch      NotificationType = "post_match"
)

type PollAnswer string

const (
	PollAnswerUnknown PollAnswer = unknown

	PollMoving  PollAnswer = "moving"
	PollTalking PollAnswer = "talking"
	PollFailed  PollAnswer = "failed"
)

type Notification struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Type   NotificationType

	ActorKey   string
	ContextKey string
	Payload    map[string]any

	CreatedAt time.Time
	SentAt    *time.Time
	FailedAt  *time.Time
	Attempts  int
}

var personalPushTypes = []NotificationType{
	NotifLiked, NotifMatched,
	NotifProfileOK, NotifPhotoRejected, NotifAboutRejected,
	NotifPointOK, NotifPointRejected,
}

func ParseNotificationType(raw string) NotificationType {
	return parseEnum(raw, NotifZoneNew, NotifPoolNew, NotifLiked, NotifMatched,
		NotifProfileOK, NotifPhotoRejected, NotifAboutRejected,
		NotifPointOK, NotifPointRejected, NotifStillSearching, NotifPostMatch)
}

func ParsePollAnswer(raw string) PollAnswer {
	return parseEnum(raw, PollMoving, PollTalking, PollFailed)
}

func PersonalPushTypes() []NotificationType {
	return slices.Clone(personalPushTypes)
}

func (t NotificationType) Aggregated() bool {
	return t == NotifZoneNew || t == NotifPoolNew
}
