package likesrepo

import (
	"context"
	"errors"
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

// seedPoint writes the point directly: the like reads its type, status and
// author, and the interesting cases — pending, paused, closed — are states
// pointsrepo.Create does not hand out in one call.
func seedPoint(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID, kind, status string) uuid.UUID {
	t.Helper()
	var stage, reason *string
	if kind == "intent_home" {
		stage = pointer.To("live")
	}
	if status == "closed" {
		reason = pointer.To("rejected")
	}

	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO points (type, stage, status, close_reason, source, created_by,
			zone_id, price_total, photo_key)
		VALUES ($1, $2, $3, $4, 'user_link', $5, $6, 90000, 'points/test.jpg')
		RETURNING id`, kind, stage, status, reason, owner, testZone).Scan(&id)
	if err != nil {
		t.Fatalf("seed %s point: %v", kind, err)
	}
	return id
}

func enterPool(t *testing.T, pool *pgxpool.Pool, userID, pointID uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO likes_points (user_id, point_id) VALUES ($1, $2)`, userID, pointID)
	if err != nil {
		t.Fatalf("enter the pool of %s: %v", pointID, err)
	}
}

func pointCtx(pointID uuid.UUID) LikeContext {
	return LikeContext{PointID: &pointID}
}

// Пул квартиры несёт отношение сам: вошедшему лайк открыт БЕЗ единого поиска
// (§2.3), не вошедшему точка отвечает тем же 404, что и невидимый человек.
func TestFlatPointLikeNeedsPoolMembership(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	target := seedUser(t, pool, 2, "active", "male", "any")
	owner := seedUser(t, pool, 3, "active", "male", "any")
	flat := seedPoint(t, pool, owner, "flat", "active")

	if _, err := repo.Create(t.Context(), liker, target, pointCtx(flat)); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Create() from outside the pool = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM likes_users WHERE from_id = $1`, liker); n != 0 {
		t.Errorf("database holds %d likes after the refusal, want 0", n)
	}

	enterPool(t, pool, liker, flat)
	res, err := repo.Create(t.Context(), liker, target, pointCtx(flat))
	if err != nil {
		t.Fatalf("Create() from the pool failed: %v", err)
	}
	// Ни одного поиска у лайкающего нет — в зонной ветке это был бы ErrNeedSearch.
	if !res.Like.Spent || res.Like.PointID == nil || *res.Like.PointID != flat {
		t.Errorf("like = %+v, want a spent one with the point context", res.Like)
	}
	if res.Like.ZoneID != "" {
		t.Errorf("point like carries zone %q, want none", res.Like.ZoneID)
	}
	if res.Budget.Remaining != entity.MaxDailyLikes-1 {
		t.Errorf("remaining = %d, want %d", res.Budget.Remaining, entity.MaxDailyLikes-1)
	}
}

// «Хочу эту комнату» уходит автору карточки и больше никому: чужой человек с
// тем же point-контекстом — не наш кейс (§2.2, пула у «к себе» нет).
func TestIntentHomeLikeReachesOnlyTheAuthor(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	author := seedUser(t, pool, 2, "active", "male", "any")
	stranger := seedUser(t, pool, 3, "active", "male", "any")
	room := seedPoint(t, pool, author, "intent_home", "active")

	if _, err := repo.Create(t.Context(), liker, stranger, pointCtx(room)); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Create() to a non-author = %v, want ErrNotFound", err)
	}

	res, err := repo.Create(t.Context(), liker, author, pointCtx(room))
	if err != nil {
		t.Fatalf("Create() to the author failed: %v", err)
	}
	if !res.Like.Spent || res.Like.PointID == nil || *res.Like.PointID != room {
		t.Errorf("like = %+v, want a spent one on the room", res.Like)
	}
}

// Точка, которой человек видеть не мог, — не контекст: карточка на проверке,
// на паузе или закрытая отвечает 404, как и несуществующая.
func TestPointLikeNeedsALivePoint(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	target := seedUser(t, pool, 2, "active", "male", "any")
	owner := seedUser(t, pool, 3, "active", "male", "any")

	for _, status := range []string{"pending", "paused", "closed"} {
		t.Run(status, func(t *testing.T) {
			flat := seedPoint(t, pool, owner, "flat", status)
			// Членство в пуле есть — отказ обязан прийти именно от статуса.
			enterPool(t, pool, liker, flat)

			_, err := repo.Create(t.Context(), liker, target, pointCtx(flat))
			if !errors.Is(err, entity.ErrNotFound) {
				t.Errorf("Create() on a %s point = %v, want ErrNotFound", status, err)
			}
		})
	}

	if _, err := repo.Create(t.Context(), liker, target, pointCtx(uuid.New())); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Create() on an unknown point = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM likes_users WHERE from_id = $1`, liker); n != 0 {
		t.Errorf("database holds %d likes after the refusals, want 0", n)
	}
}

