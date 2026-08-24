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
	photoKeys  []string
	photoCall  bool
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

func (f *fakeRepo) SetPhotos(_ context.Context, _ uuid.UUID, keys []string) (entity.User, error) {
	f.photoCall = true
	f.photoKeys = keys

	updated := f.user
	updated.PhotoKeys = keys
	updated.PhotoStatus = entity.ModerationPending
	return updated, f.err
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

type fakeStore struct {
	put     []string
	deleted []string
}

func (f *fakeStore) Put(_ context.Context, key string, _ io.Reader, _ string) error {
	f.put = append(f.put, key)
	return nil
}

func (f *fakeStore) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

func (f *fakeStore) URL(key string) string { return "/media/" + key }

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
		Status:    entity.StatusDraft,
		Profile:   completeProfile(),
		PhotoKeys: []string{"photos/x.jpg"},
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
		Status:    entity.StatusActive,
		Profile:   completeProfile(),
		PhotoKeys: []string{"photos/x.jpg"},
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
		PhotoKeys:   []string{"photos/x.jpg"},
		PhotoStatus: entity.ModerationPending,
		AboutStatus: entity.ModerationPending,
	}}
	svc, notifier := newModeratedService(repo)

	about := "новый текст"
	if _, err := svc.UpdateProfile(t.Context(), uuid.New(), entity.ProfilePatch{About: &about}); err != nil {
		t.Fatalf("UpdateProfile() failed: %v", err)
	}
	if _, err := svc.AddPhoto(t.Context(), uuid.New(), bytes.NewReader(jpegFile)); err != nil {
		t.Fatalf("AddPhoto() failed: %v", err)
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
		PhotoKeys:   []string{"photos/x.jpg"},
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

func TestAddPhotoAppendsAKeyOfItsOwn(t *testing.T) {
	store := &fakeStore{}
	repo := &fakeRepo{user: entity.User{
		Status:      entity.StatusDraft,
		PhotoKeys:   []string{"photos/first.jpg"},
		PhotoStatus: entity.ModerationOK,
	}}
	id := uuid.New()

	updated, err := New(repo, store, nil).AddPhoto(t.Context(), id, bytes.NewReader(jpegFile))
	if err != nil {
		t.Fatalf("AddPhoto() failed: %v", err)
	}

	if len(store.put) != 1 {
		t.Fatalf("store got %v, want exactly one file", store.put)
	}
	key := store.put[0]
	if !strings.HasPrefix(key, "photos/"+id.String()+"-") || !strings.HasSuffix(key, ".jpg") {
		t.Errorf("stored under %q, want photos/<user>-<hex>.jpg", key)
	}
	if !slices.Equal(repo.photoKeys, []string{"photos/first.jpg", key}) {
		t.Errorf("recorded %v, want the new key appended to the end", repo.photoKeys)
	}
	if updated.PhotoStatus != entity.ModerationPending {
		t.Errorf("photo status = %q, want the whole set back on review", updated.PhotoStatus)
	}
}

func TestAddPhotoStopsAtTheFifth(t *testing.T) {
	store := &fakeStore{}
	keys := []string{"photos/1.jpg", "photos/2.jpg", "photos/3.jpg", "photos/4.jpg", "photos/5.jpg"}
	repo := &fakeRepo{user: entity.User{Status: entity.StatusActive, PhotoKeys: keys}}

	_, err := New(repo, store, nil).AddPhoto(t.Context(), uuid.New(), bytes.NewReader(jpegFile))

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrPhotoLimit) {
		t.Fatalf("AddPhoto() error = %v, want ErrPhotoLimit", err)
	}
	if !slices.Equal(fields.Fields, []string{"photo"}) {
		t.Errorf("fields = %v, want [photo]", fields.Fields)
	}
	if len(store.put) != 0 || repo.photoCall {
		t.Errorf("a sixth photo reached the store %v or the row %v", store.put, repo.photoCall)
	}
}

