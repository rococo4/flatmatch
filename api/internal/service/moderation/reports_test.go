package moderation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

type resolveCall struct {
	id      uuid.UUID
	verdict entity.ReportVerdict
}

type fakeReports struct {
	report     entity.Report
	byIDErr    error
	unresolved []entity.Report
	resolves   []resolveCall
}

type fakeQueue struct {
	failed []uuid.UUID
	err    error
}

func (f *fakeReports) ByID(context.Context, uuid.UUID) (entity.Report, error) {
	if f.byIDErr != nil {
		return entity.Report{}, f.byIDErr
	}
	return f.report, nil
}

func (f *fakeReports) Resolve(_ context.Context, id uuid.UUID, verdict entity.ReportVerdict) (bool, error) {
	f.resolves = append(f.resolves, resolveCall{id: id, verdict: verdict})
	return true, nil
}

func (f *fakeReports) Unresolved(context.Context) ([]entity.Report, error) {
	return f.unresolved, nil
}

func (q *fakeQueue) FailAllPending(_ context.Context, userID uuid.UUID) error {
	q.failed = append(q.failed, userID)
	return q.err
}

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

func TestReportBanSilencesTheTarget(t *testing.T) {
	target := reported()
	target.Status = entity.StatusActive
	report := entity.Report{ID: uuid.New(), TargetID: target.ID, Reason: entity.ReasonScam}

	users := &fakeUsers{user: target}
	reports := &fakeReports{report: report}
	queue := &fakeQueue{}
	points := &fakePoints{}
	svc := newReportService(t, testChat, &fakeSender{}, users, points, reports, queue)

	ack, err := svc.HandleCallback(t.Context(), callback(subjectReport, opBan, report.ID))
	if err != nil {
		t.Fatalf("HandleCallback() failed: %v", err)
	}
	if ack != ackBanned {
		t.Errorf("ack = %q, want %q", ack, ackBanned)
	}
	if len(points.closedBy) != 1 || points.closedBy[0] != target.ID {
		t.Errorf("closed points of = %v, want the points of %s", points.closedBy, target.ID)
	}
	if len(users.statuses) != 1 || users.statuses[0].id != target.ID ||
		users.statuses[0].status != entity.StatusBanned {
		t.Errorf("status writes = %+v, want one ban of %s", users.statuses, target.ID)
	}
	if len(queue.failed) != 1 || queue.failed[0] != target.ID {
		t.Errorf("failed queues = %v, want the queue of %s", queue.failed, target.ID)
	}
	if len(reports.resolves) != 1 || reports.resolves[0].id != report.ID ||
		reports.resolves[0].verdict != entity.ReportBanned {
		t.Errorf("resolves = %+v, want one banned verdict on %s", reports.resolves, report.ID)
	}
}

func TestReportSkipResolvesWithoutABan(t *testing.T) {
	target := reported()
	target.Status = entity.StatusActive
	report := entity.Report{ID: uuid.New(), TargetID: target.ID, Reason: entity.ReasonOther}

	users := &fakeUsers{user: target}
	reports := &fakeReports{report: report}
	queue := &fakeQueue{}
	svc := newReportService(t, testChat, &fakeSender{}, users, &fakePoints{}, reports, queue)

	ack, err := svc.HandleCallback(t.Context(), callback(subjectReport, opSkip, report.ID))
	if err != nil {
		t.Fatalf("HandleCallback() failed: %v", err)
	}
	if ack != ackSkipped {
		t.Errorf("ack = %q, want %q", ack, ackSkipped)
	}
	if len(users.statuses) != 0 || len(queue.failed) != 0 {
		t.Errorf("a skipped complaint touched the person: %+v / %v", users.statuses, queue.failed)
	}
	if len(reports.resolves) != 1 || reports.resolves[0].verdict != entity.ReportSkipped {
		t.Errorf("resolves = %+v, want one skipped verdict", reports.resolves)
	}
}

