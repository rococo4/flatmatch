package usersapi

import (
	"time"

	"github.com/AlekSi/pointer"

	"flatmatch/api/internal/entity"
)

type ownProfile struct {
	ID         string `json:"id"`
	TelegramID int64  `json:"tg_id"`
	Name       string `json:"name"`
	Username   string `json:"tg_username,omitempty"`

	Age        int               `json:"age,omitempty"`
	Gender     entity.Gender     `json:"gender,omitempty"`
	GenderPref entity.GenderPref `json:"gender_pref,omitempty"`
	Occupation entity.Occupation `json:"occupation,omitempty"`
	Schedule   entity.Schedule   `json:"schedule,omitempty"`
	Smoking    entity.Smoking    `json:"smoking,omitempty"`
	PetsOwn    entity.PetsOwn    `json:"pets_own,omitempty"`
	PetsOK     entity.PetsOK     `json:"pets_ok,omitempty"`
	Noise      entity.Noise      `json:"noise,omitempty"`
	Guests     entity.Guests     `json:"guests,omitempty"`
	Cleaning   entity.Cleaning   `json:"cleaning,omitempty"`
	Alcohol    entity.Alcohol    `json:"alcohol,omitempty"`
	Expenses   entity.Expenses   `json:"expenses,omitempty"`
	About      string            `json:"about,omitempty"`

	PhotoURL    string                  `json:"photo_url,omitempty"`
	PhotoStatus entity.ModerationStatus `json:"photo_status"`
	AboutStatus entity.ModerationStatus `json:"about_status"`

	Status         entity.Status `json:"status"`
	OnboardingStep int           `json:"onboarding_step"`
	CreatedAt      time.Time     `json:"created_at"`

	BotUsername string `json:"bot_username,omitempty"`
}

type patchRequest struct {
	Age        *int    `json:"age"`
	Gender     *string `json:"gender"`
	GenderPref *string `json:"gender_pref"`
	Occupation *string `json:"occupation"`
	Schedule   *string `json:"schedule"`
	Smoking    *string `json:"smoking"`
	PetsOwn    *string `json:"pets_own"`
	PetsOK     *string `json:"pets_ok"`
	Noise      *string `json:"noise"`
	Guests     *string `json:"guests"`
	Cleaning   *string `json:"cleaning"`
	Alcohol    *string `json:"alcohol"`
	Expenses   *string `json:"expenses"`
	About      *string `json:"about"`

	OnboardingStep *int `json:"onboarding_step"`
}

func toOwnProfile(u entity.User, photoURL, botUsername string) ownProfile {
	return ownProfile{
		ID:         u.ID.String(),
		TelegramID: u.TelegramID,
		Name:       u.Name,
		Username:   u.Username,

		Age:        u.Profile.Age,
		Gender:     u.Profile.Gender,
		GenderPref: u.Profile.GenderPref,
		Occupation: u.Profile.Occupation,
		Schedule:   u.Profile.Schedule,
		Smoking:    u.Profile.Smoking,
		PetsOwn:    u.Profile.PetsOwn,
		PetsOK:     u.Profile.PetsOK,
		Noise:      u.Profile.Noise,
		Guests:     u.Profile.Guests,
		Cleaning:   u.Profile.Cleaning,
		Alcohol:    u.Profile.Alcohol,
		Expenses:   u.Profile.Expenses,
		About:      u.Profile.About,

		PhotoURL:    photoURL,
		PhotoStatus: u.PhotoStatus,
		AboutStatus: u.AboutStatus,

		Status:         u.Status,
		OnboardingStep: u.OnboardingStep,
		CreatedAt:      u.CreatedAt,

		BotUsername: botUsername,
	}
}

func (p patchRequest) toPatch() entity.ProfilePatch {
	return entity.ProfilePatch{
		Age:        p.Age,
		Gender:     parsed(p.Gender, entity.ParseGender),
		GenderPref: parsed(p.GenderPref, entity.ParseGenderPref),
		Occupation: parsed(p.Occupation, entity.ParseOccupation),
		Schedule:   parsed(p.Schedule, entity.ParseSchedule),
		Smoking:    parsed(p.Smoking, entity.ParseSmoking),
		PetsOwn:    parsed(p.PetsOwn, entity.ParsePetsOwn),
		PetsOK:     parsed(p.PetsOK, entity.ParsePetsOK),
		Noise:      parsed(p.Noise, entity.ParseNoise),
		Guests:     parsed(p.Guests, entity.ParseGuests),
		Cleaning:   parsed(p.Cleaning, entity.ParseCleaning),
		Alcohol:    parsed(p.Alcohol, entity.ParseAlcohol),
		Expenses:   parsed(p.Expenses, entity.ParseExpenses),
		About:      p.About,

		OnboardingStep: p.OnboardingStep,
	}
}

func parsed[T ~string](raw *string, parse func(string) T) *T {
	if raw == nil {
		return nil
	}
	return pointer.To(parse(*raw))
}
