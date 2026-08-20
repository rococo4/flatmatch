package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/repo/notificationsrepo"
	"flatmatch/api/internal/tgbot"
)

type clock struct{ at time.Time }

func (c *clock) now() time.Time { return c.at }

func (c *clock) add(d time.Duration) { c.at = c.at.Add(d) }

type fakeQueue struct {
	rows  []*entity.Notification
	tails map[uuid.UUID]notificationsrepo.TailMessage

	pendingErr error
	sentAtErr  error
}

func newQueue(rows ...entity.Notification) *fakeQueue {
	q := &fakeQueue{tails: map[uuid.UUID]notificationsrepo.TailMessage{}}
	for _, row := range rows {
		q.put(row)
	}
	return q
}

func (q *fakeQueue) put(row entity.Notification) *entity.Notification {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Unix(int64(len(q.rows)), 0).UTC()
	}
	q.rows = append(q.rows, &row)
	return &row
}

func (q *fakeQueue) Pending(_ context.Context, limit int) ([]entity.Notification, error) {
	if q.pendingErr != nil {
		return nil, q.pendingErr
	}
	var out []entity.Notification
	for _, row := range q.rows {
		if row.SentAt == nil && row.FailedAt == nil && len(out) < limit {
			out = append(out, *row)
		}
	}
	return out, nil
}

func (q *fakeQueue) PendingPersonal(ctx context.Context, limit int) ([]entity.Notification, error) {
	all, err := q.Pending(ctx, limit)
	if err != nil {
		return nil, err
	}
	var out []entity.Notification
	for _, n := range all {
		if !n.Type.Aggregated() {
			out = append(out, n)
		}
	}
	return out, nil
}

func (q *fakeQueue) Insert(_ context.Context, n entity.Notification) (bool, error) {
	for _, row := range q.rows {
		if row.UserID == n.UserID && row.Type == n.Type &&
			row.ActorKey == n.ActorKey && row.ContextKey == n.ContextKey {
			return false, nil
		}
	}
	q.put(n)
	return true, nil
}

func (q *fakeQueue) MarkSent(_ context.Context, ids []uuid.UUID, at time.Time) error {
	for _, row := range q.rows {
		if slices.Contains(ids, row.ID) && row.SentAt == nil {
			sent := at
			row.SentAt = &sent
		}
	}
	return nil
}

func (q *fakeQueue) MarkFailed(_ context.Context, ids []uuid.UUID) error {
	for _, row := range q.rows {
		if slices.Contains(ids, row.ID) && row.SentAt == nil {
			failed := time.Now()
			row.FailedAt = &failed
		}
	}
	return nil
}

func (q *fakeQueue) Bump(_ context.Context, id uuid.UUID) error {
	for _, row := range q.rows {
		if row.ID == id {
			row.Attempts++
		}
	}
	return nil
}

func (q *fakeQueue) LastSentAt(_ context.Context, userID uuid.UUID,
	typ entity.NotificationType, contextKey string) (*time.Time, error) {
	if q.sentAtErr != nil {
		return nil, q.sentAtErr
	}
	var last *time.Time
	for _, row := range q.rows {
		if row.UserID != userID || row.Type != typ || row.ContextKey != contextKey || row.SentAt == nil {
			continue
		}
		if last == nil || row.SentAt.After(*last) {
			last = row.SentAt
		}
	}
	return last, nil
}

func (q *fakeQueue) SentPersonalCount(_ context.Context, userID uuid.UUID, since time.Time) (int, error) {
	var n int
	for _, row := range q.rows {
		if row.UserID == userID && row.SentAt != nil && !row.SentAt.Before(since) &&
			slices.Contains(entity.PersonalPushTypes(), row.Type) {
			n++
		}
	}
	return n, nil
}

func (q *fakeQueue) FailAllPending(_ context.Context, userID uuid.UUID) error {
	for _, row := range q.rows {
		if row.UserID == userID && row.SentAt == nil && row.FailedAt == nil {
			failed := time.Now()
			row.FailedAt = &failed
		}
	}
	return nil
}

func (q *fakeQueue) LastLikedTail(_ context.Context, userID uuid.UUID, since time.Time) (*notificationsrepo.TailMessage, error) {
	var (
		found *notificationsrepo.TailMessage
		at    time.Time
	)
	for _, row := range q.rows {
		tail, ok := q.tails[row.ID]
		if !ok || row.UserID != userID || row.Type != entity.NotifLiked ||
			row.SentAt == nil || row.SentAt.Before(since) {
			continue
		}
		if found == nil || row.SentAt.After(at) {
			carrier := tail
			found, at = &carrier, *row.SentAt
		}
	}
	return found, nil
}

func (q *fakeQueue) SetTail(_ context.Context, id uuid.UUID, messageID, count int) error {
	q.tails[id] = notificationsrepo.TailMessage{ID: id, MessageID: messageID, Count: count}
	return nil
}

