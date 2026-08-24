package entity

import "slices"

const unknown = "unknown"

type Gender string

const (
	GenderUnknown Gender = unknown
	GenderMale    Gender = "male"
	GenderFemale  Gender = "female"
)

type GenderPref string

const (
	GenderPrefUnknown GenderPref = unknown
	GenderPrefAny     GenderPref = "any"
	GenderPrefSame    GenderPref = "same"
)

type Occupation string

const (
	OccupationUnknown Occupation = unknown
	OccupationStudy   Occupation = "study"
	OccupationOffice  Occupation = "office"
	OccupationHybrid  Occupation = "hybrid"
	OccupationRemote  Occupation = "remote"
	OccupationShift   Occupation = "shift"
	OccupationNone    Occupation = "none"
)

type Schedule string

const (
	ScheduleUnknown  Schedule = unknown
	ScheduleOwl      Schedule = "owl"
	ScheduleLark     Schedule = "lark"
	ScheduleFloating Schedule = "floating"
)

type Smoking string

const (
	SmokingUnknown Smoking = unknown
	SmokingNo      Smoking = "no"
	SmokingOutside Smoking = "outside"
	SmokingHome    Smoking = "home"
)

type PetsOwn string

const (
	PetsOwnUnknown PetsOwn = unknown
	PetsOwnNone    PetsOwn = "none"
	PetsOwnCat     PetsOwn = "cat"
	PetsOwnDog     PetsOwn = "dog"
	PetsOwnOther   PetsOwn = "other"
)

type PetsOK string

const (
	PetsOKUnknown PetsOK = unknown
	PetsOKFine    PetsOK = "ok"
	PetsOKAllergy PetsOK = "allergy"
	PetsOKAgainst PetsOK = "against"
)

type Noise string

const (
	NoiseUnknown Noise = unknown
	NoiseSilence Noise = "silence"
	NoiseNormal  Noise = "normal"
	NoiseLoud    Noise = "loud"
)

type Guests string

const (
	GuestsUnknown   Guests = unknown
	GuestsRare      Guests = "rare"
	GuestsSometimes Guests = "sometimes"
	GuestsOften     Guests = "often"
	GuestsPartner   Guests = "partner"
)

type Cleaning string

const (
	CleaningUnknown      Cleaning = unknown
	CleaningSchedule     Cleaning = "schedule"
	CleaningWhenDirty    Cleaning = "when_dirty"
	CleaningCreativeMess Cleaning = "creative_mess"
)

type Alcohol string

const (
	AlcoholUnknown      Alcohol = unknown
	AlcoholNone         Alcohol = "none"
	AlcoholWithNeighbor Alcohol = "with_neighbor"
	AlcoholParties      Alcohol = "parties"
)

type Expenses string

const (
	ExpensesUnknown      Expenses = unknown
	ExpensesSplitEqually Expenses = "split_equally"
	ExpensesTracked      Expenses = "tracked"
	ExpensesAsWeGo       Expenses = "as_we_go"
)

func ParseGender(raw string) Gender {
	return parseEnum(raw, GenderMale, GenderFemale)
}

func ParseGenderPref(raw string) GenderPref {
	return parseEnum(raw, GenderPrefAny, GenderPrefSame)
}

func ParseOccupation(raw string) Occupation {
	return parseEnum(raw,
		OccupationStudy, OccupationOffice, OccupationHybrid,
		OccupationRemote, OccupationShift, OccupationNone,
	)
}

func ParseSchedule(raw string) Schedule {
	return parseEnum(raw, ScheduleOwl, ScheduleLark, ScheduleFloating)
}

func ParseSmoking(raw string) Smoking {
	return parseEnum(raw, SmokingNo, SmokingOutside, SmokingHome)
}

func ParsePetsOwn(raw string) PetsOwn {
	return parseEnum(raw, PetsOwnNone, PetsOwnCat, PetsOwnDog, PetsOwnOther)
}

func ParsePetsOK(raw string) PetsOK {
	return parseEnum(raw, PetsOKFine, PetsOKAllergy, PetsOKAgainst)
}

func ParseNoise(raw string) Noise {
	return parseEnum(raw, NoiseSilence, NoiseNormal, NoiseLoud)
}

func ParseGuests(raw string) Guests {
	return parseEnum(raw, GuestsRare, GuestsSometimes, GuestsOften, GuestsPartner)
}

func ParseCleaning(raw string) Cleaning {
	return parseEnum(raw, CleaningSchedule, CleaningWhenDirty, CleaningCreativeMess)
}

func ParseAlcohol(raw string) Alcohol {
	return parseEnum(raw, AlcoholNone, AlcoholWithNeighbor, AlcoholParties)
}

func ParseExpenses(raw string) Expenses {
	return parseEnum(raw, ExpensesSplitEqually, ExpensesTracked, ExpensesAsWeGo)
}

func parseEnum[T ~string](raw string, allowed ...T) T {
	if raw == "" {
		return T("")
	}
	if slices.Contains(allowed, T(raw)) {
		return T(raw)
	}
	return T(unknown)
}
