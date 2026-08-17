package ru

// Plural выбирает форму существительного при числе: «1 человек», «2 человека»,
// «5 человек». Правило русское, а не английское: 21 берёт форму one, 14 — many.
func Plural(n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	mod10, mod100 := n%10, n%100
	switch {
	case mod10 == 1 && mod100 != 11:
		return one
	case mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14):
		return few
	default:
		return many
	}
}