func (q *fakeQueue) count(typ entity.NotificationType) int {
	var n int
	for _, row := range q.rows {
		if row.Type == typ {
			n++
		}
	}
	return n
}

func (q *fakeQueue) byID(id uuid.UUID) *entity.Notification {
	for _, row := range q.rows {
		if row.ID == id {
			return row
		}
	}
	return nil
}

type sentPush struct {
	chatID  int64
	text    string
	buttons []tgbot.PushButton
}

type editedPush struct {
	chatID    int64
	messageID int
	text      string
}

type fakeSender struct {
	sent   []sentPush
	edited []editedPush

	err error

	editErr error
	nextID  int
}

func (f *fakeSender) SendPush(_ context.Context, chatID int64, text string, buttons []tgbot.PushButton) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.sent = append(f.sent, sentPush{chatID, text, buttons})
	f.nextID++
	return f.nextID, nil
}

func (f *fakeSender) EditPushText(_ context.Context, chatID int64, messageID int, text string, _ []tgbot.PushButton) error {
	if f.editErr != nil {
		return f.editErr
	}
	if f.err != nil {
		return f.err
	}
	f.edited = append(f.edited, editedPush{chatID, messageID, text})
	return nil
}

func (f *fakeSender) texts() []string {
	out := make([]string, 0, len(f.sent))
	for _, push := range f.sent {
		out = append(out, push.text)
	}
	return out
}

type fakeRecipients struct {
	chats   map[uuid.UUID]int64
	blocked []uuid.UUID
}

func (f *fakeRecipients) ByID(_ context.Context, id uuid.UUID) (entity.User, error) {
	chatID, ok := f.chats[id]
	if !ok {
		return entity.User{}, entity.ErrNotFound
	}
	return entity.User{ID: id, TelegramID: chatID}, nil
}

func (f *fakeRecipients) MarkPushBlocked(_ context.Context, id uuid.UUID) error {
	f.blocked = append(f.blocked, id)
	return nil
}

type fakeTTL struct {
	stale       []entity.Search
	staleCalls  int
	asked       []uuid.UUID
	paused      []uuid.UUID
	pauseCalls  int
	pauseWindow time.Duration
	quiet       time.Duration
	afterMatch  time.Duration
}

func (f *fakeTTL) StaleForTTL(_ context.Context, quiet, afterMatch time.Duration) ([]entity.Search, error) {
	f.staleCalls++
	f.quiet, f.afterMatch = quiet, afterMatch
	return f.stale, nil
}

func (f *fakeTTL) MarkTTLAsked(_ context.Context, id uuid.UUID) error {
	f.asked = append(f.asked, id)
	return nil
}

func (f *fakeTTL) PauseUnanswered(_ context.Context, olderThan time.Duration) ([]uuid.UUID, error) {
	f.pauseCalls++
	f.pauseWindow = olderThan
	return f.paused, nil
}

type fakePolls struct {
	targets []matchesrepo.PollTarget
	window  time.Duration
}

func (f *fakePolls) MatchesForPostPoll(_ context.Context, olderThan time.Duration) ([]matchesrepo.PollTarget, error) {
	f.window = olderThan
	return f.targets, nil
}

type fakePointZones struct {
	points map[uuid.UUID]entity.Point
	err    error

	forPing   []entity.Point
	pingEvery time.Duration
	asked     []uuid.UUID

	expired         []uuid.UUID
	expireCalls     int
	expireQuiet     time.Duration
	expireAfterPing time.Duration
}

func (f *fakePointZones) ByID(_ context.Context, id uuid.UUID) (entity.Point, error) {
	if f.err != nil {
		return entity.Point{}, f.err
	}
	point, ok := f.points[id]
	if !ok {
		return entity.Point{}, entity.ErrNotFound
	}
	return point, nil
}

func (f *fakePointZones) PointsForPing(_ context.Context, every time.Duration) ([]entity.Point, error) {
	f.pingEvery = every
	return f.forPing, nil
}

func (f *fakePointZones) MarkPingAsked(_ context.Context, id uuid.UUID) error {
	f.asked = append(f.asked, id)
	return nil
}

func (f *fakePointZones) Expire(_ context.Context, quiet, afterPing time.Duration) ([]uuid.UUID, error) {
	f.expireCalls++
	f.expireQuiet, f.expireAfterPing = quiet, afterPing
	return f.expired, nil
}

type workerFakes struct {
	queue  *fakeQueue
	sender *fakeSender
	users  *fakeRecipients
	ttl    *fakeTTL
	polls  *fakePolls
	points *fakePointZones
	clock  *clock
}

