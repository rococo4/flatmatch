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

// fakeRepo records what the service asked for and returns what the test wants.
// Hand-written rather than generated: a fake that can be read in ten seconds
// beats a mock that cannot.
type fakeRepo struct {
	got  entity.TelegramIdentity
	user entity.User
	err  error

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

// jpegFile is the shortest thing that gets recognised as a photo.
var jpegFile = []byte("\xff\xd8\xff\xe0")

// fakeStore swallows uploads. Nothing here cares what the bytes were.
type fakeStore struct{ put string }

func (f *fakeStore) Put(_ context.Context, key string, _ io.Reader, _ string) error {
	f.put = key
	return nil
}
func (f *fakeStore) Delete(context.Context, string) error { return nil }
func (f *fakeStore) URL(key string) string                { return "/media/" + key }

func newService(repo *fakeRepo) *Service { return New(repo, &fakeStore{}) }

// completeProfile is an answer to every question in §4, so tests about one
// missing field can remove exactly that one.
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
	// Telegram guarantees a first name, but an empty one would leave the
	// greeting blank and violate the NOT NULL on name.
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
	// Nothing may reach the database once a value has been refused.
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
	// The photo lives on the account, not the questionnaire, and is just as
	// mandatory (§4 шаг 11) — a list that forgot it would send someone back to
	// a screen they already finished.
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
	// Editing must not drop someone out of the pools while a moderator gets to
	// them: the photo and «о себе» carry their own statuses for that (§5).
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

func TestSetPhotoKeepsOneObjectPerUser(t *testing.T) {
	// A key derived from the id means a re-upload overwrites rather than
	// littering the bucket with files nobody will ever look for.
	store := &fakeStore{}
	repo := &fakeRepo{}
	id := uuid.New()

	if _, err := New(repo, store).SetPhoto(t.Context(), id, bytes.NewReader(jpegFile)); err != nil {
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
	// The check belongs to the service, not to the one handler that happens to
	// call it today: nothing may reach storage or the row until the file has
	// been recognised.
	store := &fakeStore{}
	repo := &fakeRepo{}

	_, err := New(repo, store).SetPhoto(t.Context(), uuid.New(), strings.NewReader("PDF-1.4 или что угодно"))
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

	// Others see nothing and get a placeholder…
	if got := svc.PhotoURL(user); got != "" {
		t.Errorf("PhotoURL() = %q, want empty until a moderator has looked", got)
	}
	// …while the owner still sees what they uploaded, or they could not judge
	// whether to replace it.
	if got := svc.OwnPhotoURL(user); got == "" {
		t.Error("OwnPhotoURL() is empty, the owner must see their own upload")
	}
}
