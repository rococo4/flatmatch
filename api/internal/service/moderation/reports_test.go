package moderation

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

func complainant() entity.User {
	return entity.User{ID: uuid.New(), Name: "Арсений"}
}

func reported() entity.User {
	return entity.User{
		ID:       uuid.New(),
		Name:     "Катя",
		Profile:  entity.Profile{Age: 24, About: "люблю тишину"},
		PhotoKey: "photos/katya.jpg",
	}
}

func TestReportFiledSendsACardWithTwoVerdicts(t *testing.T) {
	bot := &fakeSender{}
	svc := newService(t, testChat, bot, &fakeUsers{}, &fakePoints{})
	report := entity.Report{ID: uuid.New(), Reason: entity.ReasonScam}
	target := reported()

	svc.ReportFiled(t.Context(), report, complainant(), target)

	if len(bot.sent) != 1 {
		t.Fatalf("sent %d cards, want 1", len(bot.sent))
	}
	card := bot.sent[0]
	if card.chatID != testChat {
		t.Errorf("chat = %d, want the moderation chat %d", card.chatID, testChat)
	}
	if card.path != "/photos/"+target.PhotoKey {
		t.Errorf("photo = %q, want the face of the reported person", card.path)
	}

	for _, want := range []string{
		"Жалоба: Арсений → Катя (24)", "Мошенничество", "О себе цели: люблю тишину",
	} {
		if !strings.Contains(card.caption, want) {
			t.Errorf("caption %q does not carry %q", card.caption, want)
		}
	}

	want := [][2]string{
		{"Забанить", "mod:r:ban:" + report.ID.String()},
		{"Пропустить", "mod:r:skip:" + report.ID.String()},
	}
	if card.buttons[0] != want[0] || card.buttons[1] != want[1] || len(card.buttons) != 2 {
		t.Errorf("buttons = %v, want %v", card.buttons, want)
	}
}

func TestReportFiledNamesTheContext(t *testing.T) {
	flat := entity.Point{ID: uuid.New(), Type: entity.PointFlat, ZoneID: "st-sokol"}
	room := entity.Point{ID: uuid.New(), Type: entity.PointIntentHome, ZoneID: "st-sokol"}

	tests := []struct {
		name  string
		key   string
		point entity.Point
		want  string
	}{
		{"зона", "zone:d-khamovniki", entity.Point{}, "Контекст: пул Хамовников"},
		{"квартира", "point:" + flat.ID.String(), flat, "Контекст: квартира у метро Сокол"},
		{"комната", "point:" + room.ID.String(), room, "Контекст: комната у метро Сокол"},
		{"мэтч", "match:" + uuid.NewString(), entity.Point{}, "Контекст: мэтч"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := &fakeSender{}
			svc := newService(t, testChat, bot, &fakeUsers{}, &fakePoints{point: tt.point})

			svc.ReportFiled(t.Context(), entity.Report{
				ID: uuid.New(), Reason: entity.ReasonSpam, ContextKey: tt.key,
			}, complainant(), reported())

			if len(bot.sent) != 1 {
				t.Fatalf("sent %d cards, want 1", len(bot.sent))
			}
			if !strings.Contains(bot.sent[0].caption, tt.want) {
				t.Errorf("caption %q does not carry %q", bot.sent[0].caption, tt.want)
			}
		})
	}
}

func TestReportFiledWithoutContextKeepsTheCaptionShort(t *testing.T) {
	bot := &fakeSender{}
	svc := newService(t, testChat, bot, &fakeUsers{}, &fakePoints{})
	target := reported()
	target.Profile.About = ""

	svc.ReportFiled(t.Context(), entity.Report{ID: uuid.New(), Reason: entity.ReasonOther}, complainant(), target)

	if len(bot.sent) != 1 {
		t.Fatalf("sent %d cards, want 1", len(bot.sent))
	}
	caption := bot.sent[0].caption
	if strings.Contains(caption, "Контекст") {
		t.Errorf("caption %q invented a context out of nothing", caption)
	}
	if !strings.Contains(caption, "О себе цели: —") {
		t.Errorf("caption %q does not mark the missing «о себе»", caption)
	}
}
