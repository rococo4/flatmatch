package peopleapi

import (
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/ru"
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

// pointDTO is the mini-card of «пересеклись: {точка}»: the label is ready to
// print, the kind colours the card's header (§6). Its twin lives in the match
// summary — the same phrase in the two places the context is retold.
type pointDTO struct {
	ID    uuid.UUID `json:"id"`
	Kind  string    `json:"kind"`
	Label string    `json:"label"`
}

// profileResponse is the whole profile screen (§6) — and it carries no way to
// reach the person: tg_username and tg_id live in the match detail and nowhere
// else (§3 приватность структурно). dto_test pins that.
type profileResponse struct {
	UserID   uuid.UUID `json:"user_id"`
	Name     string    `json:"name"`
	Age      int       `json:"age"`
	PhotoURL string    `json:"photo_url"`

	Occupation string `json:"occupation"`
	Schedule   string `json:"schedule"`
	Smoking    string `json:"smoking"`
	PetsOwn    string `json:"pets_own"`
	PetsOK     string `json:"pets_ok"`
	Noise      string `json:"noise"`
	Guests     string `json:"guests"`
	Cleaning   string `json:"cleaning"`
	Alcohol    string `json:"alcohol"`
	Expenses   string `json:"expenses"`
	About      string `json:"about,omitempty"`

	Shared []sharedDTO `json:"shared"`
	// ContextZone is the pool the profile was opened from — «пересеклись:
	// {зона}» — absent when the screen was reached from anywhere else.
	ContextZone *zoneDTO `json:"context_zone,omitempty"`
	// ContextPoint is the same line when the screen was opened from a point —
	// пул квартиры или карточка «к себе».
	ContextPoint *pointDTO `json:"context_point,omitempty"`

	LikedMe bool      `json:"liked_me"`
	MyLike  bool      `json:"my_like"`
	Budget  budgetDTO `json:"budget"`
}

type sharedDTO struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// budgetDTO backs «3 из 5» under the button; next_free_at is absent while
// nothing is spent, because then nothing has to come back.
type budgetDTO struct {
	Remaining  int        `json:"remaining"`
	NextFreeAt *time.Time `json:"next_free_at,omitempty"`
}

func toProfile(
	viewer, target entity.User,
	photoURL string, zone *zoneDTO, point *pointDTO,
	myLike, likedMe bool,
	budget entity.LikeBudget,
) profileResponse {
	p := target.Profile
	out := profileResponse{
		UserID:   target.ID,
		Name:     target.Name,
		Age:      p.Age,
		PhotoURL: photoURL,

		Occupation: string(p.Occupation),
		Schedule:   string(p.Schedule),
		Smoking:    string(p.Smoking),
		PetsOwn:    string(p.PetsOwn),
		PetsOK:     string(p.PetsOK),
		Noise:      string(p.Noise),
		Guests:     string(p.Guests),
		Cleaning:   string(p.Cleaning),
		Alcohol:    string(p.Alcohol),
		Expenses:   string(p.Expenses),

		Shared:       toShared(viewer.Profile.Shared(p)),
		ContextZone:  zone,
		ContextPoint: point,

		LikedMe: likedMe,
		MyLike:  myLike,
		Budget:  budgetDTO{Remaining: budget.Remaining, NextFreeAt: budget.NextFreeAt},
	}
	// «О себе» travels only once moderation has approved it: until then the
	// block is not rendered at all (§5), so the field is simply not there.
	if target.AboutApproved() {
		out.About = p.About
	}
	return out
}

func toShared(answers []entity.SharedAnswer) []sharedDTO {
	out := make([]sharedDTO, 0, len(answers))
	for _, a := range answers {
		out = append(out, sharedDTO{Field: a.Field, Value: a.Value})
	}
	return out
}

// pointLabel is the twin of the match summary's: «на комнату Кати · м. Сокол»
// у «к себе», «квартира у м. Сокол» у квартиры с рынка. Имя автора и станция
// могут не приехать — строка тогда просто короче.
func pointLabel(kind entity.PointType, author, station string) string {
	if kind == entity.PointIntentHome {
		room := "на комнату"
		if author != "" {
			room += " " + ru.GenitiveFirstName(author)
		}
		if station == "" {
			return room
		}
		return room + " · м. " + station
	}
	if station == "" {
		return "квартира"
	}
	return "квартира у м. " + station
}

func toZoneDTO(z zones.Zone) zoneDTO {
	return zoneDTO{ID: z.ID, Name: z.Name, NameIn: z.NameIn, NameGen: z.NameGen}
}
