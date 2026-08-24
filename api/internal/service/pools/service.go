package pools

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

func New(repo Repository, dir *zones.Directory, photos PhotoURLs) *Service {
	return &Service{repo: repo, dir: dir, photos: photos}
}

func (s *Service) ZonePool(ctx context.Context, viewer entity.User, zoneID string) (Pool, error) {
	zone, ok := s.dir.Get(zoneID)
	if !ok {
		return Pool{}, entity.ErrNotFound
	}

	mine, err := s.repo.ListByUser(ctx, viewer.ID)
	if err != nil {
		return Pool{}, err
	}

	var mySearch *entity.Search
	intersecting := s.dir.Intersecting(zoneID)
	for i, m := range mine {
		if m.Status != entity.SearchActive {
			continue
		}
		if m.ZoneID == zoneID {
			mySearch = &mine[i]
			break
		}
		if mySearch == nil && slices.Contains(intersecting, m.ZoneID) {
			mySearch = &mine[i]
		}
	}

	if mySearch == nil {
		return Pool{}, entity.ErrNeedSearch
	}

	cands, err := s.repo.PoolCandidates(ctx, viewer.ID, intersecting)
	if err != nil {
		return Pool{}, err
	}

	pool := Pool{Zone: zone, MySearch: exactOnly(mySearch, zoneID), Ranked: true}
	pool.Sections = rankedSections(s.dir, s.photos, viewer, mySearch.SearchParams, cands)

	if err := s.repo.MarkPoolSeen(ctx, viewer.ID, zoneID); err != nil {
		return Pool{}, err
	}
	return pool, nil
}

func (s *Service) Counts(ctx context.Context, viewerID uuid.UUID, params entity.SearchParams, newSince *time.Time) (Counts, error) {
	if _, ok := s.dir.Get(params.ZoneID); !ok {
		return Counts{}, entity.ErrNotFound
	}

	cands, err := s.repo.PoolCandidates(ctx, viewerID, s.dir.Intersecting(params.ZoneID))
	if err != nil {
		return Counts{}, err
	}

	best := bestPerUser(params, cands)
	c := Counts{People: len(best.order)}
	for _, userID := range best.order {
		sc := best.byUser[userID]
		if sc.section == SectionAll {
			c.FullMatch++
		}
		if newSince != nil && sc.cand.Search.CreatedAt.After(*newSince) {
			c.NewCount++
		}
		if len(c.Avatars) < avatarCap {
			if urls := photoURLs(s.photos, sc.cand.User); len(urls) > 0 {
				c.Avatars = append(c.Avatars, urls[0])
			}
		}
	}
	return c, nil
}

func (s *Service) Top(ctx context.Context, viewerID uuid.UUID, limit int) ([]ZoneTop, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := s.repo.ActiveZonesVisible(ctx, viewerID)
	if err != nil {
		return nil, err
	}

	people := make(map[string]map[uuid.UUID]bool)
	for _, row := range rows {
		for _, zoneID := range s.dir.Intersecting(row.ZoneID) {
			set, ok := people[zoneID]
			if !ok {
				set = make(map[uuid.UUID]bool, len(row.UserIDs))
				people[zoneID] = set
			}
			for _, userID := range row.UserIDs {
				set[userID] = true
			}
		}
	}

	top := make([]ZoneTop, 0, len(people))
	for zoneID, set := range people {
		zone, ok := s.dir.Get(zoneID)
		if !ok || len(set) == 0 {
			continue
		}
		top = append(top, ZoneTop{Zone: zone, People: len(set)})
	}
	slices.SortFunc(top, func(a, b ZoneTop) int {
		if a.People != b.People {
			return b.People - a.People
		}
		return strings.Compare(a.Zone.Name, b.Zone.Name)
	})
	if len(top) > limit {
		top = top[:limit]
	}
	return top, nil
}

func bestPerUser(mine entity.SearchParams, cands []entity.PoolCandidate) bestByUser {
	best := bestByUser{byUser: make(map[uuid.UUID]scored, len(cands))}
	for _, cand := range cands {
		chips, section := Compare(mine, cand.Search.SearchParams)
		next := scored{cand: cand, chips: chips, section: section}

		prev, seen := best.byUser[cand.User.ID]
		switch {
		case !seen:
			best.byUser[cand.User.ID] = next
			best.order = append(best.order, cand.User.ID)
		case BetterSection(next.section, prev.section),
			next.section == prev.section &&
				next.cand.Search.CreatedAt.After(prev.cand.Search.CreatedAt):
			best.byUser[cand.User.ID] = next
		}
	}
	return best
}

func rankedSections(dir *zones.Directory, photos PhotoURLs, viewer entity.User, mine entity.SearchParams, cands []entity.PoolCandidate) []Section {
	best := bestPerUser(mine, cands)
	sections := []Section{{ID: SectionAll}, {ID: SectionPartial}, {ID: SectionNone}}
	for _, userID := range best.order {
		sc := best.byUser[userID]
		for i := range sections {
			if sections[i].ID == sc.section {
				p := newPerson(dir, photos, viewer, sc)
				p.ZoneMatch = ZoneMatch(dir, mine.ZoneID, sc.cand.Search.ZoneID)
				sections[i].People = append(sections[i].People, p)
			}
		}
	}

	for i := range sections {
		slices.SortStableFunc(sections[i].People, zoneMatchFirst)
		if sections[i].ID == SectionNone && len(sections[i].People) > noneSectionCap {
			sections[i].People = sections[i].People[:noneSectionCap]
		}
	}
	return sections
}

func newPerson(dir *zones.Directory, photos PhotoURLs, viewer entity.User, sc scored) Person {
	zone, _ := dir.Get(sc.cand.Search.ZoneID)
	return Person{
		User:          sc.cand.User,
		PhotoURLs:     photoURLs(photos, sc.cand.User),
		SharedAnswers: viewer.Profile.SharedAnswers(sc.cand.User.Profile),
		Chips:         sc.chips,
		Params:        sc.cand.Search.SearchParams,
		Zone:          zone,
		SearchCreated: sc.cand.Search.CreatedAt,
	}
}

func exactOnly(search *entity.Search, zoneID string) *entity.Search {
	if search == nil || search.ZoneID != zoneID {
		return nil
	}
	return search
}

func photoURLs(photos PhotoURLs, u entity.User) []string {
	if !u.PhotoApproved() {
		return []string{}
	}

	out := make([]string, 0, len(u.PhotoKeys))
	for _, key := range u.PhotoKeys {
		out = append(out, photos.URL(key))
	}
	return out
}

func zoneMatchFirst(a, b Person) int {
	if a.ZoneMatch != b.ZoneMatch {
		if a.ZoneMatch {
			return -1
		}
		return 1
	}
	return freshestFirst(a, b)
}

func freshestFirst(a, b Person) int {
	if c := b.SearchCreated.Compare(a.SearchCreated); c != 0 {
		return c
	}
	return bytes.Compare(a.User.ID[:], b.User.ID[:])
}