func TestReportVerdictsAreIdempotent(t *testing.T) {
	tests := []struct {
		name     string
		op       string
		banned   bool
		resolved bool
		ack      string
		verdict  *entity.ReportVerdict
	}{
		{
			name: "target was banned before the tap",
			op:   opBan, banned: true, ack: ackBannedAlready,
			verdict: pointer.To(entity.ReportBanned),
		},
		{
			name: "second tap on a handled complaint",
			op:   opBan, resolved: true, ack: ackDone,
		},
		{
			name: "skip after the ban was given",
			op:   opSkip, banned: true, resolved: true, ack: ackDone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := reported()
			target.Status = entity.StatusActive
			if tt.banned {
				target.Status = entity.StatusBanned
			}
			report := entity.Report{ID: uuid.New(), TargetID: target.ID, Reason: entity.ReasonAbuse}
			if tt.resolved {
				report.ResolvedAt = pointer.To(time.Now())
				report.Verdict = pointer.To(entity.ReportBanned)
			}

			users := &fakeUsers{user: target}
			reports := &fakeReports{report: report}
			queue := &fakeQueue{}
			svc := newReportService(t, testChat, &fakeSender{}, users, &fakePoints{}, reports, queue)

			ack, err := svc.HandleCallback(t.Context(), callback(subjectReport, tt.op, report.ID))
			if err != nil {
				t.Fatalf("HandleCallback() failed: %v", err)
			}
			if ack != tt.ack {
				t.Errorf("ack = %q, want %q", ack, tt.ack)
			}
			if len(users.statuses) != 0 || len(queue.failed) != 0 {
				t.Errorf("a repeated verdict wrote %+v / %v", users.statuses, queue.failed)
			}

			if tt.verdict == nil {
				if len(reports.resolves) != 0 {
					t.Fatalf("a handled complaint was resolved again: %+v", reports.resolves)
				}
				return
			}
			if len(reports.resolves) != 1 || reports.resolves[0].verdict != *tt.verdict {
				t.Errorf("resolves = %+v, want one %s verdict", reports.resolves, *tt.verdict)
			}
		})
	}
}

func TestReportVerdictOnAMissingComplaintFails(t *testing.T) {
	reports := &fakeReports{byIDErr: entity.ErrNotFound}
	users := &fakeUsers{user: reported()}
	svc := newReportService(t, testChat, &fakeSender{}, users, &fakePoints{}, reports, &fakeQueue{})

	if _, err := svc.HandleCallback(t.Context(), callback(subjectReport, opBan, uuid.New())); err == nil {
		t.Error("HandleCallback() = nil error on a complaint that is not there")
	}
	if len(users.statuses) != 0 || len(reports.resolves) != 0 {
		t.Errorf("a missing complaint wrote %+v / %+v", users.statuses, reports.resolves)
	}
}

func TestSendQueueResendsUnresolvedReports(t *testing.T) {
	reporter, target := complainant(), reported()
	report := entity.Report{ID: uuid.New(), ReporterID: reporter.ID,
		TargetID: target.ID, Reason: entity.ReasonAbuse}

	bot := &fakeSender{}
	users := &fakeUsers{byID: map[uuid.UUID]entity.User{reporter.ID: reporter, target.ID: target}}
	svc := newReportService(t, testChat, bot, users, &fakePoints{},
		&fakeReports{unresolved: []entity.Report{report}}, &fakeQueue{})

	if err := svc.SendQueue(t.Context()); err != nil {
		t.Fatalf("SendQueue() failed: %v", err)
	}
	if len(bot.sent) != 1 {
		t.Fatalf("sent %d cards, want the unresolved complaint", len(bot.sent))
	}
	card := bot.sent[0]
	if !strings.Contains(card.caption, "Жалоба: Арсений → Катя (24)") {
		t.Errorf("caption = %q, want the complaint of Арсений", card.caption)
	}
	if want := callback(subjectReport, opBan, report.ID); card.buttons[0][1] != want {
		t.Errorf("first button = %q, want %q", card.buttons[0][1], want)
	}
}