func newWorker(t *testing.T, f workerFakes) (*Worker, workerFakes) {
	t.Helper()

	if f.queue == nil {
		f.queue = newQueue()
	}
	if f.sender == nil {
		f.sender = &fakeSender{}
	}
	if f.users == nil {
		f.users = &fakeRecipients{chats: map[uuid.UUID]int64{}}
	}
	if f.ttl == nil {
		f.ttl = &fakeTTL{}
	}
	if f.polls == nil {
		f.polls = &fakePolls{}
	}
	if f.points == nil {
		f.points = &fakePointZones{points: map[uuid.UUID]entity.Point{}}
	}
	if f.clock == nil {
		f.clock = &clock{at: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)}
	}

	w := NewWorker(f.queue, f.sender, f.users, f.ttl, f.polls, f.points, testDir(t),
		30*time.Second, slog.New(slog.DiscardHandler))
	w.now = f.clock.now
	return w, f
}

func (f workerFakes) addressee(chatID int64) uuid.UUID {
	id := uuid.New()
	f.users.chats[id] = chatID
	return id
}

func TestAggregateGoesOutAsOneMessage(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	for range 3 {
		f.queue.put(entity.Notification{
			UserID: viewer, Type: entity.NotifZoneNew,
			ActorKey: uuid.NewString(), ContextKey: "d-khamovniki",
		})
	}

	w.pass(t.Context())

	if len(f.sender.sent) != 1 {
		t.Fatalf("sent %v, want one message", f.sender.texts())
	}
	if got, want := f.sender.sent[0].text, "В Хамовниках ищут квартиру ещё 3 человека"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := f.sender.sent[0].buttons; len(got) != 1 ||
		got[0].Text != "Открыть пул" || got[0].StartParam != "pool_d-khamovniki" {
		t.Errorf("buttons = %+v, want one deep link into the zone pool", got)
	}

	pending, _ := f.queue.Pending(t.Context(), 10)
	if len(pending) != 0 {
		t.Errorf("%d rows are still pending after the aggregate went out", len(pending))
	}
}

func TestAggregateWindowHoldsTheNextGroup(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)

	sent := f.clock.at.Add(-2 * time.Hour)
	row := f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifZoneNew,
		ActorKey: uuid.NewString(), ContextKey: "d-khamovniki",
	})
	row.SentAt = &sent
	f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifZoneNew,
		ActorKey: uuid.NewString(), ContextKey: "d-khamovniki",
	})

	w.pass(t.Context())
	if len(f.sender.sent) != 0 {
		t.Fatalf("sent %v inside the 24h window", f.sender.texts())
	}
	if pending, _ := f.queue.Pending(t.Context(), 10); len(pending) != 1 {
		t.Errorf("%d rows pending, want the held one to wait", len(pending))
	}

	f.clock.add(23 * time.Hour)
	w.pass(t.Context())
	if got := f.sender.texts(); len(got) != 1 || got[0] != "В Хамовниках ищет квартиру ещё один человек" {
		t.Errorf("sent %v after the window, want one message about one person", got)
	}
}

func TestPoolAggregateNamesTheStation(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	point := entity.Point{ID: uuid.New(), ZoneID: "st-sokol"}
	f.points.points[point.ID] = point

	for range 2 {
		f.queue.put(entity.Notification{
			UserID: viewer, Type: entity.NotifPoolNew,
			ActorKey: uuid.NewString(), ContextKey: point.ID.String(),
		})
	}

	w.pass(t.Context())

	if got, want := f.sender.texts(), []string{"К квартире у м. Сокол присмотрелись ещё 2 человека"}; !slices.Equal(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
	if got := f.sender.sent[0].buttons[0].StartParam; got != "point_"+point.ID.String() {
		t.Errorf("start param = %q, want the point deep link", got)
	}
}

func TestPersonalPushesGoOneByOne(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifLiked, ActorKey: uuid.NewString(),
		Payload: map[string]any{payloadName: "Катя", payloadGender: string(entity.GenderFemale)},
	})
	f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifLiked, ActorKey: uuid.NewString(),
		Payload: map[string]any{payloadName: "Марк"},
	})

	w.pass(t.Context())

	want := []string{"Тебя лайкнула Катя", "Тебя лайкнул Марк"}
	if got := f.sender.texts(); !slices.Equal(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
	for _, push := range f.sender.sent {
		if len(push.buttons) != 1 || push.buttons[0].StartParam != linkIncoming {
			t.Errorf("buttons = %+v, want «Посмотреть» into the incoming likes", push.buttons)
		}
	}
}

