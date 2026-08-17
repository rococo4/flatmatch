package notifications

import (
	"testing"

	"flatmatch/api/internal/entity"
)

func TestZoneNewText(t *testing.T) {
	cases := []struct {
		nameIn string
		n      int
		want   string
	}{
		{"в Хамовниках", 1, "В Хамовниках ищет квартиру ещё один человек"},
		{"в Хамовниках", 2, "В Хамовниках ищут квартиру ещё 2 человека"},
		{"в Хамовниках", 4, "В Хамовниках ищут квартиру ещё 4 человека"},
		{"на красной ветке", 5, "На красной ветке ищут квартиру ещё 5 человек"},
		{"у метро Сокол", 11, "У метро Сокол ищут квартиру ещё 11 человек"},
		// «21 человек» тянет единственное число глагола.
		{"в Хамовниках", 21, "В Хамовниках ищет квартиру ещё 21 человек"},
		{"в Хамовниках", 22, "В Хамовниках ищут квартиру ещё 22 человека"},
	}

	for _, tt := range cases {
		if got := ZoneNewText(tt.nameIn, tt.n); got != tt.want {
			t.Errorf("ZoneNewText(%q, %d) = %q, want %q", tt.nameIn, tt.n, got, tt.want)
		}
	}
}

func TestPoolNewText(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "К квартире у м. Фрунзенская присмотрелся ещё один человек"},
		{2, "К квартире у м. Фрунзенская присмотрелись ещё 2 человека"},
		{5, "К квартире у м. Фрунзенская присмотрелись ещё 5 человек"},
		{21, "К квартире у м. Фрунзенская присмотрелся ещё 21 человек"},
	}

	for _, tt := range cases {
		if got := PoolNewText("Фрунзенская", tt.n); got != tt.want {
			t.Errorf("PoolNewText(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestLikedText(t *testing.T) {
	cases := []struct {
		name   string
		gender entity.Gender
		want   string
	}{
		{"Катя", entity.GenderFemale, "Тебя лайкнула Катя"},
		{"Марк", entity.GenderMale, "Тебя лайкнул Марк"},
		// Пол обязателен в анкете (§4), но пуш не место падать из-за пустого поля.
		{"Женя", "", "Тебя лайкнул Женя"},
	}

	for _, tt := range cases {
		if got := LikedText(tt.name, tt.gender); got != tt.want {
			t.Errorf("LikedText(%q, %q) = %q, want %q", tt.name, tt.gender, got, tt.want)
		}
	}

	if got, want := LikedTailText(4), "И ещё 4 лайка за сегодня"; got != want {
		t.Errorf("LikedTailText(4) = %q, want %q", got, want)
	}
	if got, want := LikedTailText(11), "И ещё 11 лайков за сегодня"; got != want {
		t.Errorf("LikedTailText(11) = %q, want %q", got, want)
	}
}

func TestFixedTexts(t *testing.T) {
	if got, want := MatchedText(), "У тебя мэтч!"; got != want {
		t.Errorf("MatchedText() = %q, want %q", got, want)
	}

	verdicts := map[entity.NotificationType]string{
		entity.NotifProfileOK:     "Анкета прошла проверку — теперь тебя видно",
		entity.NotifPhotoRejected: "Фото не подошло — загрузи другое, где видно лицо",
		entity.NotifAboutRejected: "Текст „о себе\" не подошёл — поправь его",
		entity.NotifPointOK:       "Твоё объявление одобрено",
		entity.NotifPointRejected: "Твоё объявление отклонено",
		// У типов с параметрами текст собирают свои функции.
		entity.NotifLiked: "",
	}
	for typ, want := range verdicts {
		if got := VerdictText(typ); got != want {
			t.Errorf("VerdictText(%q) = %q, want %q", typ, got, want)
		}
	}

	if got, want := StillSearchingText("в Хамовниках"), "Всё ещё ищешь в Хамовниках?"; got != want {
		t.Errorf("StillSearchingText() = %q, want %q", got, want)
	}
	if got, want := PostMatchText("Катя"), "Как дела с Катей? Расскажи, что вышло"; got != want {
		t.Errorf("PostMatchText() = %q, want %q", got, want)
	}
}
