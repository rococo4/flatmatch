package pointsapi

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/points"
	"flatmatch/api/internal/service/zones"
)

// moveInDate is the wire format of a move-in date. У точки это день, а не
// месяц как у поиска: «заезжаю 15 августа» — конкретная дата (§2.2).
const moveInDate = "2006-01-02"

// zoneDTO is the outbound twin of searchesapi's: same fields, same
// declensions. Copied rather than shared because a DTO belongs to the handler
// that serves it.
type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

// createRequest carries both forms of §6 — «Найди, с кем снять эту квартиру» и
// «Зову соседа к себе», — потому что тип решает, какие поля читать, а не какой
// эндпоинт вызвать. Словарные значения парсятся, а не валидируются: незнакомое
// становится сентинелом, который сервис называет по имени в invalid_field.
type createRequest struct {
	Type string `json:"type"`

	URL        string `json:"url"`
	Stage      string `json:"stage"`
	ZoneID     string `json:"zone_id"`
	PriceTotal int    `json:"price_total"`
	PriceShare int    `json:"price_share"`

	Rooms        *int     `json:"rooms"`
	Isolated     *bool    `json:"isolated"`
	Bathrooms    *string  `json:"bathrooms"`
	Utilities    *string  `json:"utilities"`
	Restrictions []string `json:"restrictions"`
	PhotoKey     string   `json:"photo_key"`
	MoveIn       *string  `json:"move_in"` // «2026-09-15»
	Deposit      *int     `json:"deposit"`
}

// Card is the point as every screen shows it (§6): лента, карточка, оба блока
// «Моих поисков». Тип экспортирован, потому что ленту собирает poolsapi — с
// копией шапка карточки в двух местах разъехалась бы на первой же правке.
//
// Создателя в карточке нет: квартира с рынка обезличена, created_by — это
// provenance и лимиты (§2.2). Лицо есть только у «к себе», и приезжает оно
// автором.
type Card struct {
	ID     uuid.UUID `json:"id"`
	Type   string    `json:"type"`
	Stage  string    `json:"stage,omitempty"`
	Status string    `json:"status"`
	Zone   zoneDTO   `json:"zone"`

	PriceTotal   *int     `json:"price_total,omitempty"`
	PriceShare   *int     `json:"price_share,omitempty"`
	Rooms        *int     `json:"rooms,omitempty"`
	Isolated     *bool    `json:"isolated,omitempty"`
	Bathrooms    string   `json:"bathrooms,omitempty"`
	Utilities    string   `json:"utilities,omitempty"`
	Restrictions []string `json:"restrictions,omitempty"`
	// PhotoURL едет всегда: фото точки — скрин объявления или комната, и
	// модерация одобряет его вместе с точкой, отдельного статуса у него нет.
	PhotoURL string `json:"photo_url"`
	URL      string `json:"url,omitempty"`
	MoveIn   string `json:"move_in,omitempty"`
	Deposit  *int   `json:"deposit,omitempty"`

	// CreatedDaysAgo backs «добавлена N дней назад» — точность до дня, «20
	// минут назад» продукт не обещает (§8).
	CreatedDaysAgo int `json:"created_days_ago"`
	// PoolCount is «в пуле N» рыночной квартиры, Responses — «N откликов» у
	// «к себе»: какой из счётчиков показать, решает тип карточки (§2.2).
	PoolCount int  `json:"pool_count"`
	Responses int  `json:"responses"`
	InPool    bool `json:"in_pool"`

	Author *authorDTO `json:"author,omitempty"`
}

// authorDTO is the showcase of «к себе» — единственная карточка с лицом (§2.2).
// Контактов не несёт, как и всё, кроме экрана мэтча (§3); dto_test это пинует.
type authorDTO struct {
	UserID   uuid.UUID `json:"user_id"`
	Name     string    `json:"name"`
	Age      int       `json:"age"`
	PhotoURL string    `json:"photo_url"`
	Schedule string    `json:"schedule"`
	Smoking  string    `json:"smoking"`
	PetsOwn  string    `json:"pets_own"`
	LastSeen time.Time `json:"last_seen"`
}

type chipDTO struct {
	Param   string `json:"param"`
	Verdict string `json:"verdict"`
}