func TestPersonalPushesCarryTheirDeepLinks(t *testing.T) {
	matchID := uuid.New()
	cases := []struct {
		typ   entity.NotificationType
		actor string
		text  string
		link  string
	}{
		{entity.NotifMatched, matchID.String(), "У тебя мэтч!", "match_" + matchID.String()},
		{entity.NotifProfileOK, "a:1", "Анкета прошла проверку — теперь тебя видно", "feed"},
		{entity.NotifPhotoRejected, "a:2", "Фото не подошло — загрузи другое, где видно лицо", "photo"},
		{entity.NotifAboutRejected, "a:3", "Текст „о себе\" не подошёл — поправь его", "about"},
		{entity.NotifPointOK, "a:4", "Твоё объявление одобрено", "mine"},
		{entity.NotifPointRejected, "a:5", "Твоё объявление отклонено", "mine"},
	}

	for _, tc := range cases {
		t.Run(string(tc.typ), func(t *testing.T) {
			w, f := newWorker(t, workerFakes{})
			viewer := f.addressee(1001)
			f.queue.put(entity.Notification{UserID: viewer, Type: tc.typ, ActorKey: tc.actor})

			w.pass(t.Context())

			if len(f.sender.sent) != 1 {
				t.Fatalf("sent %v, want one message", f.sender.texts())
			}
			push := f.sender.sent[0]
			if push.text != tc.text {
				t.Errorf("text = %q, want %q", push.text, tc.text)
			}
			if len(push.buttons) != 1 || push.buttons[0].StartParam != tc.link {
				t.Errorf("buttons = %+v, want a deep link to %q", push.buttons, tc.link)
			}
		})
	}
}

func TestPointGoneSpeaksDifferentlyToTheCreator(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	creator, member := f.addressee(1001), f.addressee(1002)
	point := entity.Point{ID: uuid.New(), Type: entity.PointFlat, ZoneID: "st-sokol"}
	f.points.points[point.ID] = point

	for _, own := range []struct {
		userID uuid.UUID
		mine   bool
	}{{creator, true}, {member, false}} {
		f.queue.put(entity.Notification{
			UserID: own.userID, Type: entity.NotifPointGone,
			ActorKey: point.ID.String() + ":2", ContextKey: point.ID.String(),
			Payload: map[string]any{payloadOwn: own.mine},
		})
	}

	w.pass(t.Context())

	want := []string{
		"Твою квартиру у метро Сокол отметили как сданную — проверь",
		"Квартира у метро Сокол, похоже, сдана",
	}
	if got := f.sender.texts(); !slices.Equal(got, want) {
		t.Fatalf("sent %v, want %v", got, want)
	}
	if got := f.sender.sent[0].buttons; len(got) != 1 ||
		got[0].Text != "Проверить" || got[0].StartParam != "point_"+point.ID.String() {
		t.Errorf("buttons of the creator = %+v, want «Проверить» into the point", got)
	}
	if got := f.sender.sent[1].buttons; len(got) != 1 ||
		got[0].Text != "Открыть" || got[0].StartParam != "point_"+point.ID.String() {
		t.Errorf("buttons of the pool member = %+v, want «Открыть» into the point", got)
	}
}

func TestPointPingAsksWithTwoAnswers(t *testing.T) {
	cases := []struct {
		kind  entity.PointType
		text  string
		alive string
		gone  string
		card  func(point entity.Point) string
	}{
		{entity.PointFlat, "Квартира у метро Сокол ещё актуальна?", "Да, живая", "Уже сдана",
			func(point entity.Point) string { return "point_" + point.ID.String() }},
		{entity.PointIntentHome, "Комната у метро Сокол ещё свободна?", "Да", "Нашёл соседа",
			func(point entity.Point) string { return "point_" + point.ID.String() }},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			w, f := newWorker(t, workerFakes{})
			owner := f.addressee(1001)
			point := entity.Point{ID: uuid.New(), Type: tc.kind, ZoneID: "st-sokol"}
			f.points.points[point.ID] = point
			f.queue.put(entity.Notification{
				UserID: owner, Type: entity.NotifPointPing,
				ActorKey: point.ID.String() + ":1", ContextKey: point.ID.String(),
			})

			w.pass(t.Context())

			if len(f.sender.sent) != 1 {
				t.Fatalf("sent %v, want one message", f.sender.texts())
			}
			push := f.sender.sent[0]
			if push.text != tc.text {
				t.Errorf("text = %q, want %q", push.text, tc.text)
			}
			want := []tgbot.PushButton{
				{Text: tc.alive, CallbackData: "pnt:alive:" + point.ID.String()},
				{Text: tc.gone, CallbackData: "pnt:gone:" + point.ID.String()},
				{Text: "Открыть", StartParam: tc.card(point)},
			}
			if !slices.Equal(push.buttons, want) {
				t.Errorf("buttons = %+v, want %+v", push.buttons, want)
			}
		})
	}
}

