package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/repo/notificationsrepo"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tgbot"
)

const (
	pendingLimit = 200

	maxAttempts = 5

	scanEvery = time.Hour
)

const (
	buttonPool     = "Открыть пул"
	buttonIncoming = "Посмотреть"
	buttonOpen     = "Открыть"
	buttonFeed     = "В ленту"
	buttonPhoto    = "Загрузить"
	buttonAbout    = "Поправить"
	buttonCheck    = "Проверить"
)

const (
	linkIncoming = "incoming"
	linkFeed     = "feed"
	linkPhoto    = "photo"
	linkAbout    = "about"
	linkMine     = "mine"

	linkPoolPrefix  = "pool_"
	linkPointPrefix = "point_"
	linkMatchPrefix = "match_"
)

var errUnknownContext = errors.New("the push context is gone")

type Queue interface {
	Pending(ctx context.Context, limit int) ([]entity.Notification, error)
	PendingPersonal(ctx context.Context, limit int) ([]entity.Notification, error)
	Insert(ctx context.Context, n entity.Notification) (bool, error)
	MarkSent(ctx context.Context, ids []uuid.UUID, at time.Time) error
	MarkFailed(ctx context.Context, ids []uuid.UUID) error
	Bump(ctx context.Context, id uuid.UUID) error
	LastSentAt(ctx context.Context, userID uuid.UUID, typ entity.NotificationType, contextKey string) (*time.Time, error)
	SentPersonalCount(ctx context.Context, userID uuid.UUID, since time.Time) (int, error)
	FailAllPending(ctx context.Context, userID uuid.UUID) error
	LastLikedTail(ctx context.Context, userID uuid.UUID, since time.Time) (*notificationsrepo.TailMessage, error)
	SetTail(ctx context.Context, id uuid.UUID, messageID, count int) error
}

type Sender interface {
	SendPush(ctx context.Context, chatID int64, text string, buttons []tgbot.PushButton) (int, error)
	EditPushText(ctx context.Context, chatID int64, messageID int, text string, buttons []tgbot.PushButton) error
}

type Recipients interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.User, error)
	MarkPushBlocked(ctx context.Context, id uuid.UUID) error
}

type SearchTTL interface {
	StaleForTTL(ctx context.Context, quiet, afterMatch time.Duration) ([]entity.Search, error)
	MarkTTLAsked(ctx context.Context, id uuid.UUID) error
	PauseUnanswered(ctx context.Context, olderThan time.Duration) ([]uuid.UUID, error)
}

type MatchPolls interface {
	MatchesForPostPoll(ctx context.Context, olderThan time.Duration) ([]matchesrepo.PollTarget, error)
}

type PointZones interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.Point, error)
	PointsForPing(ctx context.Context, every time.Duration) ([]entity.Point, error)
	MarkPingAsked(ctx context.Context, id uuid.UUID) error
	Expire(ctx context.Context, quiet, afterPing time.Duration) ([]uuid.UUID, error)
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Worker struct {
	queue    Queue
	sender   Sender
	users    Recipients
	searches SearchTTL
	matches  MatchPolls
	points   PointZones
	dir      ZoneDir
	log      *slog.Logger

	tick time.Duration

	now func() time.Time

	lastScan time.Time

	answersReachUs bool
}

func NewWorker(queue Queue, sender Sender, users Recipients, searches SearchTTL,
	matches MatchPolls, points PointZones, dir ZoneDir, tick time.Duration, log *slog.Logger) *Worker {
	return &Worker{
		queue: queue, sender: sender, users: users, searches: searches,
		matches: matches, points: points, dir: dir, log: log,
		tick: tick, now: time.Now, answersReachUs: true,
	}
}

func (w *Worker) WithoutAnswers() *Worker {
	w.answersReachUs = false
	return w
}

