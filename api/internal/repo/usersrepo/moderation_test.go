package usersrepo

import (
	"slices"
	"testing"

	"github.com/AlekSi/pointer"

	"flatmatch/api/internal/entity"
)

func seedSubmitted(t *testing.T, repo *Repo) entity.User {
	t.Helper()
	user := seedUser(t, repo)
	user, err := repo.SetPhotos(t.Context(), user.ID, []string{"photos/test.jpg"})
	if err != nil {
		t.Fatalf("seed photo: %v", err)
	}
	return user
}

func TestSetModerationTouchesOnlyWhatTheVerdictNames(t *testing.T) {
	repo := New(newTestPool(t))
	user := seedSubmitted(t, repo)

	approved, err := repo.SetModeration(t.Context(), user.ID,
		pointer.To(entity.StatusActive), pointer.To(entity.ModerationOK), pointer.To(entity.ModerationOK))
	if err != nil {
		t.Fatalf("SetModeration() failed: %v", err)
	}
	if approved.Status != entity.StatusActive ||
		approved.PhotoStatus != entity.ModerationOK || approved.AboutStatus != entity.ModerationOK {
		t.Fatalf("after approval: %s / %s / %s, want active / ok / ok",
			approved.Status, approved.PhotoStatus, approved.AboutStatus)
	}

	rejected, err := repo.SetModeration(t.Context(), user.ID, nil, pointer.To(entity.ModerationRejected), nil)
	if err != nil {
		t.Fatalf("SetModeration() failed: %v", err)
	}
	if rejected.Status != entity.StatusActive ||
		rejected.PhotoStatus != entity.ModerationRejected || rejected.AboutStatus != entity.ModerationOK {
		t.Errorf("after a photo reject: %s / %s / %s, want active / rejected / ok",
			rejected.Status, rejected.PhotoStatus, rejected.AboutStatus)
	}
}

func TestPendingModerationHoldsBothQueues(t *testing.T) {
	repo := New(newTestPool(t))
	user := seedSubmitted(t, repo)

	queued := func(t *testing.T) bool {
		t.Helper()
		list, err := repo.PendingModeration(t.Context())
		if err != nil {
			t.Fatalf("PendingModeration() failed: %v", err)
		}
		return slices.ContainsFunc(list, func(u entity.User) bool { return u.ID == user.ID })
	}

	set := func(t *testing.T, status *entity.Status, photo, about *entity.ModerationStatus) {
		t.Helper()
		if _, err := repo.SetModeration(t.Context(), user.ID, status, photo, about); err != nil {
			t.Fatalf("SetModeration() failed: %v", err)
		}
	}

	if queued(t) {
		t.Error("a draft is in the moderation queue")
	}

	set(t, pointer.To(entity.StatusPending), nil, nil)
	if !queued(t) {
		t.Error("a submitted questionnaire is missing from the queue")
	}

	set(t, pointer.To(entity.StatusActive), pointer.To(entity.ModerationOK), pointer.To(entity.ModerationOK))
	if queued(t) {
		t.Error("an approved questionnaire is still in the queue")
	}

	set(t, nil, pointer.To(entity.ModerationPending), nil)
	if !queued(t) {
		t.Error("a re-uploaded photo is missing from the queue")
	}

	set(t, nil, pointer.To(entity.ModerationOK), pointer.To(entity.ModerationPending))
	if !queued(t) {
		t.Error("an edited «о себе» is missing from the queue")
	}
}