// paramsDTO is the outbound twin of poolsapi's: same fields, same encodings.
type paramsDTO struct {
	PriceMin *int     `json:"price_min,omitempty"`
	PriceMax int      `json:"price_max"`
	RoomsMin *int     `json:"rooms_min,omitempty"`
	RoomsMax *int     `json:"rooms_max,omitempty"`
	MoveIn   *string  `json:"move_in,omitempty"` // «2026-09»
	Term     *string  `json:"term,omitempty"`
	HasStuff []string `json:"has_stuff,omitempty"`
}

// memberDTO is one person in the pool of a flat (§6): совпадения анкеты и
// вердикт-чипы, если смотрящему есть чем сравнивать. Поиска у участника может
// не быть вовсе — вход в пул его не требует (§2.2), — и тогда карточка едет
// без параметров и без чипов.
type memberDTO struct {
	UserID        uuid.UUID  `json:"user_id"`
	Name          string     `json:"name"`
	Age           int        `json:"age"`
	PhotoURL      string     `json:"photo_url"`
	Schedule      string     `json:"schedule"`
	Smoking       string     `json:"smoking"`
	PetsOwn       string     `json:"pets_own"`
	SharedAnswers int        `json:"shared_answers"`
	Chips         []chipDTO  `json:"chips,omitempty"`
	Params        *paramsDTO `json:"params,omitempty"`
	Zone          *zoneDTO   `json:"zone,omitempty"`
	EnteredAt     time.Time  `json:"entered_at"`
}

type poolResponse struct {
	Members []memberDTO `json:"members"`
}

type mineResponse struct {
	Created []Card `json:"created"`
	Entered []Card `json:"entered"`
}

// duplicateResponse is the 409 of a link somebody already brought: «эта
// квартира уже есть, в пуле N — войти?» (§6). Всё, что нужно кнопке «Войти»,
// едет рядом с сообщением — иначе фронту пришлось бы искать точку по ссылке
// второй раз.
type duplicateResponse struct {
	Error   httpx.ErrorDetail `json:"error"`
	PointID uuid.UUID         `json:"point_id"`
	// PoolCount is «в пуле N» существующей точки — персональный, как всякий
	// счётчик пула (§4).
	PoolCount int    `json:"pool_count"`
	Status    string `json:"status"`
}

func (b createRequest) toFlat() points.CreateFlat {
	return points.CreateFlat{
		URL:          b.URL,
		ZoneID:       b.ZoneID,
		PriceTotal:   b.PriceTotal,
		Rooms:        b.Rooms,
		Isolated:     b.Isolated,
		Bathrooms:    parseEnum(b.Bathrooms, entity.ParseBathrooms),
		Utilities:    parseEnum(b.Utilities, entity.ParseUtilities),
		Restrictions: parseRestrictions(b.Restrictions),
		PhotoKey:     b.PhotoKey,
		Deposit:      b.Deposit,
	}
}

// toIntentHome fails only on a date that cannot be represented at all; всё
// остальное — вопрос сервиса, включая обязательность заезда у «заезжаю» (§2.2).
func (b createRequest) toIntentHome() (points.CreateIntentHome, error) {
	in := points.CreateIntentHome{
		Stage:        entity.ParsePointStage(b.Stage),
		ZoneID:       b.ZoneID,
		PriceShare:   b.PriceShare,
		Rooms:        b.Rooms,
		Isolated:     b.Isolated,
		Restrictions: parseRestrictions(b.Restrictions),
		PhotoKey:     b.PhotoKey,
		Deposit:      b.Deposit,
	}
	if b.MoveIn != nil && *b.MoveIn != "" {
		day, err := time.Parse(moveInDate, *b.MoveIn)
		if err != nil {
			return points.CreateIntentHome{},
				&entity.FieldsError{Err: points.ErrInvalidForm, Fields: []string{"move_in"}}
		}
		in.MoveIn = &day
	}
	return in, nil
}

