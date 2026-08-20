package users

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

type fakeRepo struct {
	got  entity.TelegramIdentity
	user entity.User
	err  error

	source     string
	sourceCall bool

	patched    entity.ProfilePatch
	photoKey   string
	setStatus  entity.Status
	statusCall bool
}

func (f *fakeRepo) EnsureByTelegramID(_ context.Context, tg entity.TelegramIdentity) (entity.User, error) {
	f.got = tg
	return f.user, f.err
}

func (f *fakeRepo) ByID(context.Context, uuid.UUID) (entity.User, error) {
	return f.user, f.err
}

func (f *fakeRepo) UpdateProfile(_ context.Context, _ uuid.UUID, p entity.ProfilePatch) (entity.User, error) {
	f.patched = p
	return f.user, f.err
}

func (f *fakeRepo) SetPhoto(_ context.Context, _ uuid.UUID, key string) (entity.User, error) {
	f.photoKey = key
	return f.user, f.err
}

func (f *fakeRepo) SetStatus(_ context.Context, _ uuid.UUID, s entity.Status) (entity.User, error) {
	f.statusCall = true
	f.setStatus = s
	return f.user, f.err
}

func (f *fakeRepo) SetSourceIfEmpty(_ context.Context, _ uuid.UUID, source string) error {
	f.sourceCall = true
	f.source = source
	return f.err
}

var jpegFile = []byte("\xff\xd8\xff\xe0")

type fakeStore struct{ put string }

func (f *fakeStore) Put(_ context.Context, key string, _ io.Reader, _ string) error {
	f.put = key
	return nil
}
func (f *fakeStore) Delete(context.Context, string) error { return nil }
func (f *fakeStore) URL(key string) string                { return "/media/" + key }

type fakeNotifier struct {
	queued []entity.User
	ctxs   []context.Context
}

func (f *fakeNotifier) UserPending(ctx context.Context, u entity.User) {
	f.queued = append(f.queued, u)
	f.ctxs = append(f.ctxs, ctx)
}

func newService(repo *fakeRepo) *Service { return New(repo, &fakeStore{}, nil) }

func newModeratedService(repo *fakeRepo) (*Service, *fakeNotifier) {
	notifier := &fakeNotifier{}
	return New(repo, &fakeStore{}, notifier), notifier
}

func completeProfile() entity.Profile {
	return entity.Profile{
		Age:        28,
		Gender:     entity.GenderMale,
		GenderPref: entity.GenderPrefAny,
		Occupation: entity.OccupationRemote,
		Schedule:   entity.ScheduleOwl,
		Smoking:    entity.SmokingNo,
		PetsOwn:    entity.PetsOwnCat,
		PetsOK:     entity.PetsOKFine,
		Noise:      entity.NoiseNormal,
		Guests:     entity.GuestsRare,
		Cleaning:   entity.CleaningSchedule,
		Alcohol:    entity.AlcoholNone,
		Expenses:   entity.ExpensesSplitEqually,
		About:      "Люблю тишину по утрам",
	}
}

func TestEnsureFromTelegramPassesIdentityThrough(t *testing.T) {
	repo := &fakeRepo{user: entity.User{TelegramID: 42, Status: entity.StatusDraft}}

	identity := entity.TelegramIdentity{ID: 42, Name: "Арсений", Username: "arsenii"}
	user, err := newService(repo).EnsureFromTelegram(t.Context(), identity)
	if err != nil {
		t.Fatalf("EnsureFromTelegram() failed: %v", err)
	}

	if repo.got != identity {
		t.Errorf("repository received %+v, want %+v", repo.got, identity)
	}
	if user.TelegramID != 42 {
		t.Errorf("TelegramID = %d, want 42", user.TelegramID)
	}
}

func TestEnsureFromTelegramSubstitutesAnEmptyName(t *testing.T) {
	repo := &fakeRepo{user: entity.User{Status: entity.StatusDraft}}

	if _, err := newService(repo).EnsureFromTelegram(t.Context(), entity.TelegramIdentity{ID: 42}); err != nil {
		t.Fatalf("EnsureFromTelegram() failed: %v", err)
	}
	if got, want := repo.got.Name, "user42"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
}

func TestEnsureFromTelegramRejectsBannedAccounts(t *testing.T) {
	repo := &fakeRepo{user: entity.User{TelegramID: 42, Status: entity.StatusBanned}}

	_, err := newService(repo).EnsureFromTelegram(t.Context(), entity.TelegramIdentity{ID: 42, Name: "Mallory"})
	if !errors.Is(err, ErrBanned) {
		t.Fatalf("EnsureFromTelegram() error = %v, want ErrBanned", err)
	}
}

func TestEnsureFromTelegramRejectsIdentityWithoutID(t *testing.T) {
	repo := &fakeRepo{}

	if _, err := newService(repo).EnsureFromTelegram(t.Context(), entity.TelegramIdentity{Name: "Никто"}); err == nil {
		t.Fatal("EnsureFromTelegram() accepted an identity with no id")
	}
	if repo.got.Name != "" {
		t.Error("the repository was called despite an invalid identity")
	}
}