func TestPointPushWithoutItsPointIsBuried(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	row := f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifPointGone,
		ActorKey: "gone:1", ContextKey: uuid.NewString(),
	})

	w.pass(t.Context())

	if f.queue.byID(row.ID).FailedAt == nil {
		t.Error("a push about a point that no longer exists is still queued")
	}
	if len(f.sender.sent) != 0 {
		t.Errorf("sent %v about a point that is gone", f.sender.texts())
	}
}

func TestPointPushWaitsForAnUnreadablePoint(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	f.points.err = errors.New("no database today")
	row := f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifPointPing,
		ActorKey: "ping:1", ContextKey: uuid.NewString(),
	})

	w.pass(t.Context())

	if f.queue.byID(row.ID).FailedAt != nil {
		t.Error("a push was buried because the database blinked")
	}
	if len(f.sender.sent) != 0 {
		t.Errorf("sent %v without reading the point", f.sender.texts())
	}
}

func TestLikedFoldsIntoTheTailOverTheDailyLimit(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)

	carrier := seedSentLikes(f, viewer, entity.MaxPersonalPushesPerDay)
	fresh := f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifLiked, ActorKey: uuid.NewString(),
		Payload: map[string]any{payloadName: "Катя"},
	})

	w.pass(t.Context())

	if len(f.sender.sent) != 0 {
		t.Fatalf("sent %v, want the like folded into the tail", f.sender.texts())
	}
	want := editedPush{chatID: 1001, messageID: 777, text: "И ещё 2 лайка за сегодня"}
	if len(f.sender.edited) != 1 || f.sender.edited[0] != want {
		t.Fatalf("edited %+v, want %+v", f.sender.edited, want)
	}
	if f.queue.byID(fresh.ID).SentAt == nil {
		t.Error("the folded row was left pending and will be folded again")
	}

	if got := f.queue.tails[carrier.ID]; got.Count != 2 || got.MessageID != 777 {
		t.Errorf("tail = %+v, want the carrier grown to 2", got)
	}
}

func TestMatchedIsNeverFolded(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	seedSentLikes(f, viewer, entity.MaxPersonalPushesPerDay)
	f.queue.put(entity.Notification{UserID: viewer, Type: entity.NotifMatched, ActorKey: uuid.NewString()})

	w.pass(t.Context())

	if got := f.sender.texts(); !slices.Equal(got, []string{"У тебя мэтч!"}) {
		t.Errorf("sent %v, want the match to go out as it is", got)
	}
	if len(f.sender.edited) != 0 {
		t.Errorf("the match was folded into the tail: %+v", f.sender.edited)
	}
}

func TestLikedWithoutACarrierIsSentAndBecomesOne(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)

	sent := f.clock.at.Add(-time.Hour)
	for range entity.MaxPersonalPushesPerDay {
		row := f.queue.put(entity.Notification{
			UserID: viewer, Type: entity.NotifLiked, ActorKey: uuid.NewString(),
		})
		row.SentAt = &sent
	}
	fresh := f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifLiked, ActorKey: uuid.NewString(),
		Payload: map[string]any{payloadName: "Катя", payloadGender: string(entity.GenderFemale)},
	})

	w.pass(t.Context())

	if got := f.sender.texts(); !slices.Equal(got, []string{"Тебя лайкнула Катя"}) {
		t.Errorf("sent %v, want the like as an ordinary push", got)
	}
	if got := f.queue.tails[fresh.ID]; got.Count != 1 {
		t.Errorf("tail of the sent like = %+v, want it remembered as a carrier", got)
	}
}

func TestBlockedRecipientLosesTheWholeQueue(t *testing.T) {
	w, f := newWorker(t, workerFakes{sender: &fakeSender{
		err: fmt.Errorf("%w: telegram sendMessage: 403 Forbidden: bot was blocked by the user", tgbot.ErrBlocked),
	}})
	viewer := f.addressee(1001)
	f.queue.put(entity.Notification{UserID: viewer, Type: entity.NotifMatched, ActorKey: uuid.NewString()})
	f.queue.put(entity.Notification{UserID: viewer, Type: entity.NotifLiked, ActorKey: uuid.NewString()})

	w.pass(t.Context())

	if !slices.Equal(f.users.blocked, []uuid.UUID{viewer}) {
		t.Errorf("blocked %v, want [%s]", f.users.blocked, viewer)
	}
	if pending, _ := f.queue.Pending(t.Context(), 10); len(pending) != 0 {
		t.Errorf("%d rows still queued for a blocked user", len(pending))
	}
}

func TestFifthFailureBuriesThePush(t *testing.T) {
	w, f := newWorker(t, workerFakes{sender: &fakeSender{err: errors.New("connection refused")}})
	viewer := f.addressee(1001)
	fresh := f.queue.put(entity.Notification{UserID: viewer, Type: entity.NotifMatched, ActorKey: uuid.NewString()})
	tired := f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifLiked, ActorKey: uuid.NewString(), Attempts: maxAttempts - 1,
	})

	w.pass(t.Context())

	if got := f.queue.byID(fresh.ID); got.Attempts != 1 || got.FailedAt != nil {
		t.Errorf("fresh row = %+v, want one attempt counted and no burial", got)
	}
	if got := f.queue.byID(tired.ID); got.FailedAt == nil {
		t.Errorf("row with %d attempts is still queued", got.Attempts)
	}
}

