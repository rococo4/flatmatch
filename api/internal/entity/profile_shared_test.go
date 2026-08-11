package entity

import (
	"slices"
	"testing"
)

func filledProfile() Profile {
	return Profile{
		Schedule: ScheduleOwl,
		Smoking:  SmokingNo,
		PetsOwn:  PetsOwnCat,
		PetsOK:   PetsOKFine,
		Noise:    NoiseSilence,
		Guests:   GuestsRare,
		Cleaning: CleaningSchedule,
		Alcohol:  AlcoholNone,
		Expenses: ExpensesSplitEqually,
	}
}

func TestSharedAnswers(t *testing.T) {
	a := Profile{Schedule: ScheduleOwl, Smoking: SmokingNo, Noise: NoiseSilence}
	b := Profile{Schedule: ScheduleOwl, Smoking: SmokingNo, Noise: NoiseLoud}
	if got := a.SharedAnswers(b); got != 2 {
		t.Errorf("SharedAnswers() = %d, want 2", got)
	}
	// Пустые поля не совпадение: две недозаполненные анкеты дают 0, не 9.
	if got := (Profile{}).SharedAnswers(Profile{}); got != 0 {
		t.Errorf("empty SharedAnswers() = %d, want 0", got)
	}
}

// Ровно девять полей привычек из §4 — возраст, пол, занятость и «о себе»
// в счётчик «N совпадений анкеты» не входят.
func TestSharedAnswersCountsNineHabitFields(t *testing.T) {
	a, b := filledProfile(), filledProfile()
	a.Age, b.Age = 25, 25
	a.Gender, b.Gender = GenderMale, GenderMale
	a.GenderPref, b.GenderPref = GenderPrefAny, GenderPrefAny
	a.Occupation, b.Occupation = OccupationOffice, OccupationOffice
	a.About, b.About = "same text", "same text"

	if got := a.SharedAnswers(b); got != 9 {
		t.Errorf("SharedAnswers() = %d, want 9", got)
	}
}

func TestSharedAnswersEmptyOnOneSideNeverCounts(t *testing.T) {
	full := filledProfile()
	if got := full.SharedAnswers(Profile{}); got != 0 {
		t.Errorf("full vs empty SharedAnswers() = %d, want 0", got)
	}
	if got := (Profile{}).SharedAnswers(full); got != 0 {
		t.Errorf("empty vs full SharedAnswers() = %d, want 0", got)
	}
}

// Саммари мэтча называет совпавшие ответы («вы оба совы»), поэтому Shared
// отдаёт именно их — поле и значение, в порядке анкеты.
func TestSharedNamesTheMatchedFields(t *testing.T) {
	a := Profile{Schedule: ScheduleOwl, Smoking: SmokingNo, Noise: NoiseSilence}
	b := Profile{Schedule: ScheduleOwl, Smoking: SmokingNo, Noise: NoiseLoud}

	want := []SharedAnswer{
		{Field: "schedule", Value: string(ScheduleOwl)},
		{Field: "smoking", Value: string(SmokingNo)},
	}
	if got := a.Shared(b); !slices.Equal(got, want) {
		t.Errorf("Shared() = %v, want %v", got, want)
	}
}

func TestSharedAnswersCountsShared(t *testing.T) {
	a, b := filledProfile(), filledProfile()
	b.Noise = NoiseLoud
	b.Alcohol = ""

	if got, want := a.SharedAnswers(b), len(a.Shared(b)); got != want {
		t.Errorf("SharedAnswers() = %d, want len(Shared()) = %d", got, want)
	}
}

func TestSharedIsEmptyWithoutMatches(t *testing.T) {
	if got := filledProfile().Shared(Profile{}); len(got) != 0 {
		t.Errorf("Shared() = %v, want empty", got)
	}
}

func TestSharedAnswersIsSymmetric(t *testing.T) {
	a := filledProfile()
	b := filledProfile()
	b.Noise = NoiseLoud
	b.Guests = GuestsOften
	b.Alcohol = ""

	if got, want := a.SharedAnswers(b), 6; got != want {
		t.Errorf("a.SharedAnswers(b) = %d, want %d", got, want)
	}
	if a.SharedAnswers(b) != b.SharedAnswers(a) {
		t.Errorf("SharedAnswers not symmetric: %d vs %d", a.SharedAnswers(b), b.SharedAnswers(a))
	}
}