func TestAddPhotoRefusesWhatIsNotAnImage(t *testing.T) {
	store := &fakeStore{}
	repo := &fakeRepo{}

	_, err := New(repo, store, nil).AddPhoto(t.Context(), uuid.New(), strings.NewReader("PDF-1.4 или что угодно"))
	if !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("AddPhoto() error = %v, want ErrUnsupportedImage", err)
	}
	if len(store.put) != 0 {
		t.Errorf("stored %v, want nothing written", store.put)
	}
	if repo.photoCall {
		t.Error("the row was touched despite an unsupported file")
	}
}

func TestRemovePhotoDropsTheChosenOne(t *testing.T) {
	store := &fakeStore{}
	repo := &fakeRepo{user: entity.User{
		Status:      entity.StatusActive,
		PhotoKeys:   []string{"photos/a.jpg", "photos/b.jpg"},
		PhotoStatus: entity.ModerationOK,
	}}

	updated, err := New(repo, store, nil).RemovePhoto(t.Context(), uuid.New(), 0)
	if err != nil {
		t.Fatalf("RemovePhoto() failed: %v", err)
	}
	if !slices.Equal(repo.photoKeys, []string{"photos/b.jpg"}) {
		t.Errorf("recorded %v, want [photos/b.jpg]", repo.photoKeys)
	}
	if !slices.Equal(store.deleted, []string{"photos/a.jpg"}) {
		t.Errorf("deleted %v, want the dropped file gone", store.deleted)
	}
	if updated.PhotoStatus != entity.ModerationPending {
		t.Errorf("photo status = %q, want the whole set back on review", updated.PhotoStatus)
	}
}

func TestRemovePhotoKeepsTheLastOneOfASubmittedProfile(t *testing.T) {
	submitted := []entity.User{
		{Status: entity.StatusPending, PhotoKeys: []string{"photos/a.jpg"}},
		{Status: entity.StatusActive, PhotoKeys: []string{"photos/a.jpg"}, PhotoStatus: entity.ModerationOK},
		{Status: entity.StatusDraft, PhotoKeys: []string{"photos/a.jpg"}, PhotoStatus: entity.ModerationRejected},
	}

	for _, user := range submitted {
		repo := &fakeRepo{user: user}

		_, err := newService(repo).RemovePhoto(t.Context(), uuid.New(), 0)

		var fields *entity.FieldsError
		if !errors.As(err, &fields) || !errors.Is(err, ErrLastPhoto) {
			t.Fatalf("%s/%s: RemovePhoto() error = %v, want ErrLastPhoto",
				user.Status, user.PhotoStatus, err)
		}
		if !slices.Equal(fields.Fields, []string{"photo"}) {
			t.Errorf("fields = %v, want [photo]", fields.Fields)
		}
		if repo.photoCall {
			t.Error("the row was touched despite the refusal")
		}
	}
}

func TestRemovePhotoLetsAFreshDraftEmptyTheSet(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Status:      entity.StatusDraft,
		PhotoKeys:   []string{"photos/a.jpg"},
		PhotoStatus: entity.ModerationPending,
		AboutStatus: entity.ModerationPending,
	}}

	if _, err := newService(repo).RemovePhoto(t.Context(), uuid.New(), 0); err != nil {
		t.Fatalf("RemovePhoto() failed: %v", err)
	}
	if len(repo.photoKeys) != 0 {
		t.Errorf("recorded %v, want an empty set", repo.photoKeys)
	}
}

func TestRemovePhotoRefusesAnIndexOutsideTheSet(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Status:    entity.StatusActive,
		PhotoKeys: []string{"photos/a.jpg", "photos/b.jpg"},
	}}

	_, err := newService(repo).RemovePhoto(t.Context(), uuid.New(), 5)

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidPhotoIndex) {
		t.Fatalf("RemovePhoto() error = %v, want ErrInvalidPhotoIndex", err)
	}
	if !slices.Equal(fields.Fields, []string{"index"}) {
		t.Errorf("fields = %v, want [index]", fields.Fields)
	}
	if repo.photoCall {
		t.Error("the row was touched despite an index outside the set")
	}
}

