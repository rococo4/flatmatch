package ru

import "strings"

const consonants = "бвгджзклмнпрстфхцчшщ"

const hushing = "гкхжчшщ"

const sibilants = "жчшщц"

func GenitiveFirstName(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) == 0 {
		return name
	}

	last := runes[len(runes)-1]
	head := string(runes)
	stem := string(runes[:len(runes)-1])
	switch {
	case last == 'а' && len(runes) > 1 && strings.ContainsRune(hushing, runes[len(runes)-2]):
		return stem + "и"
	case last == 'а':
		return stem + "ы"
	case last == 'я':
		return stem + "и"
	case last == 'й' || last == 'ь':
		return stem + "я"
	case strings.ContainsRune(consonants, last):
		return head + "а"
	default:

		return head
	}
}

func InstrumentalFirstName(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) == 0 {
		return name
	}

	last := runes[len(runes)-1]
	head := string(runes)
	stem := string(runes[:len(runes)-1])
	switch {
	case last == 'а' && len(runes) > 1 && strings.ContainsRune(sibilants, runes[len(runes)-2]):
		return stem + "ей"
	case last == 'а':
		return stem + "ой"
	case last == 'я':
		return stem + "ей"
	case last == 'й' || last == 'ь':
		return stem + "ем"
	case strings.ContainsRune(consonants, last):
		return head + "ом"
	default:
		return head
	}
}
