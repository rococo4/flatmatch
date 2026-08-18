package pools

import (
	"bytes"
	"context"
	"slices"
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
			if url := photoURL(s.photos, sc.cand.User); url != "" {
				c.Avatars = append(c.Avatars, url)
			}
		}
	}
	return c, nil
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
				p.ZoneMatch = zoneMatch(dir, mine.ZoneID, sc.cand.Search.ZoneID)
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
		PhotoURL:      photoURL(photos, sc.cand.User),
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

func photoURL(photos PhotoURLs, u entity.User) string {
	if !u.PhotoApproved() {
		return ""
	}
	return photos.URL(u.PhotoKey)
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

func zoneMatch(dir *zones.Directory, mineID, theirsID string) bool {
	if mineID == theirsID {
		return true
	}
	mine, ok1 := dir.Get(mineID)
	theirs, ok2 := dir.Get(theirsID)
	if !ok1 || !ok2 {
		return false
	}
	return subset(mine.Stations, theirs.Stations) || subset(theirs.Stations, mine.Stations)
}

func subset(inner, outer []string) bool {
	set := make(map[string]bool, len(outer))
	for _, s := range outer {
		set[s] = true
	}
	for _, s := range inner {
		if !set[s] {
			return false
		}
	}
	return true
}

func freshestFirst(a, b Person) int {
	if c := b.SearchCreated.Compare(a.SearchCreated); c != 0 {
		return c
	}
	return bytes.Compare(a.User.ID[:], b.User.ID[:])
}