func TestReorderPhotosStoresTheNewOrder(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Status:      entity.StatusActive,
		PhotoKeys:   []string{"photos/a.jpg", "photos/b.jpg", "photos/c.jpg"},
		PhotoStatus: entity.ModerationOK,
	}}

	updated, err := newService(repo).ReorderPhotos(t.Context(), uuid.New(), []int{2, 0, 1})
	if err != nil {
		t.Fatalf("ReorderPhotos() failed: %v", err)
	}
	if !slices.Equal(repo.photoKeys, []string{"photos/c.jpg", "photos/a.jpg", "photos/b.jpg"}) {
		t.Errorf("recorded %v, want [c a b]", repo.photoKeys)
	}
	if updated.PhotoStatus != entity.ModerationPending {
		t.Errorf("photo status = %q, want the whole set back on review", updated.PhotoStatus)
	}
}

func TestReorderPhotosRefusesAnythingButAPermutation(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Status:    entity.StatusActive,
		PhotoKeys: []string{"photos/a.jpg", "photos/b.jpg"},
	}}

	_, err := newService(repo).ReorderPhotos(t.Context(), uuid.New(), []int{0, 0})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidPhotoOrder) {
		t.Fatalf("ReorderPhotos() error = %v, want ErrInvalidPhotoOrder", err)
	}
	if !slices.Equal(fields.Fields, []string{"order"}) {
		t.Errorf("fields = %v, want [order]", fields.Fields)
	}
	if repo.photoCall {
		t.Error("the row was touched despite a broken order")
	}
}

func TestPhotoURLsHideAnUnreviewedSet(t *testing.T) {
	svc := newService(&fakeRepo{})
	user := entity.User{
		PhotoKeys:   []string{"photos/x.jpg", "photos/y.jpg"},
		PhotoStatus: entity.ModerationPending,
	}

	if got := svc.PhotoURLs(user); len(got) != 0 {
		t.Errorf("PhotoURLs() = %v, want empty until a moderator has looked", got)
	}

	if got := svc.OwnPhotoURLs(user); !slices.Equal(got, []string{"/media/photos/x.jpg", "/media/photos/y.jpg"}) {
		t.Errorf("OwnPhotoURLs() = %v, the owner must see their own uploads in order", got)
	}

	user.PhotoStatus = entity.ModerationOK
	if got := svc.PhotoURLs(user); !slices.Equal(got, []string{"/media/photos/x.jpg", "/media/photos/y.jpg"}) {
		t.Errorf("PhotoURLs() = %v, want the approved set in order", got)
	}
}

func TestSubmitPutsTheQuestionnaireInFrontOfTheModerator(t *testing.T) {
	repo := &fakeRepo{user: entity.User{
		Name:      "Катя",
		Status:    entity.StatusDraft,
		Profile:   completeProfile(),
		PhotoKeys: []string{"photos/x.jpg"},
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
		Status:    entity.StatusPending,
		Profile:   completeProfile(),
		PhotoKeys: []string{"photos/x.jpg"},
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

	if _, err := svc.AddPhoto(t.Context(), uuid.New(), bytes.NewReader(jpegFile)); err != nil {
		t.Fatalf("AddPhoto() failed: %v", err)
	}
	if len(notifier.queued) != 0 {
		t.Errorf("queued %d questionnaires, want none", len(notifier.queued))
	}
}

func TestAStandWithoutABotStillWorks(t *testing.T) {
	repo := &fakeRepo{user: entity.User{Status: entity.StatusActive, Profile: completeProfile()}}

	if _, err := newService(repo).AddPhoto(t.Context(), uuid.New(), bytes.NewReader(jpegFile)); err != nil {
		t.Fatalf("AddPhoto() failed without a moderation chat: %v", err)
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