func TestTTLScanAsksAndMarks(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	stale := search(viewer, "d-khamovniki")
	f.ttl.stale = []entity.Search{stale}

	w.pass(t.Context())

	if f.ttl.quiet != entity.TTLQuietDays*24*time.Hour || f.ttl.afterMatch != entity.TTLAfterMatchDays*24*time.Hour {
		t.Errorf("asked for stale searches with (%s, %s), want 7d and 3d", f.ttl.quiet, f.ttl.afterMatch)
	}
	if !slices.Equal(f.ttl.asked, []uuid.UUID{stale.ID}) {
		t.Errorf("marked %v asked, want [%s]", f.ttl.asked, stale.ID)
	}
	pending, _ := f.queue.Pending(t.Context(), 10)
	if len(pending) != 1 {
		t.Fatalf("queued %d rows, want the ttl question", len(pending))
	}

	if got := pending[0]; got.Type != entity.NotifStillSearching ||
		got.ActorKey != ttlActor(stale) || got.ContextKey != "d-khamovniki" ||
		got.Payload[payloadSearchID] != stale.ID.String() {
		t.Errorf("row = %+v, want a still_searching one about the search itself", got)
	}

	w.pass(t.Context())
	if len(f.sender.sent) != 1 {
		t.Fatalf("sent %v, want the ttl question", f.sender.texts())
	}
	push := f.sender.sent[0]
	if push.text != "Всё ещё ищешь в Хамовниках?" {
		t.Errorf("text = %q", push.text)
	}
	want := []tgbot.PushButton{
		{Text: "Да, ищу", CallbackData: "srch:yes:" + stale.ID.String()},
		{Text: "Нашёл", CallbackData: "srch:found:" + stale.ID.String()},
	}
	if !slices.Equal(push.buttons, want) {
		t.Errorf("buttons = %+v, want %+v", push.buttons, want)
	}
}

func TestSilentSearchesArePaused(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	f.ttl.paused = []uuid.UUID{uuid.New()}

	w.pass(t.Context())

	if f.ttl.pauseCalls != 1 {
		t.Errorf("PauseUnanswered called %d times, want 1", f.ttl.pauseCalls)
	}
	if got, want := f.ttl.pauseWindow, entity.TTLAnswerDays*24*time.Hour; got != want {
		t.Errorf("pause window = %s, want %s", got, want)
	}
}

func TestPostMatchQuestionCarriesThePeer(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	match := entity.Match{ID: uuid.New(), AID: viewer, BID: uuid.New()}
	f.polls.targets = []matchesrepo.PollTarget{{Match: match, UserID: viewer, PeerName: "Катя"}}

	w.pass(t.Context())
	if got, want := f.polls.window, entity.PostMatchDays*24*time.Hour; got != want {
		t.Errorf("asked for matches older than %s, want %s", got, want)
	}
	w.pass(t.Context())

	if len(f.sender.sent) != 1 {
		t.Fatalf("sent %v, want the post-match question", f.sender.texts())
	}
	push := f.sender.sent[0]
	if push.text != "Как дела с Катей? Расскажи, что вышло" {
		t.Errorf("text = %q", push.text)
	}
	want := []tgbot.PushButton{
		{Text: "Съезжаемся", CallbackData: "poll:" + match.ID.String() + ":moving"},
		{Text: "Общаемся", CallbackData: "poll:" + match.ID.String() + ":talking"},
		{Text: "Не сложилось", CallbackData: "poll:" + match.ID.String() + ":failed"},
	}
	if !slices.Equal(push.buttons, want) {
		t.Errorf("buttons = %+v, want %+v", push.buttons, want)
	}

	f.clock.add(scanEvery)
	w.pass(t.Context())
	if len(f.sender.sent) != 1 {
		t.Errorf("sent %v, want the question asked once", f.sender.texts())
	}
}

