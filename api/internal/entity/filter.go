package entity

type PeopleFilter struct {
	AgeMin *int
	AgeMax *int

	Occupation []string
	Schedule   []string
	Smoking    []string

	PetsOwnNone bool
	PetsOKFine  bool
}
