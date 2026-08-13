// Package pointsapi serves the points of §2.2: квартира с рынка, под которую
// собирают людей, и «сосед к себе» — витрина с лицом автора. Обе формы, обе
// карточки, дешёвый вход в пул и сам пул.
//
// Гейты живут в сервисе и в транзакциях репозитория: pending-точка своему
// создателю, пул — только вошедшим, лимиты — под локом. Ручка переводит их
// сентинелы в коды §3 и решает, какой микрокопией ответить.
package pointsapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/points"
	"flatmatch/api/internal/service/zones"
)

const maxBodyBytes = 64 << 10

type Points interface {
	CreateFlat(ctx context.Context, userID uuid.UUID, in points.CreateFlat) (entity.Point, error)
	CreateIntentHome(ctx context.Context, userID uuid.UUID, in points.CreateIntentHome) (entity.Point, error)
	Point(ctx context.Context, viewerID, pointID uuid.UUID) (pointsrepo.FeedPoint, error)
	Mine(ctx context.Context, viewerID uuid.UUID) (created, entered []pointsrepo.FeedPoint, err error)
	Enter(ctx context.Context, userID, pointID uuid.UUID) error
	Leave(ctx context.Context, userID, pointID uuid.UUID) error
	Pool(ctx context.Context, viewer entity.User, pointID uuid.UUID) (points.PoolView, error)
}

// Photos turns a stored key into an address. Фото точки отдельного статуса не
// имеет — модератор одобряет точку целиком (§5), — поэтому ключ превращается в
// адрес без проверок; лицо человека проходит правило §5 в personPhoto.
type Photos interface {
	URL(key string) string
}

type ZoneDir interface {
	Get(id string) (zones.Zone, bool)
}

type Handler struct {
	points Points
	photos Photos
	dir    ZoneDir
}

var errNoUserInContext = errors.New("no authenticated user in request context")

func New(points Points, photos Photos, dir ZoneDir) *Handler {
	return &Handler{points: points, photos: photos, dir: dir}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.create)
	// «Мои» — раньше карточки: иначе /mine ушло бы в разбор uuid.
	r.Get("/mine", h.mine)
	r.Get("/{id}", h.card)
	r.Post("/{id}/enter", h.enter)
	r.Delete("/{id}/enter", h.leave)
	r.Get("/{id}/pool", h.pool)
	return r
}

// Cards serialises a list of points for whoever shows them — лента живёт в
// poolsapi, но карточка в ней та же, что и здесь.
func Cards(list []pointsrepo.FeedPoint, photos Photos, dir ZoneDir) []Card {
	out := make([]Card, 0, len(list))
	for _, fp := range list {
		zone, _ := dir.Get(fp.Point.ZoneID)
		out = append(out, toCard(fp, photos, toZoneDTO(zone)))
	}
	return out
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	var body createRequest
	if !decode(w, r, &body) {
		return
	}

	kind := entity.ParsePointType(body.Type)
	var created entity.Point
	var err error
	switch kind {
	case entity.PointFlat:
		created, err = h.points.CreateFlat(r.Context(), user.ID, body.toFlat())
	case entity.PointIntentHome:
		var in points.CreateIntentHome
		if in, err = body.toIntentHome(); err == nil {
			created, err = h.points.CreateIntentHome(r.Context(), user.ID, in)
		}
	default:
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "unknown point type",
			[]string{"type"}, errors.New("unknown point type "+body.Type))
		return
	}
	if err != nil {
		handleFormError(w, r, kind, err)
		return
	}

	// Создатель квартиры входит в её пул той же транзакцией — «ты первый в
	// пуле» (§6); у «к себе» пула нет вовсе (§2.2).
	card := pointsrepo.FeedPoint{Point: created, InPool: created.Type == entity.PointFlat}
	httpx.JSON(w, r, http.StatusCreated, h.toCard(card))
}

func (h *Handler) card(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	card, err := h.points.Point(r.Context(), user.ID, id)
	if err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.toCard(card))
}

// mine backs blocks 2–3 of «Моих поисков» (§6): «Присмотренные квартиры» —
// созданные вместе с теми, в чьи пулы человек вошёл, — и «Зову к себе».
func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}

	created, entered, err := h.points.Mine(r.Context(), user.ID)
	if err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, mineResponse{
		Created: Cards(created, h.photos, h.dir),
		Entered: Cards(entered, h.photos, h.dir),
	})
}

// enter is «Интересно — войти в пул» (§6): бесплатно, безлимитно и в один тап,
// поэтому повторный тап — не ошибка.
func (h *Handler) enter(w http.ResponseWriter, r *http.Request) {
	h.membership(w, r, h.points.Enter)
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	h.membership(w, r, h.points.Leave)
}

func (h *Handler) pool(w http.ResponseWriter, r *http.Request) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	view, err := h.points.Pool(r.Context(), user, id)
	if err != nil {
		handleError(w, r, err)
		return
	}

	members := make([]memberDTO, 0, len(view.Members))
	for _, m := range view.Members {
		members = append(members, toMember(m, h.photos, h.searchZone(m)))
	}
	httpx.JSON(w, r, http.StatusOK, poolResponse{Members: members})
}

func (h *Handler) membership(w http.ResponseWriter, r *http.Request, act func(context.Context, uuid.UUID, uuid.UUID) error) {
	user, ok := authmw.FromContext(r.Context())
	if !ok {
		httpx.Internal(w, r, errNoUserInContext)
		return
	}
	id, ok := pointID(w, r)
	if !ok {
		return
	}

	if err := act(r.Context(), user.ID, id); err != nil {
		handleError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusNoContent, nil)
}

func (h *Handler) toCard(fp pointsrepo.FeedPoint) Card {
	zone, _ := h.dir.Get(fp.Point.ZoneID)
	return toCard(fp, h.photos, toZoneDTO(zone))
}

// searchZone names the zone a pool member is searching in — «ищет на красной
// ветке». Поиска может не быть, и зона, которой справочник больше не знает,
// строку не ломает: карточка просто едет без неё.
func (h *Handler) searchZone(m points.MemberView) *zoneDTO {
	if m.Search == nil {
		return nil
	}
	zone, ok := h.dir.Get(m.Search.ZoneID)
	if !ok {
		return nil
	}
	dto := toZoneDTO(zone)
	return &dto
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))

	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.BadRequest(w, r, httpx.CodeBadRequest, "cannot read the request body", nil, err)
		return false
	}
	return true
}

func pointID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "point id is not a uuid", []string{"id"}, err)
		return uuid.Nil, false
	}
	return id, true
}
