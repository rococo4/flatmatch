package pointsapi

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/pointsrepo"
)

type stubPhotos struct{}

func (stubPhotos) URL(key string) string {
	if key == "" {
		return ""
	}
	return "/media/" + key
}

func fullCard() pointsrepo.FeedPoint {
	creator := uuid.MustParse("2b0cb2f6-6b2f-4e6a-9ad3-1f8e2a0d5c11")
	return pointsrepo.FeedPoint{
		Point: entity.Point{
			ID:           uuid.New(),
			Type:         entity.PointFlat,
			Status:       entity.PointActive,
			Source:       entity.SourceUserLink,
			CreatedBy:    &creator,
			ZoneID:       "st-sokol",
			PriceTotal:   pointer.To(90000),
			Rooms:        pointer.To(2),
			Isolated:     pointer.To(true),
			Bathrooms:    pointer.To(entity.BathroomsSeparate),
			Utilities:    pointer.To(entity.UtilitiesMeters),
			Restrictions: []entity.Restriction{entity.RestrictionNoPets, entity.RestrictionNoCouples},
			PhotoKeys:    []string{"points/flat.jpg"},
			URLNorm:      pointer.To("https://avito.ru/1"),
			Deposit:      pointer.To(45000),
			CreatedAt:    time.Now().Add(-72 * time.Hour),
		},
		PoolCount: 6,
	}
}

func TestMemberParamsAreAlwaysArrays(t *testing.T) {
	sept := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		params entity.SearchParams
		want   []string
	}{
		"empty sets": {
			params: entity.SearchParams{PriceMax: 45000},
			want:   []string{`"rooms":[]`, `"terms":[]`, `"move_in":[]`},
		},
		"filled sets": {
			params: entity.SearchParams{PriceMax: 45000, Rooms: []int{2, 3},
				Terms: []entity.Term{entity.TermYear}, MoveInMonths: []time.Time{sept}},
			want: []string{`"rooms":[2,3]`, `"terms":["year"]`, `"move_in":["2026-09"]`},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(toParamsDTO(tt.params))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(string(raw), want) {
					t.Errorf("params %s does not contain %s", raw, want)
				}
			}
		})
	}
}

func TestCardCarriesNoCreator(t *testing.T) {
	card := fullCard()

	raw, err := json.Marshal(toCard(card, stubPhotos{}, zoneDTO{ID: "st-sokol"}, uuid.New()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, forbidden := range []string{"created_by", "creator", card.Point.CreatedBy.String()} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("card JSON leaks %q:\n%s", forbidden, raw)
		}
	}
}

func TestCardKeepsDictionaryCodes(t *testing.T) {
	card := fullCard()
	card.Point.Type = entity.PointIntentHome
	card.Point.Stage = pointer.To(entity.StageMovingIn)
	card.Point.PriceShare = pointer.To(40000)
	card.Point.MoveIn = pointer.To(time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC))

	got := toCard(card, stubPhotos{}, zoneDTO{ID: "st-sokol"}, uuid.New())
	if got.Stage != "moving_in" || got.Bathrooms != "separate" || got.Utilities != "meters" {
		t.Errorf("dictionary codes lost: %+v", got)
	}
	if len(got.Restrictions) != 2 || got.Restrictions[0] != "no_pets" || got.Restrictions[1] != "no_couples" {
		t.Errorf("restrictions = %v, want the codes", got.Restrictions)
	}
	if got.MoveIn != "2026-09-15" {
		t.Errorf("MoveIn = %q, want 2026-09-15", got.MoveIn)
	}

	if got.CreatedDaysAgo != 3 {
		t.Errorf("CreatedDaysAgo = %d, want 3", got.CreatedDaysAgo)
	}

	if got.URL != "https://avito.ru/1" || got.PhotoURLs[0] != "/media/points/flat.jpg" {
		t.Errorf("link or photo lost: %+v", got)
	}
}

