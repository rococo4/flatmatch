package notifications

import (
	"fmt"
	"unicode"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/ru"
)

func ZoneNewText(nameIn string, n int) string {
	if n == 1 {
		return capitalized(nameIn) + " ищет квартиру ещё один человек"
	}
	return fmt.Sprintf("%s %s квартиру ещё %d %s",
		capitalized(nameIn), verb(n, "ищет", "ищут"), n, people(n))
}

func PoolNewText(station string, n int) string {
	if n == 1 {
		return "К квартире у м. " + station + " присмотрелся ещё один человек"
	}
	return fmt.Sprintf("К квартире у м. %s %s ещё %d %s",
		station, verb(n, "присмотрелся", "присмотрелись"), n, people(n))
}

func LikedText(name string, gender entity.Gender) string {
	if gender == entity.GenderFemale {
		return "Тебя лайкнула " + name
	}
	return "Тебя лайкнул " + name
}

func LikedTailText(n int) string {
	return fmt.Sprintf("И ещё %d %s за сегодня", n, ru.Plural(n, "лайк", "лайка", "лайков"))
}

func MatchedText() string {
	return "У тебя мэтч!"
}

func VerdictText(typ entity.NotificationType) string {
	switch typ {
	case entity.NotifProfileOK:
		return "Анкета прошла проверку — теперь тебя видно"
	case entity.NotifPhotoRejected:
		return "Фото не подошло — загрузи другое, где видно лицо"
	case entity.NotifAboutRejected:
		return "Текст „о себе\" не подошёл — поправь его"
	case entity.NotifPointOK:
		return "Твоё объявление одобрено"
	case entity.NotifPointRejected:
		return "Твоё объявление отклонено"
	default:
		return ""
	}
}

func StillSearchingText(nameIn string) string {
	return "Всё ещё ищешь " + nameIn + "?"
}

func PostMatchText(peerName string) string {
	return "Как дела с " + ru.InstrumentalFirstName(peerName) + "? Расскажи, что вышло"
}

func PointGoneText(nameIn string) string {
	return "Квартира " + nameIn + ", похоже, сдана"
}

func PointGoneOwnText(nameIn string) string {
	return "Твою квартиру " + nameIn + " отметили как сданную — проверь"
}

func PointPingText(kind entity.PointType, nameIn string) string {
	if kind == entity.PointIntentHome {
		return "Комната " + nameIn + " ещё свободна?"
	}
	return "Квартира " + nameIn + " ещё актуальна?"
}

func people(n int) string {
	return ru.Plural(n, "человек", "человека", "человек")
}

func verb(n int, singular, plural string) string {
	return ru.Plural(n, singular, plural, plural)
}

func capitalized(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
