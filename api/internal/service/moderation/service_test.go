package moderation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

const testChat int64 = -100500

type sentPhoto struct {
	chatID  int64
	path    string
	caption string
	buttons [][2]string
}

type fakeSender struct {
	sent []sentPhoto
	err  error
}

type setCall struct {
	id     uuid.UUID
	status *entity.Status
	photo  *entity.ModerationStatus
	about  *entity.ModerationStatus
}

type fakeUsers struct {
	user    entity.User
	pending []entity.User
	sets    []setCall
}

type statusCall struct {
	id     uuid.UUID
	status entity.PointStatus
	reason *string
}

type fakePoints struct {
	pending []entity.Point
	sets    []statusCall
	changed bool
}

type fakePhotos struct{}

func (f *fakeSender) SendModerationPhoto(_ context.Context, chatID int64, path, caption string, buttons [][2]string) (int, error) {
	f.sent = append(f.sent, sentPhoto{chatID: chatID, path: path, caption: caption, buttons: buttons})
	return len(f.sent), f.err
}

func (f *fakeUsers) ByID(context.Context, uuid.UUID) (entity.User, error) {
	return f.user, nil
}

func (f *fakeUsers) SetModeration(_ context.Context, id uuid.UUID, status *entity.Status, photo, about *entity.ModerationStatus) (entity.User, error) {
	f.sets = append(f.sets, setCall{id: id, status: status, photo: photo, about: about})
	return f.user, nil
}

func (f *fakeUsers) PendingModeration(context.Context) ([]entity.User, error) {
	return f.pending, nil
}

func (f *fakePoints) SetStatus(_ context.Context, id uuid.UUID, status entity.PointStatus, reason *string) (bool, error) {
	f.sets = append(f.sets, statusCall{id: id, status: status, reason: reason})
	return f.changed, nil
}

func (f *fakePoints) PendingModeration(context.Context) ([]entity.Point, error) {
	return f.pending, nil
}

func (fakePhotos) Path(key string) (string, error) {
	if key == "broken" {
		return "", errors.New("no such file")
	}
	return "/photos/" + key, nil
}

func newService(t *testing.T, chatID int64, bot Sender, users UsersMod, points PointsMod) *Service {
	t.Helper()
	dir, err := zones.Load()
	if err != nil {
		t.Fatalf("load zones: %v", err)
	}
	return New(bot, chatID, users, points, fakePhotos{}, dir)
}

func submitted() entity.User {
	return entity.User{
		ID:          uuid.New(),
		Name:        "Катя",
		Profile:     entity.Profile{Age: 24, About: "люблю тишину"},
		PhotoKey:    "photos/katya.jpg",
		PhotoStatus: entity.ModerationPending,
		AboutStatus: entity.ModerationPending,
		Status:      entity.StatusPending,
	}
}

