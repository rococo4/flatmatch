package matchesapi

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/service/matches"
	"flatmatch/api/internal/service/zones"
)

// zoneDTO is the outbound twin of searchesapi's: same fields, same
// declensions. Copied rather than shared because a DTO belongs to the handler
// that serves it.
type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

type peerDTO struct {
	UserID   uuid.UUID `json:"user_id"`
	Name     string    `json:"name"`
	Age      int       `json:"age"`
	PhotoURL string    `json:"photo_url"`
}

// peerContactDTO is the ONLY DTO in the product that carries a way to reach a
// person (§3). A banned peer or a blocked pair arrives here with both fields
// empty — the service strips them, and the screen has nothing to dial.
type peerContactDTO struct {
	peerDTO

	Username   string `json:"username,omitempty"`
	TelegramID int64  `json:"tg_id"`
}

type matchDTO struct {
	MatchID uuid.UUID `json:"match_id"`
	Peer    peerDTO   `json:"peer"`
	Zone    *zoneDTO  `json:"zone,omitempty"`
	At      time.Time `json:"at"`
}

// matchDetailResponse is «Это мэтч» with its three blocks (§6): where the pair
// crossed paths, what their parameters have in common, what their
// questionnaires answered alike.
type matchDetailResponse struct {
	MatchID uuid.UUID      `json:"match_id"`
	Peer    peerContactDTO `json:"peer"`
	At      time.Time      `json:"at"`

	// Where and WherePoint are the same ГДЕ block in two shapes: зона пула или
	// точка. Контексты взаимоисключимы — лайк несёт один (§2.3), — поэтому
	// приезжает ровно одно из полей, а то и ни одного: точку могли закрыть, и
	// саммари переживает это молча (мэтч необратим).
	Where      *zoneDTO          `json:"where,omitempty"`
	WherePoint *pointDTO         `json:"where_point,omitempty"`
	Params     []paramOverlapDTO `json:"params"`
	Profile    []sharedDTO       `json:"profile"`
}

// pointDTO is the mini-card of «пересеклись на комнате Кати · м. Сокол»: the
// label is ready to print, the kind colours the header (§6). Its twin lives in
// the profile screen — the same phrase in the two places the context is retold.
type pointDTO struct {
	ID    uuid.UUID `json:"id"`
	Kind  string    `json:"kind"`
	Label string    `json:"label"`
}

// paramOverlapDTO is one line of the «по параметрам» block: a range («у обоих
// 40–45к»), a month or a term — whichever the parameter is made of.
type paramOverlapDTO struct {
	Param string `json:"param"`
	Min   *int   `json:"min,omitempty"`
	Max   *int   `json:"max,omitempty"`
	Month string `json:"month,omitempty"`
	Term  string `json:"term,omitempty"`
}

type sharedDTO struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

func toMatchDTO(m matchesrepo.MatchWithPeer, photoURL string, zone *zoneDTO) matchDTO {
	return matchDTO{
		MatchID: m.Match.ID,
		Peer:    toPeerDTO(m.Peer, photoURL),
		Zone:    zone,
		At:      m.Match.CreatedAt,
	}
}

func toMatchDetail(m matchesrepo.MatchWithPeer, summary matches.Summary, photoURL string) matchDetailResponse {
	out := matchDetailResponse{
		MatchID: m.Match.ID,
		Peer: peerContactDTO{
			peerDTO:    toPeerDTO(m.Peer, photoURL),
			Username:   m.Peer.Username,
			TelegramID: m.Peer.TelegramID,
		},
		At:      m.Match.CreatedAt,
		Params:  make([]paramOverlapDTO, 0, len(summary.ParamsOverlap)),
		Profile: toShared(summary.SharedAnswers),
	}
	if summary.Zone != nil {
		out.Where = pointer.To(toZoneDTO(*summary.Zone))
	}
	if summary.Point != nil {
		out.WherePoint = &pointDTO{
			ID:    summary.Point.ID,
			Kind:  string(summary.Point.Kind),
			Label: summary.Point.Label,
		}
	}
	for _, o := range summary.ParamsOverlap {
		out.Params = append(out.Params, toParamOverlap(o))
	}
	return out
}

func toPeerDTO(u entity.User, photoURL string) peerDTO {
	return peerDTO{UserID: u.ID, Name: u.Name, Age: u.Profile.Age, PhotoURL: photoURL}
}

// toParamOverlap: only the range parameters carry bounds, and an absent max
// means «и сколько угодно» — rooms whose upper bound neither side named. A
// degenerate range (min == max) is what «у обоих 45к» is made of; the DTO
// carries both numbers and the screen decides to print one.
func toParamOverlap(o matches.ParamOverlap) paramOverlapDTO {
	out := paramOverlapDTO{Param: o.Param, Month: o.Month, Term: o.Term}
	switch o.Param {
	case matches.ParamPrice, matches.ParamRooms:
		out.Min = pointer.To(o.Min)
		if o.Max != 0 {
			out.Max = pointer.To(o.Max)
		}
	}
	return out
}

func toShared(answers []entity.SharedAnswer) []sharedDTO {
	out := make([]sharedDTO, 0, len(answers))
	for _, a := range answers {
		out = append(out, sharedDTO{Field: a.Field, Value: a.Value})
	}
	return out
}

func toZoneDTO(z zones.Zone) zoneDTO {
	return zoneDTO{ID: z.ID, Name: z.Name, NameIn: z.NameIn, NameGen: z.NameGen}
}