func (w *Worker) Run(ctx context.Context) {
	w.log.Info("push worker started", "tick", w.tick)
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			w.log.Info("push worker stopped")
			return
		}
		w.pass(ctx)

		select {
		case <-ctx.Done():
			w.log.Info("push worker stopped")
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) pass(ctx context.Context) {
	unreachable := map[uuid.UUID]bool{}

	personal, err := w.queue.PendingPersonal(ctx, pendingLimit)
	if err != nil {
		w.log.Error("read the personal push queue", "error", err)
	} else {
		w.deliverPersonal(ctx, personal, unreachable)
	}

	pending, err := w.queue.Pending(ctx, pendingLimit)
	if err != nil {
		w.log.Error("read the push queue", "error", err)
	} else {
		var aggregated []entity.Notification
		for _, row := range pending {
			if row.Type.Aggregated() {
				aggregated = append(aggregated, row)
			}
		}
		w.deliverAggregates(ctx, aggregated, unreachable)
	}

	if !w.answersReachUs || w.now().Sub(w.lastScan) < scanEvery {
		return
	}
	w.lastScan = w.now()
	w.askTTL(ctx)
	w.pauseSilent(ctx)
	w.askPostMatch(ctx)
	w.pingPoints(ctx)
	w.expirePoints(ctx)
}

func (w *Worker) deliverPersonal(ctx context.Context, rows []entity.Notification, unreachable map[uuid.UUID]bool) {
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		w.deliverOne(ctx, row, unreachable)
	}
}

func (w *Worker) deliverOne(ctx context.Context, row entity.Notification, unreachable map[uuid.UUID]bool) {
	if unreachable[row.UserID] {
		return
	}
	chatID, ok := w.chatOf(ctx, row.UserID)
	if !ok {
		return
	}

	text, buttons, err := w.personalMessage(ctx, row)
	if err != nil {
		if !errors.Is(err, errUnknownContext) {
			w.log.Warn("build a personal push", "type", row.Type, "notification", row.ID, "error", err)
			return
		}
		w.giveUp(ctx, row, err)
		return
	}

	if w.folded(ctx, row, chatID, unreachable) {
		return
	}

	msgID, err := w.sender.SendPush(ctx, chatID, text, buttons)
	if err != nil {
		if w.sendFailed(ctx, row.UserID, []entity.Notification{row}, err) {
			unreachable[row.UserID] = true
		}
		return
	}
	w.markSent(ctx, []uuid.UUID{row.ID})

	if row.Type == entity.NotifLiked {
		if err := w.queue.SetTail(ctx, row.ID, msgID, 1); err != nil {
			w.log.Warn("remember the tail carrier", "notification", row.ID, "error", err)
		}
	}
}

func (w *Worker) folded(ctx context.Context, row entity.Notification, chatID int64, unreachable map[uuid.UUID]bool) bool {
	if !slices.Contains(entity.PersonalPushTypes(), row.Type) {
		return false
	}
	since := w.now().Add(-entity.AggregateWindow)
	sent, err := w.queue.SentPersonalCount(ctx, row.UserID, since)
	if err != nil {
		w.log.Warn("count today's personal pushes", "user", row.UserID, "error", err)
		return false
	}
	if sent < entity.MaxPersonalPushesPerDay {
		return false
	}
	if row.Type != entity.NotifLiked {
		w.log.Warn("over the daily personal push limit, but this type does not fold",
			"type", row.Type, "user", row.UserID, "sent", sent)
		return false
	}

	tail, err := w.queue.LastLikedTail(ctx, row.UserID, since)
	if err != nil {
		w.log.Warn("find the tail carrier", "user", row.UserID, "error", err)
		return false
	}
	if tail == nil {
		return false
	}

	count := tail.Count + 1
	if err := w.sender.EditPushText(ctx, chatID, tail.MessageID, LikedTailText(count), incomingButton()); err != nil {
		if uneditable(err) {
			w.log.Info("tail carrier is gone, sending the like as a fresh push", "user", row.UserID)
			return false
		}
		if w.sendFailed(ctx, row.UserID, []entity.Notification{row}, err) {
			unreachable[row.UserID] = true
		}
		return true
	}
	w.markSent(ctx, []uuid.UUID{row.ID})
	if err := w.queue.SetTail(ctx, tail.ID, tail.MessageID, count); err != nil {
		w.log.Warn("grow the tail", "notification", tail.ID, "error", err)
	}
	return true
}

func (w *Worker) deliverAggregates(ctx context.Context, rows []entity.Notification, unreachable map[uuid.UUID]bool) {
	type key struct {
		user       uuid.UUID
		typ        entity.NotificationType
		contextKey string
	}

	groups := map[key][]entity.Notification{}
	var order []key
	for _, row := range rows {
		k := key{row.UserID, row.Type, row.ContextKey}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], row)
	}

	for _, k := range order {
		if ctx.Err() != nil {
			return
		}
		if unreachable[k.user] {
			continue
		}
		w.deliverGroup(ctx, k.user, k.typ, k.contextKey, groups[k], unreachable)
	}
}