func TestUserVerdictsFollowTheTwoBranches(t *testing.T) {
	active := func(u entity.User) entity.User {
		u.Status = entity.StatusActive
		u.PhotoStatus, u.AboutStatus = entity.ModerationOK, entity.ModerationOK
		return u
	}

	tests := []struct {
		name string
		user func(entity.User) entity.User
		op   string
		ack  string
		want *setCall
	}{
		{
			name: "fresh questionnaire approved",
			user: func(u entity.User) entity.User { return u },
			op:   opOK, ack: ackOK,
			// «Проявляется везде разом» (§5).
			want: &setCall{
				status: pointer.To(entity.StatusActive),
				photo:  pointer.To(entity.ModerationOK),
				about:  pointer.To(entity.ModerationOK)},
		},
		{
			name: "re-checked photo approved",
			user: func(u entity.User) entity.User {
				u = active(u)
				u.PhotoStatus = entity.ModerationPending
				return u
			},
			op: opOK, ack: ackOK,
			want: &setCall{
				status: pointer.To(entity.StatusActive),
				photo:  pointer.To(entity.ModerationOK),
				about:  pointer.To(entity.ModerationOK)},
		},
		{
			// Не прошедшая проверку анкета без фото не показывается никому —
			// возврат в draft, дорога назад одна: «загрузи другое» (§5).
			name: "photo rejected before approval",
			user: func(u entity.User) entity.User { return u },
			op:   opPhoto, ack: ackOK,
			want: &setCall{
				status: pointer.To(entity.StatusDraft),
				photo:  pointer.To(entity.ModerationRejected)},
		},
		{
			// У одобренной статус не трогаем: правка фото не должна выкидывать
			// человека из пулов (§5).
			name: "photo rejected on an approved profile",
			user: func(u entity.User) entity.User {
				u = active(u)
				u.PhotoStatus = entity.ModerationPending
				return u
			},
			op: opPhoto, ack: ackOK,
			want: &setCall{photo: pointer.To(entity.ModerationRejected)},
		},
		{
			name: "text rejected before approval",
			user: func(u entity.User) entity.User { return u },
			op:   opAbout, ack: ackOK,
			want: &setCall{
				status: pointer.To(entity.StatusDraft),
				about:  pointer.To(entity.ModerationRejected)},
		},
		{
			name: "text rejected on an approved profile",
			user: func(u entity.User) entity.User {
				u = active(u)
				u.AboutStatus = entity.ModerationPending
				return u
			},
			op: opAbout, ack: ackOK,
			want: &setCall{about: pointer.To(entity.ModerationRejected)},
		},
		{
			name: "second tap on an approved questionnaire",
			user: active,
			op:   opOK, ack: ackDone,
		},
		{
			name: "second tap on an already rejected photo",
			user: func(u entity.User) entity.User {
				u.PhotoStatus = entity.ModerationRejected
				u.Status = entity.StatusDraft
				return u
			},
			op: opPhoto, ack: ackDone,
		},
		{
			// Бан — исход разбора жалобы, кнопка очереди его не отменяет (§5).
			name: "banned account",
			user: func(u entity.User) entity.User {
				u.Status = entity.StatusBanned
				return u
			},
			op: opOK, ack: ackDone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := tt.user(submitted())
			users := &fakeUsers{user: user}
			s := newService(t, testChat, &fakeSender{}, users, &fakePoints{})

			ack, err := s.HandleCallback(t.Context(), callback(subjectUser, tt.op, user.ID))
			if err != nil {
				t.Fatalf("HandleCallback() failed: %v", err)
			}
			if ack != tt.ack {
				t.Errorf("ack = %q, want %q", ack, tt.ack)
			}

			if tt.want == nil {
				if len(users.sets) != 0 {
					t.Fatalf("a handled verdict wrote %+v", users.sets)
				}
				return
			}
			if len(users.sets) != 1 {
				t.Fatalf("SetModeration called %d times, want 1", len(users.sets))
			}
			got := users.sets[0]
			if got.id != user.ID || !samePtr(got.status, tt.want.status) ||
				!samePtr(got.photo, tt.want.photo) || !samePtr(got.about, tt.want.about) {
				t.Errorf("verdict = %s / %s / %s, want %s / %s / %s",
					show(got.status), show(got.photo), show(got.about),
					show(tt.want.status), show(tt.want.photo), show(tt.want.about))
			}
		})
	}
}

func TestPointVerdicts(t *testing.T) {
	tests := []struct {
		name    string
		op      string
		changed bool
		ack     string
		status  entity.PointStatus
		reason  *string
	}{
		{"approved", opOK, true, ackOK, entity.PointActive, nil},
		{"rejected", opNo, true, ackOK, entity.PointClosed, pointer.To(rejectedReason)},
		{"second tap", opOK, false, ackDone, entity.PointActive, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := &fakePoints{changed: tt.changed}
			s := newService(t, testChat, &fakeSender{}, &fakeUsers{}, points)
			id := uuid.New()

			ack, err := s.HandleCallback(t.Context(), callback(subjectPoint, tt.op, id))
			if err != nil {
				t.Fatalf("HandleCallback() failed: %v", err)
			}
			if ack != tt.ack {
				t.Errorf("ack = %q, want %q", ack, tt.ack)
			}
			if len(points.sets) != 1 {
				t.Fatalf("SetStatus called %d times, want 1", len(points.sets))
			}
			got := points.sets[0]
			if got.id != id || got.status != tt.status || !samePtr(got.reason, tt.reason) {
				t.Errorf("verdict = %s / %s, want %s / %s",
					got.status, show(got.reason), tt.status, show(tt.reason))
			}
		})
	}
}

