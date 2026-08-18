package points

import (
	"net/url"
	"slices"
	"strings"

	"github.com/AlekSi/pointer"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

var listingHosts = []string{"avito.ru", "cian.ru"}

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
		PhotoKeys:  in.PhotoKeys,
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
	bad = append(bad, s.commonInvalid(in.ZoneID, in.PhotoKeys, in.Rooms, in.Deposit)...)
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
		PhotoKeys:  in.PhotoKeys,
		MoveIn:     in.MoveIn,
		Deposit:    in.Deposit,
	}

	restrictions, restrictionsOK := validRestrictions(in.Restrictions)
	p.Restrictions = restrictions

	bad := invalidFields([]check{
		{"stage", !known(in.Stage, entity.PointStageUnknown, entity.ParsePointStage)},

		{"move_in", in.Stage == entity.StageMovingIn && in.MoveIn == nil},
		{"price_share", in.PriceShare < MinPrice || in.PriceShare > MaxPrice},
		{"restrictions", !restrictionsOK},
	})
	bad = append(bad, s.commonInvalid(in.ZoneID, in.PhotoKeys, in.Rooms, in.Deposit)...)
	if len(bad) > 0 {
		return entity.Point{}, &entity.FieldsError{Err: ErrInvalidForm, Fields: bad}
	}
	return p, nil
}

func (s *Service) commonInvalid(zoneID string, photoKeys []string, rooms, deposit *int) []string {
	zone, ok := s.dir.Get(zoneID)
	return invalidFields([]check{
		{"zone_id", !ok || zone.Kind != zones.KindStation},
		{"photo_keys", !validPhotoKeys(photoKeys)},
		{"rooms", rooms != nil && (*rooms < 1 || *rooms > entity.RoomsCap)},
		{"deposit", deposit != nil && (*deposit < 0 || *deposit > entity.PriceMaxCap)},
	})
}

func validPhotoKeys(keys []string) bool {
	if len(keys) < 1 || len(keys) > entity.MaxPointPhotos {
		return false
	}
	for _, k := range keys {
		if strings.TrimSpace(k) == "" {
			return false
		}
	}
	return true
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

func normalizeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

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

	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		return "", false
	}
	return "https://" + host + path, true
}

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

func known[T ~string](v, unknown T, parse func(string) T) bool {
	return v != "" && v != unknown && parse(string(v)) == v
}