func (w *Worker) deliverGroup(ctx context.Context, userID uuid.UUID, typ entity.NotificationType,
	contextKey string, rows []entity.Notification, unreachable map[uuid.UUID]bool) {
	last, err := w.queue.LastSentAt(ctx, userID, typ, contextKey)
	if err != nil {
		w.log.Error("check the aggregate window", "user", userID, "type", typ, "error", err)
		return
	}

	if last != nil && w.now().Sub(*last) < entity.AggregateWindow {
		return
	}

	chatID, ok := w.chatOf(ctx, userID)
	if !ok {
		return
	}

	text, buttons, err := w.aggregateMessage(ctx, typ, contextKey, len(rows))
	if err != nil {
		if !errors.Is(err, errUnknownContext) {
			w.log.Warn("build an aggregate push", "type", typ, "context", contextKey, "error", err)
			return
		}
		w.log.Warn("the context of an aggregate push is gone, giving it up",
			"type", typ, "context", contextKey, "error", err)
		w.fail(ctx, ids(rows))
		return
	}

	if _, err := w.sender.SendPush(ctx, chatID, text, buttons); err != nil {
		if w.sendFailed(ctx, userID, rows, err) {
			unreachable[userID] = true
		}
		return
	}
	w.markSent(ctx, ids(rows))
}

func (w *Worker) askTTL(ctx context.Context) {
	stale, err := w.searches.StaleForTTL(ctx,
		entity.TTLQuietDays*24*time.Hour, entity.TTLAfterMatchDays*24*time.Hour)
	if err != nil {
		w.log.Error("find the searches to ask about", "error", err)
		return
	}

	for _, search := range stale {
		inserted, err := w.queue.Insert(ctx, entity.Notification{
			UserID:     search.UserID,
			Type:       entity.NotifStillSearching,
			ActorKey:   ttlActor(search),
			ContextKey: search.ZoneID,
			Payload:    map[string]any{payloadSearchID: search.ID.String()},
		})
		if err != nil {
			w.log.Warn("write the ttl question", "search", search.ID, "error", err)
			continue
		}

		if err := w.searches.MarkTTLAsked(ctx, search.ID); err != nil {
			w.log.Warn("mark the search asked", "search", search.ID, "error", err)
			continue
		}
		w.log.Debug("asked a search whether it is still alive", "search", search.ID, "new", inserted)
	}
}

func (w *Worker) pauseSilent(ctx context.Context) {
	paused, err := w.searches.PauseUnanswered(ctx, entity.TTLAnswerDays*24*time.Hour)
	if err != nil {
		w.log.Error("pause the unanswered searches", "error", err)
		return
	}
	if len(paused) > 0 {
		w.log.Info("searches paused after silence", "count", len(paused))
	}
}

func (w *Worker) askPostMatch(ctx context.Context) {
	targets, err := w.matches.MatchesForPostPoll(ctx, entity.PostMatchDays*24*time.Hour)
	if err != nil {
		w.log.Error("find the matches to ask about", "error", err)
		return
	}

	for _, target := range targets {
		_, err := w.queue.Insert(ctx, entity.Notification{
			UserID:   target.UserID,
			Type:     entity.NotifPostMatch,
			ActorKey: target.Match.ID.String(),
			Payload:  map[string]any{payloadPeerName: target.PeerName},
		})
		if err != nil {
			w.log.Warn("write the post-match question", "match", target.Match.ID, "error", err)
		}
	}
}

func (w *Worker) pingPoints(ctx context.Context) {
	due, err := w.points.PointsForPing(ctx, entity.PingEveryDays*24*time.Hour)
	if err != nil {
		w.log.Error("find the points to ping", "error", err)
		return
	}

	for _, point := range due {
		if point.CreatedBy == nil {
			continue
		}
		if _, err := w.queue.Insert(ctx, entity.Notification{
			UserID:     *point.CreatedBy,
			Type:       entity.NotifPointPing,
			ActorKey:   pingActor(point),
			ContextKey: point.ID.String(),
		}); err != nil {
			w.log.Warn("write the point ping", "point", point.ID, "error", err)
			continue
		}

		if err := w.points.MarkPingAsked(ctx, point.ID); err != nil {
			w.log.Warn("mark the point pinged", "point", point.ID, "error", err)
		}
	}
}

