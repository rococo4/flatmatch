package pointsrepo

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

const testTGBase int64 = -993000000

const testZone = "s-test-points"

func cleanTestUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	clean := func() {
		exec := func(query string, arg any) {
			if _, err := pool.Exec(context.Background(), query, arg); err != nil {
				t.Errorf("clean up test points: %v", err)
			}
		}
		for _, q := range []string{
			`DELETE FROM point_votes WHERE point_id IN (SELECT id FROM points WHERE zone_id = $1)`,
			`DELETE FROM likes_points WHERE point_id IN (SELECT id FROM points WHERE zone_id = $1)`,
			`DELETE FROM points WHERE zone_id = $1`,
		} {
			exec(q, testZone)
		}
		for _, q := range []string{
			`DELETE FROM likes_points WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM point_votes WHERE point_id IN (SELECT id FROM points WHERE created_by IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000))`,
			`DELETE FROM likes_points WHERE point_id IN (SELECT id FROM points WHERE created_by IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000))`,
			`DELETE FROM points WHERE created_by IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM searches WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`,
		} {
			exec(q, testTGBase)
		}
	}
	clean()
	t.Cleanup(clean)
}

func seedUser(t *testing.T, pool *pgxpool.Pool, n int64) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (tg_id, name, status, gender, gender_pref)
		VALUES ($1, 'test', 'active', 'male', 'any') RETURNING id`, testTGBase-n).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func flatPoint(createdBy *uuid.UUID, source entity.PointSource, urlNorm string, status entity.PointStatus) entity.Point {
	p := entity.Point{
		Type:       entity.PointFlat,
		Status:     status,
		Source:     source,
		CreatedBy:  createdBy,
		ZoneID:     testZone,
		PriceTotal: pointer.To(90000),
		PhotoKeys:  []string{"points/flat.jpg"},
	}
	if urlNorm != "" {
		p.URLNorm = pointer.To(urlNorm)
	}
	return p
}

func intentHome(createdBy uuid.UUID, stage entity.PointStage) entity.Point {
	p := entity.Point{
		Type:       entity.PointIntentHome,
		Stage:      pointer.To(stage),
		Status:     entity.PointActive,
		Source:     entity.SourceUserLink,
		CreatedBy:  &createdBy,
		ZoneID:     testZone,
		PriceShare: pointer.To(45000),
		PhotoKeys:  []string{"points/room.jpg"},
	}
	if stage == entity.StageMovingIn {
		p.MoveIn = pointer.To(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	}
	return p
}

func closePoint(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		UPDATE points SET status = 'closed', close_reason = 'closed' WHERE id = $1`, id)
	if err != nil {
		t.Fatalf("close point %s: %v", id, err)
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestCreateRoundtrip(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1)

	in := flatPoint(&user, entity.SourceUserLink, "https://avito.ru/1", "")
	in.Rooms = pointer.To(2)
	in.Isolated = pointer.To(true)
	in.Bathrooms = pointer.To(entity.BathroomsSeparate)
	in.Utilities = pointer.To(entity.UtilitiesMeters)
	in.Restrictions = []entity.Restriction{entity.RestrictionNoPets, entity.RestrictionNoSmoking}
	in.Deposit = pointer.To(45000)

	created, err := repo.Create(t.Context(), in)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if created.Status != entity.PointPending {
		t.Errorf("status = %q, want %q", created.Status, entity.PointPending)
	}

	got, err := repo.ByID(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if got.Type != entity.PointFlat || got.Source != entity.SourceUserLink ||
		got.CreatedBy == nil || *got.CreatedBy != user || got.ZoneID != testZone ||
		got.Stage != nil || got.PhotoKeys[0] != "points/flat.jpg" {
		t.Errorf("point lost its identity: %+v", got)
	}
	if got.PriceTotal == nil || *got.PriceTotal != 90000 || got.Rooms == nil || *got.Rooms != 2 ||
		got.Isolated == nil || !*got.Isolated || got.Deposit == nil || *got.Deposit != 45000 ||
		got.URLNorm == nil || *got.URLNorm != "https://avito.ru/1" {
		t.Errorf("fields lost in round-trip: %+v", got)
	}

	if got.Bathrooms == nil || *got.Bathrooms != entity.BathroomsSeparate ||
		got.Utilities == nil || *got.Utilities != entity.UtilitiesMeters ||
		len(got.Restrictions) != 2 || got.Restrictions[0] != entity.RestrictionNoPets ||
		got.Restrictions[1] != entity.RestrictionNoSmoking {
		t.Errorf("dictionary fields lost in round-trip: %+v", got)
	}

	if got.LastSeenAlive.IsZero() || got.GoneCount != 0 {
		t.Errorf("freshness fields = %+v, want LastSeenAlive set and GoneCount 0", got)
	}

	if _, err := repo.ByID(t.Context(), uuid.New()); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("ByID() of a missing point = %v, want ErrNotFound", err)
	}
}