func TestHandleCallbackRefusesGarbage(t *testing.T) {
	id := uuid.New().String()
	for _, data := range []string{
		"", "mod", "mod:u:ok", "mod:u:ok:" + id + ":extra",
		"mod:u:ok:not-a-uuid", "mod:x:ok:" + id, "mod:u:burn:" + id,
		"mod:p:burn:" + id, "moderate:u:ok:" + id,
	} {
		t.Run(data, func(t *testing.T) {
			users, points := &fakeUsers{user: submitted()}, &fakePoints{}
			s := newService(t, testChat, &fakeSender{}, users, points)

			if _, err := s.HandleCallback(t.Context(), data); err == nil {
				t.Errorf("HandleCallback(%q) = nil error, want a refusal", data)
			}
			if len(users.sets) != 0 || len(points.sets) != 0 {
				t.Errorf("garbage wrote %+v / %+v", users.sets, points.sets)
			}
		})
	}
}

func TestUserPendingCarriesTheThreeVerdicts(t *testing.T) {
	user := submitted()
	bot := &fakeSender{}
	s := newService(t, testChat, bot, &fakeUsers{user: user}, &fakePoints{})

	s.UserPending(t.Context(), user)
	if len(bot.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(bot.sent))
	}
	got := bot.sent[0]
	if got.chatID != testChat || got.path != "/photos/photos/katya.jpg" {
		t.Errorf("message went to %d with photo %q", got.chatID, got.path)
	}
	if want := "Катя, 24\nлюблю тишину\nстатус: pending"; got.caption != want {
		t.Errorf("caption = %q, want %q", got.caption, want)
	}

	wantLabels := []string{"ок", "фото не ок", "текст не ок"}
	for i, label := range wantLabels {
		if got.buttons[i][0] != label {
			t.Errorf("button %d = %q, want %q", i, got.buttons[i][0], label)
		}
	}
	// Кнопка должна доехать обратно: данные, которые шлём, обязан понимать
	// собственный парсер.
	for _, b := range got.buttons {
		if _, err := s.HandleCallback(t.Context(), b[1]); err != nil {
			t.Errorf("HandleCallback(%q) failed: %v", b[1], err)
		}
	}

	// Правка одобренной анкеты — вторая очередь, и модератор должен видеть,
	// что смотрит не новичка.
	edited := user
	edited.Status = entity.StatusActive
	s.UserPending(t.Context(), edited)
	if want := "Катя, 24\nлюблю тишину\nстатус: правка"; bot.sent[1].caption != want {
		t.Errorf("caption of an edit = %q, want %q", bot.sent[1].caption, want)
	}
}

func TestPointPendingCaptionsByType(t *testing.T) {
	author := uuid.New()
	flat := entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Source: entity.SourceUserLink,
		CreatedBy: &author, ZoneID: "st-sportivnaya", PriceTotal: pointer.To(90000),
		Rooms: pointer.To(2), PhotoKey: "points/screen.jpg",
		URLNorm: pointer.To("https://avito.ru/moskva/kvartiry/1"),
	}
	room := entity.Point{
		ID: uuid.New(), Type: entity.PointIntentHome, Stage: pointer.To(entity.StageMovingIn),
		Source: entity.SourceUserLink, CreatedBy: &author, ZoneID: "st-sportivnaya",
		PriceShare: pointer.To(45000), PhotoKey: "points/room.jpg",
	}

	bot := &fakeSender{}
	s := newService(t, testChat, bot, &fakeUsers{}, &fakePoints{})
	s.PointPending(t.Context(), flat)
	s.PointPending(t.Context(), room)

	if len(bot.sent) != 2 {
		t.Fatalf("sent %d messages, want 2", len(bot.sent))
	}
	if want := "https://avito.ru/moskva/kvartiry/1\n90000 ₽ · м. Спортивная · 2 комн."; bot.sent[0].caption != want {
		t.Errorf("flat caption = %q, want %q", bot.sent[0].caption, want)
	}
	if want := "заезжает · ищет со-съёмщика со старта\nдоля 45000 ₽ · м. Спортивная"; bot.sent[1].caption != want {
		t.Errorf("room caption = %q, want %q", bot.sent[1].caption, want)
	}

	wantLabels := []string{"ок", "отклонить"}
	for i, label := range wantLabels {
		if bot.sent[0].buttons[i][0] != label {
			t.Errorf("button %d = %q, want %q", i, bot.sent[0].buttons[i][0], label)
		}
	}
	for _, b := range bot.sent[0].buttons {
		if _, err := s.HandleCallback(t.Context(), b[1]); err != nil {
			t.Errorf("HandleCallback(%q) failed: %v", b[1], err)
		}
	}
}

