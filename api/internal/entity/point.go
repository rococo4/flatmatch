package entity

import (
	"math"
	"time"

	"github.com/google/uuid"
)

const MaxUserLinkPoints = 5

const MaxIntentHome = 2

const MaxIntentHomeLive = 1

const MaxPointPhotos = 10

const PointDescriptionMaxLen = 400

type PointType string

const (
	PointTypeUnknown PointType = unknown

	PointFlat       PointType = "flat"
	PointIntentHome PointType = "intent_home"
)

type PointStage string

const (
	PointStageUnknown PointStage = unknown

	StageLive     PointStage = "live"
	StageMovingIn PointStage = "moving_in"
)

type PointStatus string

const (
	PointStatusUnknown PointStatus = unknown

	PointPending PointStatus = "pending"
	PointActive  PointStatus = "active"
	PointPaused  PointStatus = "paused"
	PointClosed  PointStatus = "closed"
)

type PointSource string

const (
	PointSourceUnknown PointSource = unknown

	SourceTGChannel PointSource = "tg_channel"
	SourceUserLink  PointSource = "user_link"
	SourceManual    PointSource = "manual"
	SourceRealtor   PointSource = "realtor"
)

type Bathrooms string

const (
	BathroomsUnknown Bathrooms = unknown

	BathroomsCombined Bathrooms = "combined"
	BathroomsSeparate Bathrooms = "separate"
	BathroomsTwo      Bathrooms = "two"
)

type Utilities string

const (
	UtilitiesUnknown Utilities = unknown

	UtilitiesIncluded Utilities = "included"
	UtilitiesMeters   Utilities = "meters"
)

type Restriction string

const (
	RestrictionUnknown Restriction = unknown

	RestrictionNoPets    Restriction = "no_pets"
	RestrictionNoCouples Restriction = "no_couples"
	RestrictionNoSmoking Restriction = "no_smoking"
)

type PointCloseReason string

const (
	CloseReasonClosed   PointCloseReason = "closed"
	CloseReasonRejected PointCloseReason = "rejected"
	CloseReasonGone     PointCloseReason = "gone"
	CloseReasonExpired  PointCloseReason = "expired"
	CloseReasonFound    PointCloseReason = "found"
)

const (
	GoneVoteFloor  = 2
	GoneVoteShare  = 0.3
	PingEveryDays  = 7
	PointTTLDays   = 14
	PingAnswerDays = 3
)

type Point struct {
	ID    uuid.UUID
	Type  PointType
	Stage *PointStage

	Status PointStatus
	Source PointSource

	CreatedBy *uuid.UUID
	ZoneID    string

	PriceTotal   *int
	PriceShare   *int
	Rooms        *int
	Isolated     *bool
	Bathrooms    *Bathrooms
	Utilities    *Utilities
	Restrictions []Restriction

	PhotoKeys   []string
	Description *string

	URLNorm     *string
	MoveIn      *time.Time
	Deposit     *int
	CloseReason *string

	LastSeenAlive time.Time
	GoneCount     int
	PingAskedAt   *time.Time

	CreatedAt time.Time
	EditedAt  time.Time
}

func ParsePointType(raw string) PointType {
	return parseEnum(raw, PointFlat, PointIntentHome)
}

func ParsePointStage(raw string) PointStage {
	return parseEnum(raw, StageLive, StageMovingIn)
}

func ParsePointStatus(raw string) PointStatus {
	return parseEnum(raw, PointPending, PointActive, PointPaused, PointClosed)
}

func ParsePointSource(raw string) PointSource {
	return parseEnum(raw, SourceTGChannel, SourceUserLink, SourceManual, SourceRealtor)
}

func ParseBathrooms(raw string) Bathrooms {
	return parseEnum(raw, BathroomsCombined, BathroomsSeparate, BathroomsTwo)
}

func ParseUtilities(raw string) Utilities {
	return parseEnum(raw, UtilitiesIncluded, UtilitiesMeters)
}

func ParseRestriction(raw string) Restriction {
	return parseEnum(raw, RestrictionNoPets, RestrictionNoCouples, RestrictionNoSmoking)
}

func GoneThreshold(pool int) int {
	threshold := int(math.Ceil(GoneVoteShare * float64(pool)))
	if threshold < GoneVoteFloor {
		return GoneVoteFloor
	}
	return threshold
}

func (p Point) Votable() bool {
	return p.Type == PointFlat && p.URLNorm != nil
}