func TestEnsureFromTelegramPropagatesRepositoryFailure(t *testing.T) {
	wantErr := errors.New("connection refused")
	repo := &fakeRepo{err: wantErr}

	_, err := newService(repo).EnsureFromTelegram(t.Context(), entity.TelegramIdentity{ID: 42, Name: "Арсений"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("EnsureFromTelegram() error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestUpdateProfileRejectsAValueOutsideTheSet(t *testing.T) {
	repo := &fakeRepo{}

	_, err := newService(repo).UpdateProfile(t.Context(), uuid.New(), entity.ProfilePatch{
		Schedule: pointer.To(entity.ParseSchedule("nocturnal")),
	})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidPatch) {
		t.Fatalf("UpdateProfile() error = %v, want ErrInvalidPatch", err)
	}
	if !slices.Equal(fields.Fields, []string{"schedule"}) {
		t.Errorf("Fields = %v, want [schedule]", fields.Fields)
	}

	if repo.patched.Schedule != nil {
		t.Error("the repository was called despite an invalid patch")
	}
}

func TestSubmitNamesEverythingStillMissing(t *testing.T) {
	profile := completeProfile()
	profile.About = ""
	repo := &fakeRepo{user: entity.User{Status: entity.StatusDraft, Profile: profile}}

	_, err := newService(repo).Submit(t.Context(), uuid.New())

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrIncomplete) {
		t.Fatalf("Submit() error = %v, want ErrIncomplete", err)
	}

	if !slices.Equal(fields.Fields, []string{"about", "photo"}) {
		t.Errorf("Fields = %v, want [about photo]", fields.Fields)
	}
	if repo.statusCall {
		t.Error("an incomplete profile must not change status")
	}
}

func TestSubmitSendsACompleteDraftToModeration(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Status:   entity.StatusDraft,
		Profile:  completeProfile(),
		PhotoKey: "photos/x.jpg",
	}}

	if _, err := newService(repo).Submit(t.Context(), uuid.New()); err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}
	if repo.setStatus != entity.StatusPending {
		t.Errorf("status = %q, want %q", repo.setStatus, entity.StatusPending)
	}
}

func TestSubmitLeavesAnApprovedAccountActive(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Status:   entity.StatusActive,
		Profile:  completeProfile(),
		PhotoKey: "photos/x.jpg",
	}}

	user, err := newService(repo).Submit(t.Context(), uuid.New())
	if err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}
	if repo.statusCall {
		t.Errorf("status was moved to %q, want it left alone", repo.setStatus)
	}
	if !user.Visible() {
		t.Error("an approved account must stay visible after an edit")
	}
}

func TestEditsOfAnApprovedAccountReachModerationOnSubmitOnly(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Status:      entity.StatusActive,
		Profile:     completeProfile(),
		PhotoKey:    "photos/x.jpg",
		PhotoStatus: entity.ModerationPending,
		AboutStatus: entity.ModerationPending,
	}}
	svc, notifier := newModeratedService(repo)

	about := "новый текст"
	if _, err := svc.UpdateProfile(t.Context(), uuid.New(), entity.ProfilePatch{About: &about}); err != nil {
		t.Fatalf("UpdateProfile() failed: %v", err)
	}
	if _, err := svc.SetPhoto(t.Context(), uuid.New(), bytes.NewReader(jpegFile)); err != nil {
		t.Fatalf("SetPhoto() failed: %v", err)
	}
	if len(notifier.queued) != 0 {
		t.Fatalf("moderation got %d cards before the button, want none", len(notifier.queued))
	}

	if _, err := svc.Submit(t.Context(), uuid.New()); err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}
	if len(notifier.queued) != 1 {
		t.Errorf("moderation got %d cards on submit, want one for photo and text together", len(notifier.queued))
	}
	if repo.statusCall {
		t.Errorf("status was moved to %q, want it left alone", repo.setStatus)
	}
}

func TestSubmitOfAnUntouchedApprovedAccountIsSilent(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Status:      entity.StatusActive,
		Profile:     completeProfile(),
		PhotoKey:    "photos/x.jpg",
		PhotoStatus: entity.ModerationOK,
		AboutStatus: entity.ModerationOK,
	}}
	svc, notifier := newModeratedService(repo)

	if _, err := svc.Submit(t.Context(), uuid.New()); err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}
	if len(notifier.queued) != 0 {
		t.Errorf("moderation got %d cards for an untouched profile, want none", len(notifier.queued))
	}
}

