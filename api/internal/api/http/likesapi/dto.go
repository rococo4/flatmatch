package likesapi

import (
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/service/zones"
)

// zoneDTO is the outbound twin of searchesapi's: same fields, same
// declensions. Copied rather than shared because a DTO belongs to the handler
// that serves it.
type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

type likeRequest struct {
	ToID string `json:"to_id"`
	// ZoneID is the pool the like was placed from; empty is legal — a reply has
	// no pool to name (§2.3).
	ZoneID string `json:"zone_id"`
	// PointID is the other context: пул квартиры или «Хочу эту комнату» с
	// карточки «к себе». Взаимоисключим с зоной — лайк несёт один контекст, и
	// два сразу сервис отбивает по имени полей.
	PointID string `json:"point_id"`
}

type likeResponse struct {
	Matched bool       `json:"matched"`
	MatchID *uuid.UUID `json:"match_id,omitempty"`
	// Already is the repeat tap: nothing was spent, nothing changed.
	Already bool      `json:"already"`
	Budget  budgetDTO `json:"budget"`
}

type budgetDTO struct {
	Remaining  int        `json:"remaining"`
	NextFreeAt *time.Time `json:"next_free_at,omitempty"`
}

// pointDTO is the mini-card of a point context: the label is ready to print,
// the kind colours the card's header and tells «отклик на твою комнату» from a
// shared pool (§2.2).
type pointDTO struct {
	ID    uuid.UUID `json:"id"`
	Kind  string    `json:"kind"`
	Label string    `json:"label"`
}

// incomingDTO is one card of «Тебя лайкнули» (§6): a name right away, the
// context to tap into, and the grey «сейчас не в поиске» mark. No contacts —
// they exist on the match screen and nowhere else (§3), which dto_test pins.
type incomingDTO struct {
	UserID   uuid.UUID `json:"user_id"`
	Name     string    `json:"name"`
	Age      int       `json:"age"`
	PhotoURL string    `json:"photo_url"`
	Schedule string    `json:"schedule"`
	Smoking  string    `json:"smoking"`
	PetsOwn  string    `json:"pets_own"`

	Zone         *zoneDTO  `json:"zone,omitempty"`
	Point        *pointDTO `json:"point,omitempty"`
	NotSearching bool      `json:"not_searching"`
	At           time.Time `json:"at"`
}

func toLikeResponse(res entity.LikeResult) likeResponse {
	out := likeResponse{
		Already: res.Already,
		Budget:  budgetDTO{Remaining: res.Budget.Remaining, NextFreeAt: res.Budget.NextFreeAt},
	}
	if res.Match != nil {
		out.Matched = true
		out.MatchID = &res.Match.ID
	}
	return out
}

func toIncoming(like likesrepo.IncomingLike, photoURL string, zone *zoneDTO, point *pointDTO) incomingDTO {
	return incomingDTO{
		UserID:   like.From.ID,
		Name:     like.From.Name,
		Age:      like.From.Profile.Age,
		PhotoURL: photoURL,
		Schedule: string(like.From.Profile.Schedule),
		Smoking:  string(like.From.Profile.Smoking),
		PetsOwn:  string(like.From.Profile.PetsOwn),

		Zone:         zone,
		Point:        point,
		NotSearching: like.NotSearching,
		At:           like.CreatedAt,
	}
}

// pointLabel: адресат intent_home-контекста и есть автор комнаты — отклик
// пришёл именно на неё (§2.2). Станция может не приехать, если зона ушла из
// справочника, — строка тогда просто короче.
func pointLabel(kind entity.PointType, station string) string {
	label := "квартира"
	if kind == entity.PointIntentHome {
		label = "на твою комнату"
	}
	if station == "" {
		return label
	}
	if kind == entity.PointIntentHome {
		return label + " · м. " + station
	}
	return label + " у м. " + station
}

func toZoneDTO(z zones.Zone) zoneDTO {
	return zoneDTO{ID: z.ID, Name: z.Name, NameIn: z.NameIn, NameGen: z.NameGen}
}
