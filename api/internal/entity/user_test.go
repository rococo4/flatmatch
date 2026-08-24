package entity

import (
	"slices"
	"testing"
)

func TestMissingFieldsWantsAtLeastOnePhoto(t *testing.T) {
	user := User{Profile: completeAnswers()}

	if !slices.Contains(user.MissingFields(), "photo") {
		t.Errorf("MissingFields() = %v, want it to ask for a photo", user.MissingFields())
	}

	user.PhotoKeys = []string{"photos/a.jpg"}
	if got := user.MissingFields(); len(got) > 0 {
		t.Errorf("MissingFields() = %v, want nothing missing once a photo is there", got)
	}
}

func TestPhotoApprovedNeedsBothAVerdictAndAFile(t *testing.T) {
	cases := []struct {
		name string
		user User
		want bool
	}{
		{"approved set", User{PhotoKeys: []string{"photos/a.jpg"}, PhotoStatus: ModerationOK}, true},
		{"unreviewed set", User{PhotoKeys: []string{"photos/a.jpg"}, PhotoStatus: ModerationPending}, false},
		{"verdict without files", User{PhotoStatus: ModerationOK}, false},
	}

	for _, c := range cases {
		if got := c.user.PhotoApproved(); got != c.want {
			t.Errorf("%s: PhotoApproved() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestReorderPhotosPermutesTheSet(t *testing.T) {
	keys := []string{"a", "b", "c"}

	got, ok := ReorderPhotos(keys, []int{2, 0, 1})
	if !ok {
		t.Fatal("ReorderPhotos() refused a valid permutation")
	}
	if !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Errorf("ReorderPhotos() = %v, want [c a b]", got)
	}
	if !slices.Equal(keys, []string{"a", "b", "c"}) {
		t.Errorf("the original set changed to %v", keys)
	}
}

func TestReorderPhotosRefusesWhatIsNotAPermutation(t *testing.T) {
	keys := []string{"a", "b", "c"}
	bad := [][]int{
		{0, 1},
		{0, 1, 2, 0},
		{0, 1, 1},
		{0, 1, 3},
		{-1, 1, 2},
		nil,
	}

	for _, order := range bad {
		if got, ok := ReorderPhotos(keys, order); ok {
			t.Errorf("ReorderPhotos(%v) = %v, want it refused", order, got)
		}
	}
}

func TestRemovePhotoDropsOneByIndex(t *testing.T) {
	keys := []string{"a", "b", "c"}

	got, ok := RemovePhoto(keys, 1)
	if !ok {
		t.Fatal("RemovePhoto() refused a valid index")
	}
	if !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("RemovePhoto() = %v, want [a c]", got)
	}
	if !slices.Equal(keys, []string{"a", "b", "c"}) {
		t.Errorf("the original set changed to %v", keys)
	}

	if _, ok := RemovePhoto(keys, 3); ok {
		t.Error("RemovePhoto() accepted an index past the end")
	}
	if _, ok := RemovePhoto(keys, -1); ok {
		t.Error("RemovePhoto() accepted a negative index")
	}
	if _, ok := RemovePhoto(nil, 0); ok {
		t.Error("RemovePhoto() accepted an index into an empty set")
	}
}

func TestPhotoSetIsCappedAtFive(t *testing.T) {
	if MaxUserPhotos != 5 {
		t.Errorf("MaxUserPhotos = %d, want 5", MaxUserPhotos)
	}
}

func completeAnswers() Profile {
	return Profile{
		Age:        28,
		Gender:     GenderMale,
		GenderPref: GenderPrefAny,
		Occupation: OccupationNone,
		Schedule:   ScheduleOwl,
		Smoking:    SmokingNo,
		PetsOwn:    PetsOwnCat,
		PetsOK:     PetsOKFine,
		Noise:      NoiseNormal,
		Guests:     GuestsRare,
		Cleaning:   CleaningSchedule,
		Alcohol:    AlcoholNone,
		Expenses:   ExpensesSplitEqually,
		About:      "Люблю тишину по утрам",
	}
}
