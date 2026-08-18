package moderation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/notifications"
)

var _ Pushes = (*notifications.Service)(nil)

type profileVerdict struct {
	userID uuid.UUID
	typ    entity.NotificationType
}

type pointVerdictCall struct {
	point entity.Point
	ok    bool
}

type fakePushes struct {
	profiles []profileVerdict
	points   []pointVerdictCall
}

func (f *fakePushes) ProfileVerdict(_ context.Context, userID uuid.UUID, typ entity.NotificationType) {
	f.profiles = append(f.profiles, profileVerdict{userID: userID, typ: typ})
}

func (f *fakePushes) PointVerdict(_ context.Context, point entity.Point, ok bool) {
	f.points = append(f.points, pointVerdictCall{point: point, ok: ok})
}

func newPushService(t *testing.T, users UsersMod, points PointsMod, pushes Pushes) *Service {
	t.Helper()
	s := newService(t, testChat, &fakeSender{}, users, points)
	s.pushes = pushes
	return s
}

func TestUserVerdictPushesItsType(t *testing.T) {
	tests := []struct {
		name string
		op   string
		want entity.NotificationType
	}{
		{"approved", opOK, entity.NotifProfileOK},
		{"photo rejected", opPhoto, entity.NotifPhotoRejected},
		{"text rejected", opAbout, entity.NotifAboutRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := submitted()
			pushes := &fakePushes{}
			s := newPushService(t, &fakeUsers{user: user}, &fakePoints{}, pushes)

			if _, err := s.HandleCallback(t.Context(), callback(subjectUser, tt.op, user.ID)); err != nil {
				t.Fatalf("HandleCallback() failed: %v", err)
			}

			if len(pushes.profiles) != 1 {
				t.Fatalf("ProfileVerdict called %d times, want 1", len(pushes.profiles))
			}
			got := pushes.profiles[0]
			if got.userID != user.ID || got.typ != tt.want {
				t.Errorf("ProfileVerdict(%s, %s), want (%s, %s)", got.userID, got.typ, user.ID, tt.want)
			}
		})
	}
}

func TestStaleUserVerdictPushesNothing(t *testing.T) {
	tests := []struct {
		name string
		user func(entity.User) entity.User
		op   string
	}{
		{"second tap on an approved questionnaire", func(u entity.User) entity.User {
			u.Status = entity.StatusActive
			u.PhotoStatus, u.AboutStatus = entity.ModerationOK, entity.ModerationOK
			return u
		}, opOK},
		{"second tap on an already rejected photo", func(u entity.User) entity.User {
			u.Status, u.PhotoStatus = entity.StatusDraft, entity.ModerationRejected
			return u
		}, opPhoto},
		{"banned account", func(u entity.User) entity.User {
			u.Status = entity.StatusBanned
			return u
		}, opOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := tt.user(submitted())
			pushes := &fakePushes{}
			s := newPushService(t, &fakeUsers{user: user}, &fakePoints{}, pushes)

			ack, err := s.HandleCallback(t.Context(), callback(subjectUser, tt.op, user.ID))
			if err != nil {
				t.Fatalf("HandleCallback() failed: %v", err)
			}
			if ack != ackDone {
				t.Fatalf("ack = %q, want %q", ack, ackDone)
			}
			if len(pushes.profiles) != 0 {
				t.Errorf("ProfileVerdict called %d times on a stale button, want 0", len(pushes.profiles))
			}
		})
	}
}

func TestFailedUserVerdictPushesNothing(t *testing.T) {
	user := submitted()
	pushes := &fakePushes{}
	s := newPushService(t, &fakeUsers{user: user, setErr: errors.New("database is away")},
		&fakePoints{}, pushes)

	if _, err := s.HandleCallback(t.Context(), callback(subjectUser, opOK, user.ID)); err == nil {
		t.Fatal("HandleCallback() succeeded, want the repo error")
	}

	if len(pushes.profiles) != 0 {
		t.Errorf("ProfileVerdict called %d times after a failed verdict, want 0", len(pushes.profiles))
	}
}

func TestFailedPointVerdictPushesNothing(t *testing.T) {
	pushes := &fakePushes{}
	s := newPushService(t, &fakeUsers{},
		&fakePoints{setErr: errors.New("database is away")}, pushes)

	if _, err := s.HandleCallback(t.Context(), callback(subjectPoint, opOK, uuid.New())); err == nil {
		t.Fatal("HandleCallback() succeeded, want the repo error")
	}

	if len(pushes.points) != 0 {
		t.Errorf("PointVerdict called %d times after a failed verdict, want 0", len(pushes.points))
	}
}

func TestPointVerdictPushesToItsAuthor(t *testing.T) {
	tests := []struct {
		name string
		op   string
		ok   bool
	}{
		{"approved", opOK, true},
		{"rejected", opNo, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			author := uuid.New()
			id := uuid.New()
			points := &fakePoints{changed: true, point: entity.Point{ID: id, CreatedBy: &author}}
			pushes := &fakePushes{}
			s := newPushService(t, &fakeUsers{}, points, pushes)

			if _, err := s.HandleCallback(t.Context(), callback(subjectPoint, tt.op, id)); err != nil {
				t.Fatalf("HandleCallback() failed: %v", err)
			}

			if len(pushes.points) != 1 {
				t.Fatalf("PointVerdict called %d times, want 1", len(pushes.points))
			}
			got := pushes.points[0]
			if got.point.ID != id || got.ok != tt.ok {
				t.Errorf("PointVerdict(%s, %v), want (%s, %v)", got.point.ID, got.ok, id, tt.ok)
			}
		})
	}
}

func TestStalePointVerdictPushesNothing(t *testing.T) {
	pushes := &fakePushes{}
	s := newPushService(t, &fakeUsers{}, &fakePoints{changed: false}, pushes)

	ack, err := s.HandleCallback(t.Context(), callback(subjectPoint, opOK, uuid.New()))
	if err != nil {
		t.Fatalf("HandleCallback() failed: %v", err)
	}
	if ack != ackDone {
		t.Fatalf("ack = %q, want %q", ack, ackDone)
	}
	if len(pushes.points) != 0 {
		t.Errorf("PointVerdict called %d times on a stale button, want 0", len(pushes.points))
	}
}

func TestPointVerdictSurvivesAnUnreadablePoint(t *testing.T) {
	pushes := &fakePushes{}
	s := newPushService(t, &fakeUsers{},
		&fakePoints{changed: true, byIDErr: errors.New("gone")}, pushes)

	ack, err := s.HandleCallback(t.Context(), callback(subjectPoint, opOK, uuid.New()))
	if err != nil {
		t.Fatalf("HandleCallback() failed: %v", err)
	}
	if ack != ackApproved {
		t.Errorf("ack = %q, want %q", ack, ackApproved)
	}
	if len(pushes.points) != 0 {
		t.Errorf("PointVerdict called %d times without a point, want 0", len(pushes.points))
	}
}

func TestVerdictsWithoutPushesDoNotPanic(t *testing.T) {
	user := submitted()
	s := newService(t, testChat, &fakeSender{}, &fakeUsers{user: user}, &fakePoints{changed: true})

	if _, err := s.HandleCallback(t.Context(), callback(subjectUser, opOK, user.ID)); err != nil {
		t.Fatalf("HandleCallback() failed: %v", err)
	}
	if _, err := s.HandleCallback(t.Context(), callback(subjectPoint, opOK, uuid.New())); err != nil {
		t.Fatalf("HandleCallback() failed: %v", err)
	}
}