func TestPointPingScanAsksTheCreator(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	creator := f.addressee(1001)
	seenAlive := f.clock.at.Add(-8 * 24 * time.Hour)
	point := entity.Point{ID: uuid.New(), Type: entity.PointFlat, ZoneID: "st-sokol", CreatedBy: &creator, LastSeenAlive: seenAlive}
	f.points.forPing = []entity.Point{point}

	w.pass(t.Context())

	if got, want := f.points.pingEvery, entity.PingEveryDays*24*time.Hour; got != want {
		t.Errorf("asked for points quiet for %s, want %s", got, want)
	}
	if !slices.Equal(f.points.asked, []uuid.UUID{point.ID}) {
		t.Errorf("marked pinged %v, want [%s]", f.points.asked, point.ID)
	}
	if got := f.queue.count(entity.NotifPointPing); got != 1 {
		t.Fatalf("%d point pings queued, want 1", got)
	}
	row := f.queue.rows[len(f.queue.rows)-1]
	if row.UserID != creator {
		t.Errorf("addressed to %s, want the creator %s", row.UserID, creator)
	}
	if want := fmt.Sprintf("%s:%d", point.ID, seenAlive.Unix()); row.ActorKey != want {
		t.Errorf("actor = %q, want %q", row.ActorKey, want)
	}
	if row.ContextKey != point.ID.String() {
		t.Errorf("context = %q, want the point %s", row.ContextKey, point.ID)
	}
}

func TestPointWithoutACreatorIsNotPinged(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	point := entity.Point{ID: uuid.New(), Type: entity.PointFlat, ZoneID: "st-sokol"}
	f.points.forPing = []entity.Point{point}

	w.pass(t.Context())

	if len(f.points.asked) != 0 {
		t.Errorf("marked pinged %v, want none — the point has no creator", f.points.asked)
	}
	if got := f.queue.count(entity.NotifPointPing); got != 0 {
		t.Errorf("%d point pings queued for a point without a creator, want 0", got)
	}
}

func TestPointsExpireOncePerScan(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	f.points.expired = []uuid.UUID{uuid.New()}

	w.pass(t.Context())
	f.clock.add(30 * time.Second)
	w.pass(t.Context())

	if f.points.expireCalls != 1 {
		t.Errorf("Expire called %d times within the hour, want 1", f.points.expireCalls)
	}
	if got, want := f.points.expireQuiet, entity.PointTTLDays*24*time.Hour; got != want {
		t.Errorf("expire quiet window = %s, want %s", got, want)
	}
	if got, want := f.points.expireAfterPing, entity.PingAnswerDays*24*time.Hour; got != want {
		t.Errorf("expire after-ping window = %s, want %s", got, want)
	}

	f.clock.add(scanEvery)
	w.pass(t.Context())
	if f.points.expireCalls != 2 {
		t.Errorf("Expire called %d times after an hour, want 2", f.points.expireCalls)
	}
}

func TestNoAnswersMeansNoPingOrArchive(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	w = w.WithoutAnswers()
	creator := f.addressee(1001)
	f.points.forPing = []entity.Point{{ID: uuid.New(), Type: entity.PointFlat, ZoneID: "st-sokol", CreatedBy: &creator}}

	w.pass(t.Context())

	if len(f.points.asked) != 0 {
		t.Errorf("asked %v while the bot cannot hear answers", f.points.asked)
	}
	if got := f.queue.count(entity.NotifPointPing); got != 0 {
		t.Errorf("%d point pings queued with nobody listening", got)
	}
	if f.points.expireCalls != 0 {
		t.Errorf("Expire called %d times with nobody listening, want 0", f.points.expireCalls)
	}
}

func TestScansRunHourlyWhileTheQueueDrainsEveryTick(t *testing.T) {
	w, f := newWorker(t, workerFakes{})

	w.pass(t.Context())
	f.clock.add(30 * time.Second)
	w.pass(t.Context())
	if f.ttl.staleCalls != 1 {
		t.Errorf("scanned %d times within the hour, want 1", f.ttl.staleCalls)
	}

	f.clock.add(scanEvery)
	w.pass(t.Context())
	if f.ttl.staleCalls != 2 {
		t.Errorf("scanned %d times after an hour, want 2", f.ttl.staleCalls)
	}
}

func TestPushWithoutAnAddresseeWaits(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	f.queue.put(entity.Notification{UserID: uuid.New(), Type: entity.NotifMatched, ActorKey: uuid.NewString()})

	w.pass(t.Context())

	if len(f.sender.sent) != 0 {
		t.Errorf("sent %v to a user who does not exist", f.sender.texts())
	}
	if pending, _ := f.queue.Pending(t.Context(), 10); len(pending) != 1 {
		t.Errorf("%d rows pending, want the row kept", len(pending))
	}
}

func TestPushWithAGoneContextIsBuried(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	row := f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifZoneNew, ActorKey: uuid.NewString(), ContextKey: "d-atlantis",
	})

	w.pass(t.Context())

	if f.queue.byID(row.ID).FailedAt == nil {
		t.Error("a push about a zone that no longer exists is still queued")
	}
}