// toCard serialises a point. The photo of the point goes through the store as
// it is; the face of an author goes through the §5 rule, the same one every
// other screen applies.
func toCard(fp pointsrepo.FeedPoint, photos Photos, zone zoneDTO) Card {
	p := fp.Point
	out := Card{
		ID:             p.ID,
		Type:           string(p.Type),
		Status:         string(p.Status),
		Zone:           zone,
		PriceTotal:     p.PriceTotal,
		PriceShare:     p.PriceShare,
		Rooms:          p.Rooms,
		Isolated:       p.Isolated,
		Restrictions:   make([]string, 0, len(p.Restrictions)),
		PhotoURL:       photos.URL(p.PhotoKey),
		CreatedDaysAgo: daysAgo(p.CreatedAt),
		PoolCount:      fp.PoolCount,
		Responses:      fp.Responses,
		InPool:         fp.InPool,
	}
	if p.Stage != nil {
		out.Stage = string(*p.Stage)
	}
	if p.Bathrooms != nil {
		out.Bathrooms = string(*p.Bathrooms)
	}
	if p.Utilities != nil {
		out.Utilities = string(*p.Utilities)
	}
	for _, r := range p.Restrictions {
		out.Restrictions = append(out.Restrictions, string(r))
	}
	// Ссылка на оригинал — нормализованная: исходную строку схема не хранит, а
	// нормализация трогает только схему, зеркало хоста и хвост (§6).
	if p.URLNorm != nil {
		out.URL = *p.URLNorm
	}
	if p.MoveIn != nil {
		out.MoveIn = p.MoveIn.Format(moveInDate)
	}
	if p.Deposit != nil {
		out.Deposit = p.Deposit
	}
	if fp.Author != nil {
		out.Author = pointer.To(toAuthor(*fp.Author, photos))
	}
	return out
}

func toAuthor(u entity.User, photos Photos) authorDTO {
	return authorDTO{
		UserID:   u.ID,
		Name:     u.Name,
		Age:      u.Profile.Age,
		PhotoURL: personPhoto(u, photos),
		Schedule: string(u.Profile.Schedule),
		Smoking:  string(u.Profile.Smoking),
		PetsOwn:  string(u.Profile.PetsOwn),
		LastSeen: u.LastSeen,
	}
}

func toMember(m points.MemberView, photos Photos, zone *zoneDTO) memberDTO {
	out := memberDTO{
		UserID:        m.User.ID,
		Name:          m.User.Name,
		Age:           m.User.Profile.Age,
		PhotoURL:      personPhoto(m.User, photos),
		Schedule:      string(m.User.Profile.Schedule),
		Smoking:       string(m.User.Profile.Smoking),
		PetsOwn:       string(m.User.Profile.PetsOwn),
		SharedAnswers: m.SharedAnswers,
		Zone:          zone,
		EnteredAt:     m.EnteredAt,
	}
	if m.Search != nil {
		out.Params = pointer.To(toParamsDTO(m.Search.SearchParams))
	}
	for _, c := range m.Chips {
		out.Chips = append(out.Chips, chipDTO{Param: string(c.Param), Verdict: string(c.Verdict)})
	}
	return out
}

func toParamsDTO(p entity.SearchParams) paramsDTO {
	out := paramsDTO{
		PriceMin: p.PriceMin,
		PriceMax: p.PriceMax,
		RoomsMin: p.RoomsMin,
		RoomsMax: p.RoomsMax,
	}
	if p.MoveIn != nil {
		out.MoveIn = pointer.To(p.MoveIn.Format("2006-01"))
	}
	if p.Term != nil {
		out.Term = pointer.To(string(*p.Term))
	}
	for _, chip := range p.HasStuff {
		out.HasStuff = append(out.HasStuff, string(chip))
	}
	return out
}

func toZoneDTO(z zones.Zone) zoneDTO {
	return zoneDTO{ID: z.ID, Name: z.Name, NameIn: z.NameIn, NameGen: z.NameGen}
}

// personPhoto is the §5 rule: фото, которого модератор не одобрил, вместо
// адреса отдаёт пустоту — на экране его место занимает заглушка.
func personPhoto(u entity.User, photos Photos) string {
	if !u.PhotoApproved() {
		return ""
	}
	return photos.URL(u.PhotoKey)
}

// daysAgo counts whole days, что и обещает строка свежести: «добавлена вчера»
// у точки часовой давности было бы враньём в обе стороны.
func daysAgo(t time.Time) int {
	days := int(time.Since(t) / (24 * time.Hour))
	if days < 0 {
		return 0
	}
	return days
}

func parseEnum[T ~string](raw *string, parse func(string) T) *T {
	if raw == nil || *raw == "" {
		return nil
	}
	return pointer.To(parse(*raw))
}

func parseRestrictions(raw []string) []entity.Restriction {
	out := make([]entity.Restriction, 0, len(raw))
	for _, code := range raw {
		out = append(out, entity.ParseRestriction(code))
	}
	return out
}