func (w *Worker) expirePoints(ctx context.Context) {
	ids, err := w.points.Expire(ctx, entity.PointTTLDays*24*time.Hour, entity.PingAnswerDays*24*time.Hour)
	if err != nil {
		w.log.Error("expire stale points", "error", err)
		return
	}
	if len(ids) > 0 {
		w.log.Info("points expired", "count", len(ids))
	}
}

func (w *Worker) personalMessage(ctx context.Context, n entity.Notification) (string, []tgbot.PushButton, error) {
	switch n.Type {
	case entity.NotifLiked:
		name := payloadString(n.Payload, payloadName)
		gender := entity.Gender(payloadString(n.Payload, payloadGender))
		return LikedText(name, gender), incomingButton(), nil

	case entity.NotifMatched:
		return MatchedText(), deepLink(buttonOpen, linkMatchPrefix+n.ActorKey), nil

	case entity.NotifProfileOK:
		return VerdictText(n.Type), deepLink(buttonFeed, linkFeed), nil
	case entity.NotifPhotoRejected:
		return VerdictText(n.Type), deepLink(buttonPhoto, linkPhoto), nil
	case entity.NotifAboutRejected:
		return VerdictText(n.Type), deepLink(buttonAbout, linkAbout), nil
	case entity.NotifPointOK, entity.NotifPointRejected:
		return VerdictText(n.Type), deepLink(buttonOpen, linkMine), nil

	case entity.NotifStillSearching:
		zone, ok := w.dir.Get(n.ContextKey)
		if !ok {
			return "", nil, fmt.Errorf("zone %q: %w", n.ContextKey, errUnknownContext)
		}
		raw := payloadString(n.Payload, payloadSearchID)
		searchID, err := uuid.Parse(raw)
		if err != nil {
			return "", nil, fmt.Errorf("search %q: %w", raw, errUnknownContext)
		}
		return StillSearchingText(zone.NameIn), stillSearchingButtons(searchID), nil

	case entity.NotifPointGone:
		_, zone, err := w.pointContext(ctx, n.ContextKey)
		if err != nil {
			return "", nil, err
		}
		if payloadBool(n.Payload, payloadOwn) {
			return PointGoneOwnText(zone.NameIn), deepLink(buttonCheck, linkPointPrefix+n.ContextKey), nil
		}
		return PointGoneText(zone.NameIn), deepLink(buttonOpen, linkPointPrefix+n.ContextKey), nil

	case entity.NotifPointPing:
		point, zone, err := w.pointContext(ctx, n.ContextKey)
		if err != nil {
			return "", nil, err
		}
		return PointPingText(point.Type, zone.NameIn), pointPingButtons(point.Type, point.ID), nil

	case entity.NotifPostMatch:
		matchID, err := uuid.Parse(n.ActorKey)
		if err != nil {
			return "", nil, fmt.Errorf("match %q: %w", n.ActorKey, errUnknownContext)
		}
		return PostMatchText(payloadString(n.Payload, payloadPeerName)), postMatchButtons(matchID), nil

	default:
		return "", nil, fmt.Errorf("type %q: %w", n.Type, errUnknownContext)
	}
}

func (w *Worker) aggregateMessage(ctx context.Context, typ entity.NotificationType,
	contextKey string, n int) (string, []tgbot.PushButton, error) {
	switch typ {
	case entity.NotifZoneNew:
		zone, ok := w.dir.Get(contextKey)
		if !ok {
			return "", nil, fmt.Errorf("zone %q: %w", contextKey, errUnknownContext)
		}
		return ZoneNewText(zone.NameIn, n), deepLink(buttonPool, linkPoolPrefix+contextKey), nil

	case entity.NotifPoolNew:
		_, zone, err := w.pointContext(ctx, contextKey)
		if err != nil {
			return "", nil, err
		}
		return PoolNewText(zone.Name, n), deepLink(buttonPool, linkPointPrefix+contextKey), nil

	default:
		return "", nil, fmt.Errorf("type %q: %w", typ, errUnknownContext)
	}
}

