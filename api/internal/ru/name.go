// Package ru holds the little Russian morphology the product writes by hand.
//
// «На комнату Кати» (§6) — родительный падеж имени. Полноценная морфология
// ради одной подписи не нужна: имена приходят из Telegram, это почти всегда
// первое имя, и правил склонения для них четыре.
package ru

import "strings"

const consonants = "бвгджзклмнпрстфхцчшщ"

// hushing letters turn the -а ending into -и rather than -ы: «Саша → Саши»,
// «Ольга → Ольги».
const hushing = "гкхжчшщ"

// sibilants — только шипящие. В творительном падеже они дают -ей («Саша →
// Сашей»), а г/к/х из hushing ведут себя как обычные согласные («Ольга →
// Ольгой»), поэтому один список на два падежа не годится.
const sibilants = "жчшщц"

// GenitiveFirstName склоняет имя в родительный падеж. Неизвестное окончание —
// латиница, эмодзи, заглавная буква — возвращается как есть: лучше «на комнату
// Bob», чем выдуманное «Boba».
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
		// Гласная на конце (Али, Нино) и всё нерусское — несклоняемое.
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
