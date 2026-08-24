package deck

import (
	"bytes"
	"context"
	"errors"
	"slices"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

func New(searches Searches, points Points, passes Passes, people People,
	matches Matches, likes Likes, dir *zones.Directory,
) *Service {
	return &Service{
		searches: searches, points: points, passes: passes,
		people: people, matches: matches, likes: likes, dir: dir,
	}
}

func (s *Service) Deck(ctx context.Context, viewerID uuid.UUID, q Query) (Deck, error) {
	passed, err := s.passes.ActiveTargets(ctx, viewerID, passWindow)
	if err != nil {
		return Deck{}, err
	}

	list, err := s.searches.ListByUser(ctx, viewerID)
	if err != nil {
		return Deck{}, err
	}

	mine := activeOnly(list)
	if len(mine) == 0 {
		return Deck{Cards: []Card{}, Passed: len(passed), NoSearch: true}, nil
	}
	zoneIDs := s.zoneUnion(mine)

	var cards []Card
	if q.People {
		people, err := s.peopleCards(ctx, viewerID, zoneIDs, mine, q.Filter)
		if err != nil {
			return Deck{}, err
		}
		cards = append(cards, people...)
	}
	if q.Rooms {
		rooms, err := s.roomCards(ctx, viewerID, zoneIDs, mine, q.Filter)
		if err != nil {
			return Deck{}, err
		}
		cards = append(cards, rooms...)
	}

	cards = roomsWinOverPeople(cards)
	slices.SortFunc(cards, deckOrder)
	out := Deck{Cards: []Card{}, Total: len(cards), Passed: len(passed)}
	if len(cards) > cardCap {
		cards = cards[:cardCap]
	}
	if len(cards) > 0 {
		out.Cards = cards
	}
	return out, nil
}

func (s *Service) PassPerson(ctx context.Context, viewerID, targetID uuid.UUID) error {
	if viewerID == targetID {
		return entity.ErrSelfTarget
	}

	reachable, err := s.reachable(ctx, viewerID, targetID)
	if err != nil {
		return err
	}
	if !reachable {
		return entity.ErrNotFound
	}
	return s.passes.Upsert(ctx, viewerID, targetID)
}

func (s *Service) PassPoint(ctx context.Context, viewerID, pointID uuid.UUID) error {
	point, err := s.points.ByID(ctx, pointID)
	if err != nil {
		return err
	}
	if point.Type != entity.PointIntentHome || point.Status != entity.PointActive {
		return entity.ErrNotFound
	}
	if point.CreatedBy != nil && *point.CreatedBy == viewerID {
		return entity.ErrSelfTarget
	}
	return s.passes.Upsert(ctx, viewerID, pointID)
}

func (s *Service) ClearPasses(ctx context.Context, userID uuid.UUID) (int, error) {
	passed, err := s.passes.ActiveTargets(ctx, userID, passWindow)
	if err != nil {
		return 0, err
	}
	if _, err := s.passes.Clear(ctx, userID); err != nil {
		return 0, err
	}
	return len(passed), nil
}

func (s *Service) RankIncoming(ctx context.Context, viewerID uuid.UUID,
	list []likesrepo.IncomingLike,
) ([]RankedLike, error) {
	if len(list) == 0 {
		return nil, nil
	}

	passed, err := s.passes.ActiveTargets(ctx, viewerID, passWindow)
	if err != nil {
		return nil, err
	}
	kept := make([]likesrepo.IncomingLike, 0, len(list))
	ids := make([]uuid.UUID, 0, len(list))
	for _, like := range list {
		if slices.Contains(passed, like.From.ID) {
			continue
		}
		kept = append(kept, like)
		ids = append(ids, like.From.ID)
	}
	if len(kept) == 0 {
		return nil, nil
	}

	mineList, err := s.searches.ListByUser(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	theirs, err := s.searches.ActiveByUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	byUser := make(map[uuid.UUID][]entity.Search, len(ids))
	for _, search := range theirs {
		byUser[search.UserID] = append(byUser[search.UserID], search)
	}

	mine := activeOnly(mineList)
	out := make([]RankedLike, 0, len(kept))
	for _, like := range kept {
		ranked := RankedLike{Like: like}
		if pair, ok := pools.BestPair(s.dir, mine, byUser[like.From.ID]); ok {
			ranked.Pair = pointer.To(toPair(pair))
		}
		out = append(out, ranked)
	}
	slices.SortFunc(out, incomingOrder)
	return out, nil
}

func (s *Service) peopleCards(ctx context.Context, viewerID uuid.UUID, zoneIDs []string,
	mine []entity.Search, f entity.PeopleFilter,
) ([]Card, error) {
	candidates, err := s.searches.DeckCandidates(ctx, viewerID, zoneIDs, f, passWindow)
	if err != nil {
		return nil, err
	}

	out := make([]Card, 0, len(candidates))
	for _, candidate := range candidates {
		pair, ok := pools.BestPair(s.dir, mine, candidate.Searches)
		if !ok {
			continue
		}
		out = append(out, Card{
			Kind:     KindPerson,
			Person:   candidate.User,
			Pair:     toPair(pair),
			activity: candidate.User.LastSeen,
		})
	}
	return out, nil
}

func (s *Service) roomCards(ctx context.Context, viewerID uuid.UUID, zoneIDs []string,
	mine []entity.Search, f entity.PeopleFilter,
) ([]Card, error) {
	rooms, err := s.points.DeckRooms(ctx, viewerID, zoneIDs, f, passWindow)
	if err != nil {
		return nil, err
	}

	out := make([]Card, 0, len(rooms))
	for _, room := range rooms {
		pair, ok := s.roomPair(mine, room.Point)
		if !ok {
			continue
		}
		out = append(out, Card{
			Kind:     KindRoom,
			Room:     room,
			Pair:     pair,
			activity: room.Point.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) roomPair(mine []entity.Search, room entity.Point) (Pair, bool) {
	zone, _ := s.dir.Get(room.ZoneID)

	var best Pair
	var bestSearch entity.Search
	var found bool
	for _, m := range mine {
		if !slices.Contains(s.dir.Intersecting(m.ZoneID), room.ZoneID) {
			continue
		}

		next := Pair{
			Zone:      zone,
			ZoneMatch: pools.ZoneMatch(s.dir, m.ZoneID, room.ZoneID),
			Section:   pools.SectionNone,
		}
		if shareFits(m.SearchParams, room.PriceShare) {
			next.Section = pools.SectionAll
		}
		if !found || pools.BetterSection(next.Section, best.Section) ||
			next.Section == best.Section && m.CreatedAt.After(bestSearch.CreatedAt) {
			best, bestSearch, found = next, m, true
		}
	}
	return best, found
}

func (s *Service) reachable(ctx context.Context, viewerID, targetID uuid.UUID) (bool, error) {
	_, err := s.people.VisibleByID(ctx, viewerID, targetID)
	switch {
	case err == nil:
		return true, nil
	case !errors.Is(err, entity.ErrNotFound):
		return false, err
	}

	matched, err := s.matches.IsMatched(ctx, viewerID, targetID)
	if err != nil || matched {
		return matched, err
	}

	_, likedMe, err := s.likes.Between(ctx, viewerID, targetID)
	return likedMe, err
}

func (s *Service) zoneUnion(mine []entity.Search) []string {
	seen := make(map[string]bool)
	var out []string
	for _, search := range mine {
		for _, id := range s.dir.Intersecting(search.ZoneID) {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

func toPair(p pools.Pair) Pair {
	return Pair{
		Zone:      p.Zone,
		ZoneMatch: p.ZoneMatch,
		Section:   p.Section,
		Chips:     p.Chips,
		Params:    pointer.To(p.Theirs.SearchParams),
	}
}

func roomsWinOverPeople(cards []Card) []Card {
	authors := make(map[uuid.UUID]bool)
	for _, card := range cards {
		if card.Kind == KindRoom && card.Room.Point.CreatedBy != nil {
			authors[*card.Room.Point.CreatedBy] = true
		}
	}
	if len(authors) == 0 {
		return cards
	}

	out := make([]Card, 0, len(cards))
	for _, card := range cards {
		if card.Kind == KindPerson && authors[card.Person.ID] {
			continue
		}
		out = append(out, card)
	}
	return out
}

func activeOnly(list []entity.Search) []entity.Search {
	out := make([]entity.Search, 0, len(list))
	for _, search := range list {
		if search.Status == entity.SearchActive {
			out = append(out, search)
		}
	}
	return out
}

func shareFits(mine entity.SearchParams, share *int) bool {
	if share == nil {
		return false
	}
	if mine.PriceMin != nil && *share < *mine.PriceMin {
		return false
	}
	return *share <= mine.PriceMax
}

func deckOrder(a, b Card) int {
	if a.Pair.Section != b.Pair.Section {
		if pools.BetterSection(a.Pair.Section, b.Pair.Section) {
			return -1
		}
		return 1
	}
	if c := b.activity.Compare(a.activity); c != 0 {
		return c
	}
	keyA, keyB := a.key(), b.key()
	return bytes.Compare(keyA[:], keyB[:])
}

func incomingOrder(a, b RankedLike) int {
	if c := sectionRank(b.Pair) - sectionRank(a.Pair); c != 0 {
		return c
	}
	if c := greenChips(b.Pair) - greenChips(a.Pair); c != 0 {
		return c
	}
	return b.Like.CreatedAt.Compare(a.Like.CreatedAt)
}

func sectionRank(p *Pair) int {
	switch {
	case p == nil:
		return 0
	case p.Section == pools.SectionAll:
		return 3
	case p.Section == pools.SectionPartial:
		return 2
	default:
		return 1
	}
}

func greenChips(p *Pair) int {
	if p == nil {
		return 0
	}
	return pools.GreenChips(p.Chips)
}