// Автор «к себе» может не искать ничего: отклик ему прилетел, и ответ на него
// не стоит ни слота, ни поиска (§2.3). Контекст мэтча — точка первого лайка.
func TestReplyToAPointLikeIsFreeAndKeepsTheContext(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	responder := seedUser(t, pool, 1, "active", "male", "any")
	author := seedUser(t, pool, 2, "active", "male", "any")
	room := seedPoint(t, pool, author, "intent_home", "active")

	if _, err := repo.Create(t.Context(), responder, author, pointCtx(room)); err != nil {
		t.Fatalf("Create() of the response failed: %v", err)
	}
	res, err := repo.Create(t.Context(), author, responder, LikeContext{})
	if err != nil {
		t.Fatalf("reply Create() failed: %v", err)
	}
	if res.Like.Spent || res.Budget.Remaining != entity.MaxDailyLikes {
		t.Errorf("reply cost the author a slot: %+v, %+v", res.Like, res.Budget)
	}
	if res.Match == nil {
		t.Fatal("the mutual like produced no match")
	}
	if res.Match.ContextPoint == nil || *res.Match.ContextPoint != room {
		t.Errorf("match point context = %v, want %s", res.Match.ContextPoint, room)
	}
	if res.Match.ContextZone != "" {
		t.Errorf("match zone = %q, want none — контексты взаимоисключимы", res.Match.ContextZone)
	}
}

// Слотов пять на человека, а не по пять на каждый вид контекста: окно считает
// потраченные лайки, не спрашивая, откуда они (§2.3).
func TestBudgetIsSharedAcrossContexts(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	seedSearch(t, pool, liker, "active")
	owner := seedUser(t, pool, 2, "active", "male", "any")
	flat := seedPoint(t, pool, owner, "flat", "active")
	enterPool(t, pool, liker, flat)

	targets := make([]uuid.UUID, entity.MaxDailyLikes+1)
	for i := range targets {
		targets[i] = seedUser(t, pool, int64(10+i), "active", "male", "any")
	}
	for i, target := range targets[:3] {
		if _, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone)); err != nil {
			t.Fatalf("zone Create() #%d failed: %v", i+1, err)
		}
	}
	for i, target := range targets[3:entity.MaxDailyLikes] {
		if _, err := repo.Create(t.Context(), liker, target, pointCtx(flat)); err != nil {
			t.Fatalf("point Create() #%d failed: %v", i+1, err)
		}
	}

	last := targets[entity.MaxDailyLikes]
	if _, err := repo.Create(t.Context(), liker, last, pointCtx(flat)); !errors.Is(err, entity.ErrLikeLimit) {
		t.Errorf("point Create() over the shared limit = %v, want ErrLikeLimit", err)
	}
	if _, err := repo.Create(t.Context(), liker, last, zoneCtx(testZone)); !errors.Is(err, entity.ErrLikeLimit) {
		t.Errorf("zone Create() over the shared limit = %v, want ErrLikeLimit", err)
	}
	if n := countRows(t, pool,
		`SELECT count(*) FROM likes_users WHERE from_id = $1 AND spent`, liker); n != entity.MaxDailyLikes {
		t.Errorf("database holds %d spent likes, want %d", n, entity.MaxDailyLikes)
	}
}