func TestCreateEnforcesUserLinkLimitUnderConcurrency(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1)

	const attempts = entity.MaxUserLinkPoints + 1
	errs := make([]error, attempts)

	conns := make([]*pgxpool.Conn, attempts)
	for i := range conns {
		c, err := pool.Acquire(t.Context())
		if err != nil {
			t.Fatalf("warm up pool: %v", err)
		}
		conns[i] = c
	}
	for _, c := range conns {
		c.Release()
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			url := fmt.Sprintf("https://avito.ru/%d", i)
			_, errs[i] = repo.Create(t.Context(), flatPoint(&user, entity.SourceUserLink, url, entity.PointPending))
		}()
	}
	close(start)
	wg.Wait()

	var ok, limited int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, entity.ErrPointLimit):
			limited++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != entity.MaxUserLinkPoints || limited != attempts-entity.MaxUserLinkPoints {
		t.Errorf("ok=%d limited=%d, want %d/%d", ok, limited,
			entity.MaxUserLinkPoints, attempts-entity.MaxUserLinkPoints)
	}

	live := countRows(t, pool, `
		SELECT count(*) FROM points
		WHERE created_by = $1 AND status IN ('pending', 'active')`, user)
	if live != entity.MaxUserLinkPoints {
		t.Errorf("database holds %d live points, want %d", live, entity.MaxUserLinkPoints)
	}
}

func TestIntentHomeLimitsByStage(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1)

	live, err := repo.Create(t.Context(), intentHome(user, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(live) failed: %v", err)
	}

	if _, err := repo.Create(t.Context(), intentHome(user, entity.StageLive)); !errors.Is(err, entity.ErrPointLimit) {
		t.Errorf("second live = %v, want ErrPointLimit", err)
	}
	if _, err := repo.Create(t.Context(), intentHome(user, entity.StageMovingIn)); err != nil {
		t.Fatalf("Create(moving_in) alongside live failed: %v", err)
	}

	for _, stage := range []entity.PointStage{entity.StageLive, entity.StageMovingIn} {
		if _, err := repo.Create(t.Context(), intentHome(user, stage)); !errors.Is(err, entity.ErrPointLimit) {
			t.Errorf("third point (%s) = %v, want ErrPointLimit", stage, err)
		}
	}

	for i := range entity.MaxUserLinkPoints {
		url := fmt.Sprintf("https://avito.ru/%d", i)
		if _, err := repo.Create(t.Context(), flatPoint(&user, entity.SourceUserLink, url, entity.PointPending)); err != nil {
			t.Errorf("flat #%d alongside two intent_home points failed: %v", i+1, err)
		}
	}

	closePoint(t, pool, live.ID)
	if _, err := repo.Create(t.Context(), intentHome(user, entity.StageLive)); err != nil {
		t.Errorf("Create(live) after closing the previous one failed: %v", err)
	}
	if n := countRows(t, pool, `
		SELECT count(*) FROM points
		WHERE created_by = $1 AND type = 'intent_home' AND status IN ('pending', 'active')`, user); n != 2 {
		t.Errorf("database holds %d live intent_home points, want 2", n)
	}
}

func TestSourcesWithoutCreatorSkipLimits(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	for i := range entity.MaxUserLinkPoints + 3 {
		source := entity.SourceTGChannel
		if i%2 == 0 {
			source = entity.SourceManual
		}
		created, err := repo.Create(t.Context(),
			flatPoint(nil, source, fmt.Sprintf("https://cian.ru/%d", i), entity.PointActive))
		if err != nil {
			t.Fatalf("Create() #%d failed: %v", i+1, err)
		}
		if created.CreatedBy != nil || created.Status != entity.PointActive {
			t.Errorf("unexpected point: %+v", created)
		}
	}
}