func TestAuthorCarriesNoContacts(t *testing.T) {
	card := fullCard()
	card.Point.Type = entity.PointIntentHome
	card.Point.Stage = pointer.To(entity.StageLive)
	card.Author = &entity.User{
		ID:          uuid.New(),
		TelegramID:  777000123,
		Name:        "Катя",
		Username:    "katya",
		Profile:     entity.Profile{Age: 27, Schedule: entity.ScheduleOwl},
		PhotoKey:    "users/katya.jpg",
		PhotoStatus: entity.ModerationOK,
	}

	raw, err := json.Marshal(toCard(card, stubPhotos{}, zoneDTO{ID: "st-sokol"}, uuid.New()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{"username", "tg_id", "katya\"", "777000123"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("author JSON leaks %q:\n%s", forbidden, raw)
		}
	}
	if !strings.Contains(string(raw), "/media/users/katya.jpg") {
		t.Errorf("approved author photo did not travel:\n%s", raw)
	}

	card.Author.PhotoStatus = entity.ModerationPending
	if got := toCard(card, stubPhotos{}, zoneDTO{}, uuid.New()); got.Author.PhotoURL != "" {
		t.Errorf("unapproved author photo leaked: %q", got.Author.PhotoURL)
	}
}

func TestRequestParsesDictionaries(t *testing.T) {
	body := createRequest{
		Type:         "flat",
		URL:          "https://avito.ru/1",
		ZoneID:       "st-sokol",
		PriceTotal:   90000,
		Bathrooms:    pointer.To("two"),
		Utilities:    pointer.To("included"),
		Restrictions: []string{"no_smoking", "no_pets"},
		PhotoKeys:    []string{"points/flat.jpg"},
	}

	flat := body.toFlat()
	if flat.Bathrooms == nil || *flat.Bathrooms != entity.BathroomsTwo ||
		flat.Utilities == nil || *flat.Utilities != entity.UtilitiesIncluded {
		t.Errorf("flat form lost its dictionaries: %+v", flat)
	}
	if len(flat.Restrictions) != 2 || flat.Restrictions[0] != entity.RestrictionNoSmoking {
		t.Errorf("restrictions = %v, want the parsed codes", flat.Restrictions)
	}

	unknown := body
	unknown.Bathrooms = pointer.To("three")
	if got := unknown.toFlat().Bathrooms; got == nil || *got != entity.BathroomsUnknown {
		t.Errorf("unknown bathrooms = %v, want the sentinel", got)
	}

	empty := body
	empty.Bathrooms = pointer.To("")
	if got := empty.toFlat().Bathrooms; got != nil {
		t.Errorf("empty bathrooms = %v, want nil", got)
	}
}

func TestIntentHomeRequestReadsTheMoveInDay(t *testing.T) {
	body := createRequest{
		Type:       "intent_home",
		Stage:      "moving_in",
		ZoneID:     "st-sokol",
		PriceShare: 40000,
		PhotoKeys:  []string{"points/room.jpg"},
		MoveIn:     pointer.To("2026-09-15"),
	}

	in, err := body.toIntentHome()
	if err != nil {
		t.Fatalf("toIntentHome() failed: %v", err)
	}
	if in.Stage != entity.StageMovingIn || in.MoveIn == nil ||
		!in.MoveIn.Equal(time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("intent_home form lost the stage or the day: %+v", in)
	}

	broken := body
	broken.MoveIn = pointer.To("15 сентября")
	var fields *entity.FieldsError
	_, err = broken.toIntentHome()
	if !errors.As(err, &fields) || len(fields.Fields) != 1 || fields.Fields[0] != "move_in" {
		t.Errorf("error = %v, want invalid move_in", err)
	}
}

func TestCardStatusSpeaksTheCloseReason(t *testing.T) {
	cases := map[string]string{
		"gone":     "gone",
		"expired":  "expired",
		"found":    "found",
		"rejected": "rejected",
		"closed":   "closed",
	}

	for reason, want := range cases {
		point := entity.Point{Status: entity.PointClosed, CloseReason: pointer.To(reason)}
		if got := cardStatus(point); got != want {
			t.Errorf("cardStatus(closed as %s) = %q, want %q", reason, got, want)
		}
	}

	for _, status := range []entity.PointStatus{entity.PointActive, entity.PointPaused, entity.PointPending} {
		if got := cardStatus(entity.Point{Status: status}); got != string(status) {
			t.Errorf("cardStatus(%s) = %q, want the status itself", status, got)
		}
	}
	if got := cardStatus(entity.Point{Status: entity.PointClosed}); got != "closed" {
		t.Errorf("cardStatus(closed without a reason) = %q, want closed", got)
	}
}

func TestRestorableIsTheCreatorsOwnPausedPoint(t *testing.T) {
	card := fullCard()
	creator := *card.Point.CreatedBy

	if toCard(card, stubPhotos{}, zoneDTO{}, creator).Restorable {
		t.Error("an active point offers a restore")
	}

	card.Point.Status = entity.PointPaused
	if !toCard(card, stubPhotos{}, zoneDTO{}, creator).Restorable {
		t.Error("the creator of a paused point cannot restore it")
	}
	if toCard(card, stubPhotos{}, zoneDTO{}, uuid.New()).Restorable {
		t.Error("a stranger is offered a restore of a paused point")
	}

	card.Point.CreatedBy = nil
	if toCard(card, stubPhotos{}, zoneDTO{}, creator).Restorable {
		t.Error("a point without a creator offers a restore")
	}
}

func TestCardCarriesFreshness(t *testing.T) {
	card := fullCard()
	card.Point.LastSeenAlive = time.Date(2026, 8, 17, 12, 30, 0, 0, time.UTC)
	card.GoneVotes = 2
	card.MyVote = pointer.To(false)

	raw, err := json.Marshal(toCard(card, stubPhotos{}, zoneDTO{}, uuid.New()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"last_seen_alive":"2026-08-17T12:30:00Z"`, `"gone_votes":2`, `"my_vote":false`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("card JSON %s does not carry %s", raw, want)
		}
	}

	card.MyVote = nil
	silent, err := json.Marshal(toCard(card, stubPhotos{}, zoneDTO{}, uuid.New()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(silent), "my_vote") {
		t.Errorf("a card without a vote carries my_vote: %s", silent)
	}
}

func TestConfirmedAtSpeaksOnlyOfARealCheck(t *testing.T) {
	card := fullCard()
	card.Point.LastSeenAlive = card.Point.CreatedAt

	fresh := toCard(card, stubPhotos{}, zoneDTO{}, uuid.New())
	if fresh.ConfirmedAt != nil {
		t.Errorf("ConfirmedAt = %v of a point nobody checked, want nil", fresh.ConfirmedAt)
	}
	raw, err := json.Marshal(fresh)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "confirmed_at") {
		t.Errorf("an unchecked card carries confirmed_at: %s", raw)
	}

	checked := card.Point.CreatedAt.Add(time.Hour)
	card.Point.LastSeenAlive = checked
	got := toCard(card, stubPhotos{}, zoneDTO{}, uuid.New())
	if got.ConfirmedAt == nil || !got.ConfirmedAt.Equal(checked) {
		t.Errorf("ConfirmedAt = %v, want %s", got.ConfirmedAt, checked)
	}
	confirmed, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(confirmed), "confirmed_at") {
		t.Errorf("a checked card does not carry confirmed_at: %s", confirmed)
	}
}
