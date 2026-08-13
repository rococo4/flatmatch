package points

import (
	"net/url"
	"slices"
	"strings"

	"github.com/AlekSi/pointer"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

// listingHosts — единственные разрешённые площадки (§5). Авито и Циан мы не
// парсим (§8): ссылка приносит квартиру с горячим искателем внутри, и знать,
// откуда она, нужно ровно для того, чтобы отсечь всё остальное.
var listingHosts = []string{"avito.ru", "cian.ru"}

// hostAliases — мобильные и www-зеркала тех же площадок: ссылка из приложения
// приходит с m., ссылка с десктопа — с www., квартира за ними одна.
var hostAliases = []string{"www.", "m."}

func (s *Service) validFlat(in CreateFlat) (entity.Point, error) {
	p := entity.Point{
		Type:       entity.PointFlat,
		Source:     entity.SourceUserLink,
		ZoneID:     in.ZoneID,
		PriceTotal: pointer.To(in.PriceTotal),
		Rooms:      in.Rooms,
		Isolated:   in.Isolated,
		Bathrooms:  in.Bathrooms,
		Utilities:  in.Utilities,
		PhotoKey:   in.PhotoKey,
		Deposit:    in.Deposit,
	}

	norm, urlOK := normalizeURL(in.URL)
	if urlOK {
		p.URLNorm = pointer.To(norm)
	}
	restrictions, restrictionsOK := validRestrictions(in.Restrictions)
	p.Restrictions = restrictions

	bad := invalidFields([]check{
		{"url", !urlOK},
		{"price_total", in.PriceTotal < MinPrice || in.PriceTotal > MaxPrice},
		{"bathrooms", in.Bathrooms != nil &&
			!known(*in.Bathrooms, entity.BathroomsUnknown, entity.ParseBathrooms)},
		{"utilities", in.Utilities != nil &&
			!known(*in.Utilities, entity.UtilitiesUnknown, entity.ParseUtilities)},
		{"restrictions", !restrictionsOK},
	})
	bad = append(bad, s.commonInvalid(in.ZoneID, in.PhotoKey, in.Rooms, in.Deposit)...)
	if len(bad) > 0 {
		return entity.Point{}, &entity.FieldsError{Err: ErrInvalidForm, Fields: bad}
	}
	return p, nil
}

func (s *Service) validIntentHome(in CreateIntentHome) (entity.Point, error) {
	p := entity.Point{
		Type:       entity.PointIntentHome,
		Stage:      pointer.To(in.Stage),
		Source:     entity.SourceUserLink,
		ZoneID:     in.ZoneID,
		PriceShare: pointer.To(in.PriceShare),
		Rooms:      in.Rooms,
		Isolated:   in.Isolated,
		PhotoKey:   in.PhotoKey,
		MoveIn:     in.MoveIn,
		Deposit:    in.Deposit,
	}

	restrictions, restrictionsOK := validRestrictions(in.Restrictions)
	p.Restrictions = restrictions

	bad := invalidFields([]check{
		{"stage", !known(in.Stage, entity.PointStageUnknown, entity.ParsePointStage)},
		// «Договорился и заезжаю» без даты заезда — не тот кейс (§2.2).
		{"move_in", in.Stage == entity.StageMovingIn && in.MoveIn == nil},
		{"price_share", in.PriceShare < MinPrice || in.PriceShare > MaxPrice},
		{"restrictions", !restrictionsOK},
	})
	bad = append(bad, s.commonInvalid(in.ZoneID, in.PhotoKey, in.Rooms, in.Deposit)...)
	if len(bad) > 0 {
		return entity.Point{}, &entity.FieldsError{Err: ErrInvalidForm, Fields: bad}
	}
	return p, nil
}

// commonInvalid — то, что обе формы спрашивают одинаково: метро из справочника
// (§5) и фото, без которого карточку не показать.
func (s *Service) commonInvalid(zoneID, photoKey string, rooms, deposit *int) []string {
	zone, ok := s.dir.Get(zoneID)
	return invalidFields([]check{
		// Точка стоит на станции: зона выдачи — район или ветка — собирается из
		// станций, и точка на «ветке» не пересеклась бы с районом (§8).
		{"zone_id", !ok || zone.Kind != zones.KindStation},
		{"photo_key", strings.TrimSpace(photoKey) == ""},
		{"rooms", rooms != nil && (*rooms < 1 || *rooms > entity.RoomsCap)},
		{"deposit", deposit != nil && (*deposit < 0 || *deposit > entity.PriceMaxCap)},
	})
}

type check struct {
	field string
	bad   bool
}

func invalidFields(checks []check) []string {
	var bad []string
	for _, c := range checks {
		if c.bad {
			bad = append(bad, c.field)
		}
	}
	return bad
}

// normalizeURL turns a pasted link into the key the dedup is built on: схема
// всегда https, зеркала m./www. сведены к одному хосту, проценты в пути
// раскрыты, хвостовые слэши и query отброшены — иначе одна и та же квартира
// заводится трижды (§6). Регистр пути остаётся как есть: слить два разных
// объявления в одно хуже, чем пропустить обход дедупа.
func normalizeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	// Ссылку копируют и без схемы; отказывать из-за этого — терять квартиру.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	for _, alias := range hostAliases {
		host = strings.TrimPrefix(host, alias)
	}
	if !slices.Contains(listingHosts, host) {
		return "", false
	}
	// Голый хост — не квартира: без пути ссылка ведёт на главную площадки, и
	// первый же такой «дубль» запер бы весь домен.
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		return "", false
	}
	return "https://" + host + path, true
}

// validRestrictions drops repeats: чипы «без животных» дважды — один и тот же
// факт, а в базе они лежат массивом кодов.
func validRestrictions(in []entity.Restriction) ([]entity.Restriction, bool) {
	out := make([]entity.Restriction, 0, len(in))
	for _, r := range in {
		if !known(r, entity.RestrictionUnknown, entity.ParseRestriction) {
			return nil, false
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out, true
}

// known: значение из словаря, а не пустое и не «unknown» — иначе CHECK в базе
// ответит пятисоткой на то, что должно быть 400.
func known[T ~string](v, unknown T, parse func(string) T) bool {
	return v != "" && v != unknown && parse(string(v)) == v
}