func TestByURLNormIgnoresClosed(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1)

	const url = "https://avito.ru/1"
	created, err := repo.Create(t.Context(), flatPoint(&user, entity.SourceUserLink, url, entity.PointActive))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	found, err := repo.ByURLNorm(t.Context(), url)
	if err != nil {
		t.Fatalf("ByURLNorm() failed: %v", err)
	}
	if found == nil || found.ID != created.ID {
		t.Fatalf("ByURLNorm() = %+v, want the created point", found)
	}
	if missing, err := repo.ByURLNorm(t.Context(), "https://avito.ru/nothing"); err != nil || missing != nil {
		t.Errorf("ByURLNorm() of an unknown url = %+v, %v, want nil, nil", missing, err)
	}

	closePoint(t, pool, created.ID)
	closed, err := repo.ByURLNorm(t.Context(), url)
	if err != nil {
		t.Fatalf("ByURLNorm() after close failed: %v", err)
	}
	if closed != nil {
		t.Errorf("ByURLNorm() found a closed point: %+v", closed)
	}
	again, err := repo.Create(t.Context(), flatPoint(&user, entity.SourceUserLink, url, entity.PointActive))
	if err != nil {
		t.Fatalf("re-submitting a closed link failed: %v", err)
	}
	found, err = repo.ByURLNorm(t.Context(), url)
	if err != nil {
		t.Fatalf("ByURLNorm() failed: %v", err)
	}
	if found == nil || found.ID != again.ID {
		t.Errorf("ByURLNorm() = %+v, want the re-submitted point", found)
	}

	if _, err := repo.Create(t.Context(), flatPoint(&user, entity.SourceUserLink, url, entity.PointActive)); !errors.Is(err, ErrURLTaken) {
		t.Errorf("duplicate of a live link = %v, want ErrURLTaken", err)
	}
}

