package entity

const AboutMaxLen = 200

const (
	AgeMin = 16
	AgeMax = 100
)

type Profile struct {
	Age        int
	Gender     Gender
	GenderPref GenderPref
	Occupation Occupation
	Schedule   Schedule
	Smoking    Smoking
	PetsOwn    PetsOwn
	PetsOK     PetsOK
	Noise      Noise
	Guests     Guests
	Cleaning   Cleaning
	Alcohol    Alcohol
	Expenses   Expenses
	About      string
}

type ProfilePatch struct {
	Age        *int
	Gender     *Gender
	GenderPref *GenderPref
	Occupation *Occupation
	Schedule   *Schedule
	Smoking    *Smoking
	PetsOwn    *PetsOwn
	PetsOK     *PetsOK
	Noise      *Noise
	Guests     *Guests
	Cleaning   *Cleaning
	Alcohol    *Alcohol
	Expenses   *Expenses
	About      *string

	OnboardingStep *int
}

func (p ProfilePatch) InvalidFields() []string {
	checks := []struct {
		field string
		bad   bool
	}{
		{"age", p.Age != nil && (*p.Age < AgeMin || *p.Age > AgeMax)},
		{"gender", p.Gender != nil && *p.Gender == GenderUnknown},
		{"gender_pref", p.GenderPref != nil && *p.GenderPref == GenderPrefUnknown},
		{"occupation", p.Occupation != nil && *p.Occupation == OccupationUnknown},
		{"schedule", p.Schedule != nil && *p.Schedule == ScheduleUnknown},
		{"smoking", p.Smoking != nil && *p.Smoking == SmokingUnknown},
		{"pets_own", p.PetsOwn != nil && *p.PetsOwn == PetsOwnUnknown},
		{"pets_ok", p.PetsOK != nil && *p.PetsOK == PetsOKUnknown},
		{"noise", p.Noise != nil && *p.Noise == NoiseUnknown},
		{"guests", p.Guests != nil && *p.Guests == GuestsUnknown},
		{"cleaning", p.Cleaning != nil && *p.Cleaning == CleaningUnknown},
		{"alcohol", p.Alcohol != nil && *p.Alcohol == AlcoholUnknown},
		{"expenses", p.Expenses != nil && *p.Expenses == ExpensesUnknown},
		{"about", p.About != nil && len([]rune(*p.About)) > AboutMaxLen},
		{"onboarding_step", p.OnboardingStep != nil &&
			(*p.OnboardingStep < 0 || *p.OnboardingStep >= OnboardingSteps)},
	}

	var bad []string
	for _, c := range checks {
		if c.bad {
			bad = append(bad, c.field)
		}
	}
	return bad
}

func (p Profile) MissingFields() []string {
	answered := []struct {
		field string
		ok    bool
	}{
		{"age", p.Age != 0},
		{"gender", p.Gender != ""},
		{"gender_pref", p.GenderPref != ""},
		{"occupation", p.Occupation != ""},
		{"schedule", p.Schedule != ""},
		{"smoking", p.Smoking != ""},
		{"pets_own", p.PetsOwn != ""},
		{"pets_ok", p.PetsOK != ""},
		{"noise", p.Noise != ""},
		{"guests", p.Guests != ""},
		{"cleaning", p.Cleaning != ""},
		{"alcohol", p.Alcohol != ""},
		{"expenses", p.Expenses != ""},
		{"about", p.About != ""},
	}

	var missing []string
	for _, a := range answered {
		if !a.ok {
			missing = append(missing, a.field)
		}
	}
	return missing
}
