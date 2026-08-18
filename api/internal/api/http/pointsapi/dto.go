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

const moveInDate = "2006-01-02"

type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

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
	PhotoKeys    []string `json:"photo_keys"`
	MoveIn       *string  `json:"move_in"`
	Deposit      *int     `json:"deposit"`
}

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

	PhotoURLs []string `json:"photo_urls"`
	URL       string   `json:"url,omitempty"`
	MoveIn    string   `json:"move_in,omitempty"`
	Deposit   *int     `json:"deposit,omitempty"`

	CreatedDaysAgo int `json:"created_days_ago"`

	PoolCount int  `json:"pool_count"`
	Responses int  `json:"responses"`
	InPool    bool `json:"in_pool"`

	Author *authorDTO `json:"author,omitempty"`
}

type authorDTO struct {
	UserID     uuid.UUID `json:"user_id"`
	Name       string    `json:"name"`
	Age        int       `json:"age"`
	PhotoURL   string    `json:"photo_url"`
	Occupation string    `json:"occupation"`
	Schedule   string    `json:"schedule"`
	Smoking    string    `json:"smoking"`
	PetsOwn    string    `json:"pets_own"`
	LastSeen   time.Time `json:"last_seen"`
}

type chipDTO struct {
	Param   string `json:"param"`
	Verdict string `json:"verdict"`
}

type paramsDTO struct {
	PriceMin *int     `json:"price_min,omitempty"`
	PriceMax int      `json:"price_max"`
	RoomsMin *int     `json:"rooms_min,omitempty"`
	RoomsMax *int     `json:"rooms_max,omitempty"`
	MoveIn   *string  `json:"move_in,omitempty"`
	Term     *string  `json:"term,omitempty"`
	HasStuff []string `json:"has_stuff,omitempty"`
}

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
	Matched       bool       `json:"matched,omitempty"`
}

type photoResponse struct {
	PhotoKey string `json:"photo_key"`
}

type poolResponse struct {
	Members []memberDTO `json:"members"`
}

type mineResponse struct {
	Created []Card `json:"created"`
	Entered []Card `json:"entered"`
}

type duplicateResponse struct {
	Error   httpx.ErrorDetail `json:"error"`
	PointID *uuid.UUID        `json:"point_id,omitempty"`

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
		PhotoKeys:    b.PhotoKeys,
		Deposit:      b.Deposit,
	}
}

func (b createRequest) toIntentHome() (points.CreateIntentHome, error) {
	in := points.CreateIntentHome{
		Stage:        entity.ParsePointStage(b.Stage),
		ZoneID:       b.ZoneID,
		PriceShare:   b.PriceShare,
		Rooms:        b.Rooms,
		Isolated:     b.Isolated,
		Restrictions: parseRestrictions(b.Restrictions),
		PhotoKeys:    b.PhotoKeys,
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

func cardStatus(p entity.Point) string {
	if p.Status == entity.PointClosed && p.CloseReason != nil && *p.CloseReason == "rejected" {
		return "rejected"
	}
	return string(p.Status)
}

func photoURLs(keys []string, photos Photos) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, photos.URL(k))
	}
	return out
}

func toCard(fp pointsrepo.FeedPoint, photos Photos, zone zoneDTO) Card {
	p := fp.Point
	out := Card{
		ID:             p.ID,
		Type:           string(p.Type),
		Status:         cardStatus(p),
		Zone:           zone,
		PriceTotal:     p.PriceTotal,
		PriceShare:     p.PriceShare,
		Rooms:          p.Rooms,
		Isolated:       p.Isolated,
		Restrictions:   make([]string, 0, len(p.Restrictions)),
		PhotoURLs:      photoURLs(p.PhotoKeys, photos),
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
		UserID:     u.ID,
		Name:       u.Name,
		Age:        u.Profile.Age,
		PhotoURL:   personPhoto(u, photos),
		Occupation: string(u.Profile.Occupation),
		Schedule:   string(u.Profile.Schedule),
		Smoking:    string(u.Profile.Smoking),
		PetsOwn:    string(u.Profile.PetsOwn),
		LastSeen:   u.LastSeen,
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
		Matched:       m.Matched,
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

func personPhoto(u entity.User, photos Photos) string {
	if !u.PhotoApproved() {
		return ""
	}
	return photos.URL(u.PhotoKey)
}

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