func TestCreateEntersTheCreatorIntoOwnPool(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)

	flat, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/1", entity.PointPending))
	if err != nil {
		t.Fatalf("Create(flat) failed: %v", err)
	}
	if in, err := repo.InPool(t.Context(), author, flat.ID); err != nil || !in {
		t.Errorf("InPool() of the creator = %v, %v, want true, nil", in, err)
	}

	home, err := repo.Create(t.Context(), intentHome(author, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}
	if in, err := repo.InPool(t.Context(), author, home.ID); err != nil || in {
		t.Errorf("InPool() of the «к себе» author = %v, %v, want false, nil", in, err)
	}

	market, err := repo.Create(t.Context(),
		flatPoint(nil, entity.SourceManual, "https://cian.ru/1", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(manual flat) failed: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM likes_points WHERE point_id = $1`, market.ID); n != 0 {
		t.Errorf("pool of a point without a creator holds %d rows, want 0", n)
	}
}

func TestPoolMembershipIsPerPoint(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)
	guest := seedUser(t, pool, 2)

	first, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/1", entity.PointActive))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	second, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/2", entity.PointActive))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	inPool := func(pointID uuid.UUID) bool {
		t.Helper()
		in, err := repo.InPool(t.Context(), guest, pointID)
		if err != nil {
			t.Fatalf("InPool() failed: %v", err)
		}
		return in
	}

	if _, err := repo.Enter(t.Context(), guest, first.ID); err != nil {
		t.Fatalf("Enter() failed: %v", err)
	}
	if !inPool(first.ID) || inPool(second.ID) {
		t.Errorf("after entering the first point: in first = %v, in second = %v, want true, false",
			inPool(first.ID), inPool(second.ID))
	}

	if _, err := repo.Enter(t.Context(), guest, second.ID); err != nil {
		t.Fatalf("Enter() failed: %v", err)
	}
	if err := repo.Leave(t.Context(), guest, second.ID); err != nil {
		t.Fatalf("Leave() failed: %v", err)
	}
	if !inPool(first.ID) || inPool(second.ID) {
		t.Errorf("after leaving the second point: in first = %v, in second = %v, want true, false",
			inPool(first.ID), inPool(second.ID))
	}
}

func TestEnterOnlyActiveFlat(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)
	guest := seedUser(t, pool, 2)

	home, err := repo.Create(t.Context(), intentHome(author, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}
	pending, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/1", entity.PointPending))
	if err != nil {
		t.Fatalf("Create(pending flat) failed: %v", err)
	}

	refused := []struct {
		name  string
		point uuid.UUID
	}{
		{"intent_home", home.ID},
		{"pending flat", pending.ID},
		{"missing point", uuid.New()},
	}
	for _, tt := range refused {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := repo.Enter(t.Context(), guest, tt.point); !errors.Is(err, entity.ErrNotFound) {
				t.Errorf("Enter() = %v, want ErrNotFound", err)
			}
			if in, err := repo.InPool(t.Context(), guest, tt.point); err != nil || in {
				t.Errorf("InPool() = %v, %v, want false, nil", in, err)
			}
		})
	}
	if n := countRows(t, pool, `SELECT count(*) FROM likes_points WHERE user_id = $1`, guest); n != 0 {
		t.Errorf("database holds %d pool rows after refusals, want 0", n)
	}

	flat, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/2", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(active flat) failed: %v", err)
	}

	for i := range 2 {
		entered, err := repo.Enter(t.Context(), guest, flat.ID)
		if err != nil {
			t.Fatalf("Enter() failed: %v", err)
		}
		if want := i == 0; entered != want {
			t.Errorf("Enter() #%d entered = %v, want %v", i+1, entered, want)
		}
	}
	if in, err := repo.InPool(t.Context(), guest, flat.ID); err != nil || !in {
		t.Errorf("InPool() = %v, %v, want true, nil", in, err)
	}
	if n := countRows(t, pool,
		`SELECT count(*) FROM likes_points WHERE user_id = $1 AND point_id = $2`, guest, flat.ID); n != 1 {
		t.Errorf("database holds %d pool rows, want 1", n)
	}

	for range 2 {
		if err := repo.Leave(t.Context(), guest, flat.ID); err != nil {
			t.Fatalf("Leave() failed: %v", err)
		}
	}
	if in, err := repo.InPool(t.Context(), guest, flat.ID); err != nil || in {
		t.Errorf("InPool() after leaving = %v, %v, want false, nil", in, err)
	}
}

func TestCloseBelongsToTheCreator(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)
	stranger := seedUser(t, pool, 2)

	pending, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/1", entity.PointPending))
	if err != nil {
		t.Fatalf("Create(pending) failed: %v", err)
	}
	active, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/2", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(active) failed: %v", err)
	}
	home, err := repo.Create(t.Context(), intentHome(author, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}

	for _, id := range []uuid.UUID{pending.ID, active.ID, home.ID, uuid.New()} {
		if err := repo.Close(t.Context(), stranger, id); !errors.Is(err, entity.ErrNotFound) {
			t.Errorf("Close() by a stranger = %v, want ErrNotFound", err)
		}
	}
	if n := countRows(t, pool,
		`SELECT count(*) FROM points WHERE created_by = $1 AND status = 'closed'`, author); n != 0 {
		t.Fatalf("a stranger closed %d points", n)
	}

	for _, id := range []uuid.UUID{pending.ID, active.ID, home.ID} {
		if err := repo.Close(t.Context(), author, id); err != nil {
			t.Fatalf("Close() failed: %v", err)
		}
		got, err := repo.ByID(t.Context(), id)
		if err != nil {
			t.Fatalf("ByID() failed: %v", err)
		}

		if got.Status != entity.PointClosed || got.CloseReason == nil || *got.CloseReason != "closed" {
			t.Errorf("point %s = %s / %v, want closed with a reason", id, got.Status, got.CloseReason)
		}
	}

	if err := repo.Close(t.Context(), author, active.ID); err != nil {
		t.Errorf("second Close() = %v, want nil — идемпотентно", err)
	}

	for i := range entity.MaxUserLinkPoints {
		url := fmt.Sprintf("https://avito.ru/after-%d", i)
		if _, err := repo.Create(t.Context(), flatPoint(&author, entity.SourceUserLink, url, entity.PointPending)); err != nil {
			t.Fatalf("Create() after closing #%d failed: %v", i+1, err)
		}
	}
	if _, err := repo.Create(t.Context(), intentHome(author, entity.StageLive)); err != nil {
		t.Errorf("Create(intent_home) after closing failed: %v", err)
	}
}
