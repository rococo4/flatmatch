package usersrepo

import (
	"context"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/logctx"
)

const columns = `

	id, tg_id, name, tg_username,
	age, gender, gender_pref, occupation, schedule, smoking,
	pets_own, pets_ok, noise, guests, cleaning, alcohol, expenses, about,
	photo_key, photo_status, about_status,
	status, onboarding_step, source, last_seen, created_at`

type userRow struct {
	ID         uuid.UUID `db:"id"`
	TelegramID int64     `db:"tg_id"`
	Name       string    `db:"name"`
	Username   *string   `db:"tg_username"`

	Age        *int    `db:"age"`
	Gender     *string `db:"gender"`
	GenderPref string  `db:"gender_pref"`
	Occupation *string `db:"occupation"`
	Schedule   *string `db:"schedule"`
	Smoking    *string `db:"smoking"`
	PetsOwn    *string `db:"pets_own"`
	PetsOK     *string `db:"pets_ok"`
	Noise      *string `db:"noise"`
	Guests     *string `db:"guests"`
	Cleaning   *string `db:"cleaning"`
	Alcohol    *string `db:"alcohol"`
	Expenses   *string `db:"expenses"`
	About      *string `db:"about"`

	PhotoKey    *string `db:"photo_key"`
	PhotoStatus string  `db:"photo_status"`
	AboutStatus string  `db:"about_status"`

	Status         string  `db:"status"`
	OnboardingStep int     `db:"onboarding_step"`
	Source         *string `db:"source"`

	LastSeen  time.Time `db:"last_seen"`
	CreatedAt time.Time `db:"created_at"`
}

func (r userRow) toEntity(ctx context.Context) entity.User {
	user := entity.User{
		ID:         r.ID,
		TelegramID: r.TelegramID,
		Name:       r.Name,
		Username:   pointer.Get(r.Username),

		Profile: entity.Profile{
			Age:        pointer.Get(r.Age),
			Gender:     entity.ParseGender(pointer.Get(r.Gender)),
			GenderPref: entity.ParseGenderPref(r.GenderPref),
			Occupation: entity.ParseOccupation(pointer.Get(r.Occupation)),
			Schedule:   entity.ParseSchedule(pointer.Get(r.Schedule)),
			Smoking:    entity.ParseSmoking(pointer.Get(r.Smoking)),
			PetsOwn:    entity.ParsePetsOwn(pointer.Get(r.PetsOwn)),
			PetsOK:     entity.ParsePetsOK(pointer.Get(r.PetsOK)),
			Noise:      entity.ParseNoise(pointer.Get(r.Noise)),
			Guests:     entity.ParseGuests(pointer.Get(r.Guests)),
			Cleaning:   entity.ParseCleaning(pointer.Get(r.Cleaning)),
			Alcohol:    entity.ParseAlcohol(pointer.Get(r.Alcohol)),
			Expenses:   entity.ParseExpenses(pointer.Get(r.Expenses)),
			About:      pointer.Get(r.About),
		},

		PhotoKey:    pointer.Get(r.PhotoKey),
		PhotoStatus: entity.ParseModerationStatus(r.PhotoStatus),
		AboutStatus: entity.ParseModerationStatus(r.AboutStatus),

		Status:         entity.ParseStatus(r.Status),
		OnboardingStep: r.OnboardingStep,
		Source:         pointer.Get(r.Source),

		LastSeen:  r.LastSeen,
		CreatedAt: r.CreatedAt,
	}

	if bad := unreadable(user); len(bad) > 0 {
		logctx.From(ctx).Warn("unreadable enum values, those fields are hidden",
			"user_id", r.ID, "fields", bad)
	}
	return user
}

func unreadable(u entity.User) []string {
	checks := []struct {
		field string
		bad   bool
	}{
		{"status", u.Status == entity.StatusUnknown},
		{"photo_status", u.PhotoStatus == entity.ModerationUnknown},
		{"about_status", u.AboutStatus == entity.ModerationUnknown},
		{"gender", u.Profile.Gender == entity.GenderUnknown},
		{"gender_pref", u.Profile.GenderPref == entity.GenderPrefUnknown},
		{"occupation", u.Profile.Occupation == entity.OccupationUnknown},
		{"schedule", u.Profile.Schedule == entity.ScheduleUnknown},
		{"smoking", u.Profile.Smoking == entity.SmokingUnknown},
		{"pets_own", u.Profile.PetsOwn == entity.PetsOwnUnknown},
		{"pets_ok", u.Profile.PetsOK == entity.PetsOKUnknown},
		{"noise", u.Profile.Noise == entity.NoiseUnknown},
		{"guests", u.Profile.Guests == entity.GuestsUnknown},
		{"cleaning", u.Profile.Cleaning == entity.CleaningUnknown},
		{"alcohol", u.Profile.Alcohol == entity.AlcoholUnknown},
		{"expenses", u.Profile.Expenses == entity.ExpensesUnknown},
	}

	var bad []string
	for _, c := range checks {
		if c.bad {
			bad = append(bad, c.field)
		}
	}
	return bad
}