func (w *Worker) pointContext(ctx context.Context, contextKey string) (entity.Point, zones.Zone, error) {
	pointID, err := uuid.Parse(contextKey)
	if err != nil {
		return entity.Point{}, zones.Zone{}, fmt.Errorf("point %q: %w", contextKey, errUnknownContext)
	}
	point, err := w.points.ByID(ctx, pointID)
	if err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			return entity.Point{}, zones.Zone{}, fmt.Errorf("point %s: %w", pointID, errUnknownContext)
		}
		return entity.Point{}, zones.Zone{}, err
	}
	zone, ok := w.dir.Get(point.ZoneID)
	if !ok {
		return entity.Point{}, zones.Zone{}, fmt.Errorf("zone %q: %w", point.ZoneID, errUnknownContext)
	}
	return point, zone, nil
}

func (w *Worker) chatOf(ctx context.Context, userID uuid.UUID) (int64, bool) {
	user, err := w.users.ByID(ctx, userID)
	if err != nil {
		w.log.Warn("find the addressee of a push", "user", userID, "error", err)
		return 0, false
	}
	return user.TelegramID, true
}

func (w *Worker) sendFailed(ctx context.Context, userID uuid.UUID, rows []entity.Notification, sendErr error) bool {
	if errors.Is(sendErr, tgbot.ErrBlocked) {
		w.log.Info("the bot cannot reach this user, dropping their queue", "user", userID, "error", sendErr)
		if err := w.users.MarkPushBlocked(ctx, userID); err != nil {
			w.log.Error("mark the pushes blocked", "user", userID, "error", err)
		}
		if err := w.queue.FailAllPending(ctx, userID); err != nil {
			w.log.Error("drop the queue of a blocked user", "user", userID, "error", err)
		}
		return true
	}

	if transient(sendErr) {
		w.log.Warn("could not deliver a push, transient — will retry without counting", "user", userID, "rows", len(rows), "error", sendErr)
		return false
	}
	w.log.Warn("could not deliver a push, will retry", "user", userID, "rows", len(rows), "error", sendErr)
	var dead []uuid.UUID
	for _, row := range rows {
		if err := w.queue.Bump(ctx, row.ID); err != nil {
			w.log.Error("count a failed attempt", "notification", row.ID, "error", err)
			continue
		}
		if row.Attempts+1 >= maxAttempts {
			dead = append(dead, row.ID)
		}
	}
	if len(dead) > 0 {
		w.log.Warn("giving up on pushes after too many attempts", "count", len(dead))
		w.fail(ctx, dead)
	}
	return false
}

func uneditable(err error) bool {
	var apiErr *tgbot.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	d := strings.ToLower(apiErr.Description)
	return strings.Contains(d, "message to edit not found") || strings.Contains(d, "message can't be edited")
}

func transient(err error) bool {
	var apiErr *tgbot.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == 429
	}
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded)
}

func (w *Worker) giveUp(ctx context.Context, row entity.Notification, cause error) {
	w.log.Warn("cannot build a push, giving it up", "type", row.Type, "notification", row.ID, "error", cause)
	w.fail(ctx, []uuid.UUID{row.ID})
}

func (w *Worker) markSent(ctx context.Context, ids []uuid.UUID) {
	if err := w.queue.MarkSent(ctx, ids, w.now()); err != nil {
		w.log.Error("mark pushes sent", "count", len(ids), "error", err)
	}
}

func (w *Worker) fail(ctx context.Context, ids []uuid.UUID) {
	if err := w.queue.MarkFailed(ctx, ids); err != nil {
		w.log.Error("mark pushes failed", "count", len(ids), "error", err)
	}
}

func ttlActor(search entity.Search) string {
	return fmt.Sprintf("%s:%d", search.ID, search.LastActiveAt.Unix())
}

func pingActor(point entity.Point) string {
	return fmt.Sprintf("%s:%d", point.ID, point.LastSeenAlive.Unix())
}

func deepLink(text, startParam string) []tgbot.PushButton {
	return []tgbot.PushButton{{Text: text, StartParam: startParam}}
}

func incomingButton() []tgbot.PushButton {
	return deepLink(buttonIncoming, linkIncoming)
}

func ids(rows []entity.Notification) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}

func payloadString(payload map[string]any, key string) string {
	v, _ := payload[key].(string)
	return v
}

func payloadBool(payload map[string]any, key string) bool {
	v, _ := payload[key].(bool)
	return v
}