func seedSentLikes(f workerFakes, userID uuid.UUID, limit int) *entity.Notification {
	var carrier *entity.Notification
	for i := range limit {
		sent := f.clock.at.Add(-time.Duration(limit-i) * time.Minute)
		row := f.queue.put(entity.Notification{
			UserID: userID, Type: entity.NotifLiked, ActorKey: uuid.NewString(),
		})
		row.SentAt = &sent
		carrier = row
	}
	f.queue.tails[carrier.ID] = notificationsrepo.TailMessage{ID: carrier.ID, MessageID: 777, Count: 1}
	return carrier
}

func TestBrokenQueueStillLetsTheScansRun(t *testing.T) {
	w, f := newWorker(t, workerFakes{queue: newQueue()})
	f.queue.pendingErr = errors.New("no database today")

	w.pass(t.Context())

	if f.ttl.staleCalls != 1 || f.ttl.pauseCalls != 1 {
		t.Errorf("scans ran %d/%d times, want them to run anyway", f.ttl.staleCalls, f.ttl.pauseCalls)
	}
	if len(f.sender.sent) != 0 {
		t.Errorf("sent %v with an unreadable queue", f.sender.texts())
	}
}

func TestUncheckableWindowHoldsTheAggregate(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	f.queue.sentAtErr = errors.New("no database today")
	f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifZoneNew, ActorKey: uuid.NewString(), ContextKey: "d-khamovniki",
	})

	w.pass(t.Context())

	if len(f.sender.sent) != 0 {
		t.Errorf("sent %v without knowing the window", f.sender.texts())
	}
	if pending, _ := f.queue.Pending(t.Context(), 10); len(pending) != 1 {
		t.Errorf("%d rows pending, want the row kept for the next tick", len(pending))
	}
}

func TestTTLQuestionRepeatsOnTheNextSilence(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	viewer := f.addressee(1001)
	stale := search(viewer, "d-khamovniki")
	stale.LastActiveAt = f.clock.at.Add(-8 * 24 * time.Hour)
	f.ttl.stale = []entity.Search{stale}

	w.pass(t.Context())
	f.clock.add(scanEvery)
	w.pass(t.Context())
	if got := f.queue.count(entity.NotifStillSearching); got != 1 {
		t.Errorf("%d questions about the same silence, want 1", got)
	}

	stale.LastActiveAt = f.clock.at
	f.clock.add(8 * 24 * time.Hour)
	f.ttl.stale = []entity.Search{stale}
	w.pass(t.Context())

	if got := f.queue.count(entity.NotifStillSearching); got != 2 {
		t.Errorf("%d questions after a second silence, want 2", got)
	}
}

func TestRateLimitDoesNotCountAnAttempt(t *testing.T) {
	w, f := newWorker(t, workerFakes{sender: &fakeSender{
		err: &tgbot.APIError{Method: "sendMessage", Code: 429, Description: "Too Many Requests: retry after 3"},
	}})
	viewer := f.addressee(1001)
	row := f.queue.put(entity.Notification{UserID: viewer, Type: entity.NotifMatched, ActorKey: uuid.NewString()})

	w.pass(t.Context())

	if got := f.queue.byID(row.ID); got.Attempts != 0 || got.FailedAt != nil || got.SentAt != nil {
		t.Errorf("row after 429 = %+v, want untouched and still queued", got)
	}
}

func TestGoneTailCarrierFallsBackToAFreshPush(t *testing.T) {
	sender := &fakeSender{editErr: &tgbot.APIError{Method: "editMessageText", Code: 400,
		Description: "Bad Request: message to edit not found"}}
	w, f := newWorker(t, workerFakes{sender: sender})
	viewer := f.addressee(1001)
	seedSentLikes(f, viewer, entity.MaxPersonalPushesPerDay)
	late := f.queue.put(entity.Notification{
		UserID: viewer, Type: entity.NotifLiked, ActorKey: uuid.NewString(),
		Payload: map[string]any{payloadName: "Катя", payloadGender: "female"},
	})

	w.pass(t.Context())

	if got := f.queue.byID(late.ID); got.SentAt == nil || got.FailedAt != nil {
		t.Fatalf("late like after the carrier vanished = %+v, want sent", got)
	}
	if len(sender.sent) != 1 || sender.sent[0].text != LikedText("Катя", entity.GenderFemale) {
		t.Errorf("sent %v, want one fresh «Тебя лайкнула Катя»", sender.texts())
	}
}

func TestNoAnswersMeansNoQuestions(t *testing.T) {
	w, f := newWorker(t, workerFakes{})
	w = w.WithoutAnswers()
	viewer := f.addressee(1001)
	f.ttl.stale = []entity.Search{search(viewer, "d-khamovniki")}

	w.pass(t.Context())

	if len(f.ttl.asked) != 0 {
		t.Errorf("asked %v while the bot cannot hear answers", f.ttl.asked)
	}
	if pending, _ := f.queue.Pending(t.Context(), 10); len(pending) != 0 {
		t.Errorf("queued %d questions with nobody listening", len(pending))
	}
}