// Очередь живёт в базе, а не в чате: молчащий чат — это warn и повод для
// /queue, но не ошибка, из-за которой не создастся точка.
func TestSendingIsBestEffort(t *testing.T) {
	user := submitted()
	point := entity.Point{ID: uuid.New(), Type: entity.PointFlat, ZoneID: "st-sokol",
		PriceTotal: pointer.To(90000), PhotoKey: "points/screen.jpg"}

	t.Run("no chat configured", func(t *testing.T) {
		bot := &fakeSender{}
		s := newService(t, 0, bot, &fakeUsers{user: user}, &fakePoints{})
		s.UserPending(t.Context(), user)
		s.PointPending(t.Context(), point)
		if len(bot.sent) != 0 {
			t.Errorf("sent %d messages without a chat id", len(bot.sent))
		}
	})

	t.Run("telegram refused", func(t *testing.T) {
		bot := &fakeSender{err: errors.New("bot was blocked")}
		s := newService(t, testChat, bot, &fakeUsers{user: user}, &fakePoints{})
		s.UserPending(t.Context(), user)
		if len(bot.sent) != 1 {
			t.Errorf("sent %d messages, want the attempt to have happened", len(bot.sent))
		}
	})

	t.Run("photo is missing", func(t *testing.T) {
		bot := &fakeSender{}
		s := newService(t, testChat, bot, &fakeUsers{user: user}, &fakePoints{})
		broken := user
		broken.PhotoKey = "broken"
		s.UserPending(t.Context(), broken)
		noPhoto := point
		noPhoto.PhotoKey = ""
		s.PointPending(t.Context(), noPhoto)
		if len(bot.sent) != 0 {
			t.Errorf("sent %d messages with no photo to show", len(bot.sent))
		}
	})
}

func TestSendQueueSendsBothQueues(t *testing.T) {
	first, second := submitted(), submitted()
	second.Status = entity.StatusActive
	point := entity.Point{ID: uuid.New(), Type: entity.PointFlat, ZoneID: "st-sokol",
		PriceTotal: pointer.To(90000), PhotoKey: "points/screen.jpg",
		URLNorm: pointer.To("https://cian.ru/rent/flat/1")}

	bot := &fakeSender{}
	s := newService(t, testChat, bot,
		&fakeUsers{pending: []entity.User{first, second}},
		&fakePoints{pending: []entity.Point{point}})

	if err := s.SendQueue(t.Context()); err != nil {
		t.Fatalf("SendQueue() failed: %v", err)
	}
	if len(bot.sent) != 3 {
		t.Fatalf("sent %d messages, want both questionnaires and the point", len(bot.sent))
	}
	wantData := []string{
		callback(subjectUser, opOK, first.ID),
		callback(subjectUser, opOK, second.ID),
		callback(subjectPoint, opOK, point.ID),
	}
	for i, want := range wantData {
		if got := bot.sent[i].buttons[0][1]; got != want {
			t.Errorf("message %d approves %q, want %q", i, got, want)
		}
	}
}

func samePtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func show[T any](v *T) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprint(*v)
}