func TestSetPhotoKeepsOneObjectPerUser(t *testing.T) {
	store := &fakeStore{}
	repo := &fakeRepo{}
	id := uuid.New()

	if _, err := New(repo, store, nil).SetPhoto(t.Context(), id, bytes.NewReader(jpegFile)); err != nil {
		t.Fatalf("SetPhoto() failed: %v", err)
	}

	want := "photos/" + id.String() + ".jpg"
	if store.put != want {
		t.Errorf("stored under %q, want %q", store.put, want)
	}
	if repo.photoKey != want {
		t.Errorf("recorded key %q, want %q", repo.photoKey, want)
	}
}

func TestSetPhotoRefusesWhatIsNotAnImage(t *testing.T) {
	store := &fakeStore{}
	repo := &fakeRepo{}

	_, err := New(repo, store, nil).SetPhoto(t.Context(), uuid.New(), strings.NewReader("PDF-1.4 или что угодно"))
	if !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("SetPhoto() error = %v, want ErrUnsupportedImage", err)
	}
	if store.put != "" {
		t.Errorf("stored %q, want nothing written", store.put)
	}
	if repo.photoKey != "" {
		t.Errorf("recorded key %q, want the row left alone", repo.photoKey)
	}
}

func TestPhotoURLHidesAnUnreviewedPhoto(t *testing.T) {
	svc := newService(&fakeRepo{})
	user := entity.User{PhotoKey: "photos/x.jpg", PhotoStatus: entity.ModerationPending}

	if got := svc.PhotoURL(user); got != "" {
		t.Errorf("PhotoURL() = %q, want empty until a moderator has looked", got)
	}

	if got := svc.OwnPhotoURL(user); got == "" {
		t.Error("OwnPhotoURL() is empty, the owner must see their own upload")
	}
}

func TestSubmitPutsTheQuestionnaireInFrontOfTheModerator(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Name:     "Катя",
		Status:   entity.StatusDraft,
		Profile:  completeProfile(),
		PhotoKey: "photos/x.jpg",
	}}
	svc, notifier := newModeratedService(repo)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := svc.Submit(ctx, uuid.New()); err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}
	if len(notifier.queued) != 1 {
		t.Fatalf("queued %d questionnaires, want 1", len(notifier.queued))
	}
	if notifier.queued[0].Name != "Катя" {
		t.Errorf("queued %q, want the submitted account", notifier.queued[0].Name)
	}
	if err := notifier.ctxs[0].Err(); err != nil {
		t.Errorf("the moderator got a cancelled context: %v", err)
	}
}

func TestResubmitDoesNotDuplicateTheCard(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Status:   entity.StatusPending,
		Profile:  completeProfile(),
		PhotoKey: "photos/x.jpg",
	}}
	svc, notifier := newModeratedService(repo)

	if _, err := svc.Submit(t.Context(), uuid.New()); err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}
	if len(notifier.queued) != 0 {
		t.Errorf("queued %d questionnaires, want none", len(notifier.queued))
	}
}

func TestPhotoOfAnUnfinishedProfileWaitsForTheSubmit(t *testing.T) {
	repo := &fakeRepo{user: entity.User{Status: entity.StatusDraft}}
	svc, notifier := newModeratedService(repo)

	if _, err := svc.SetPhoto(t.Context(), uuid.New(), bytes.NewReader(jpegFile)); err != nil {
		t.Fatalf("SetPhoto() failed: %v", err)
	}
	if len(notifier.queued) != 0 {
		t.Errorf("queued %d questionnaires, want none", len(notifier.queued))
	}
}

func TestAStandWithoutABotStillWorks(t *testing.T) {
	repo := &fakeRepo{user: entity.User{Status: entity.StatusActive, Profile: completeProfile()}}

	if _, err := newService(repo).SetPhoto(t.Context(), uuid.New(), bytes.NewReader(jpegFile)); err != nil {
		t.Fatalf("SetPhoto() failed without a moderation chat: %v", err)
	}
}

func TestSetSourceAcceptsOnlyTrackingCodes(t *testing.T) {
	good := []string{"a", "hsetndr", "hse_tndr_2", strings.Repeat("z", 32)}
	for _, code := range good {
		repo := &fakeRepo{}
		if err := newService(repo).SetSource(t.Context(), uuid.New(), code); err != nil {
			t.Errorf("SetSource(%q) failed: %v", code, err)
			continue
		}
		if repo.source != code {
			t.Errorf("stored source = %q, want %q", repo.source, code)
		}
	}

	bad := []string{"", "HSETNDR", "hse tndr", "hse-tndr", "хсе", strings.Repeat("z", 33)}
	for _, code := range bad {
		repo := &fakeRepo{}
		err := newService(repo).SetSource(t.Context(), uuid.New(), code)

		var fields *entity.FieldsError
		if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidSource) {
			t.Errorf("SetSource(%q) error = %v, want ErrInvalidSource with fields", code, err)
			continue
		}
		if !slices.Equal(fields.Fields, []string{"source"}) {
			t.Errorf("fields = %v, want [source]", fields.Fields)
		}
		if repo.sourceCall {
			t.Errorf("SetSource(%q) reached the database", code)
		}
	}
}
