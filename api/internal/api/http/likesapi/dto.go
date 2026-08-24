package likesapi

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/service/deck"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

type likeRequest struct {
	ToID string `json:"to_id"`

	ZoneID string `json:"zone_id"`

	PointID string `json:"point_id"`
}

type likeResponse struct {
	Matched bool       `json:"matched"`
	MatchID *uuid.UUID `json:"match_id,omitempty"`

	Already bool      `json:"already"`
	Budget  budgetDTO `json:"budget"`
}

type budgetDTO struct {
	Remaining  int        `json:"remaining"`
	NextFreeAt *time.Time `json:"next_free_at,omitempty"`
}

type chipDTO struct {
	Param   string `json:"param"`
	Verdict string `json:"verdict"`
}

type pairDTO struct {
	Zone      *zoneDTO  `json:"zone,omitempty"`
	ZoneMatch bool      `json:"zone_match"`
	Section   string    `json:"section"`
	Chips     []chipDTO `json:"chips,omitempty"`
	Room      bool      `json:"room,omitempty"`
}

type pointDTO struct {
	ID    uuid.UUID `json:"id"`
	Kind  string    `json:"kind"`
	Label string    `json:"label"`
}

type incomingDTO struct {
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Age       int       `json:"age"`
	PhotoURLs []string  `json:"photo_urls"`
	Schedule  string    `json:"schedule"`
	Smoking   string    `json:"smoking"`
	PetsOwn   string    `json:"pets_own"`

	Zone         *zoneDTO  `json:"zone,omitempty"`
	Point        *pointDTO `json:"point,omitempty"`
	Pair         pairDTO   `json:"pair"`
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

func toIncoming(like likesrepo.IncomingLike, photoURLs []string,
	zone *zoneDTO, point *pointDTO, pair pairDTO,
) incomingDTO {
	return incomingDTO{
		UserID:    like.From.ID,
		Name:      like.From.Name,
		Age:       like.From.Profile.Age,
		PhotoURLs: photoURLs,
		Schedule:  string(like.From.Profile.Schedule),
		Smoking:   string(like.From.Profile.Smoking),
		PetsOwn:   string(like.From.Profile.PetsOwn),

		Zone:         zone,
		Point:        point,
		Pair:         pair,
		NotSearching: like.NotSearching,
		At:           like.CreatedAt,
	}
}

func toPairDTO(pair *deck.Pair, point *pointDTO, roomZone *zoneDTO) pairDTO {
	out := pairDTO{Section: string(pools.SectionNone)}
	if pair != nil {
		out.ZoneMatch = pair.ZoneMatch
		out.Section = string(pair.Section)
		out.Zone = pointer.To(toZoneDTO(pair.Zone))
		for _, chip := range pair.Chips {
			out.Chips = append(out.Chips, chipDTO{Param: string(chip.Param), Verdict: string(chip.Verdict)})
		}
	}
	if point != nil && point.Kind == string(entity.PointIntentHome) {
		out.Room = true
		out.Zone = roomZone
	}
	return out
}

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
