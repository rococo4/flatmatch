package people

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

type blockCall struct {
	blocker uuid.UUID
	blocked uuid.UUID
}

type fakeUsers struct {
	user    entity.User
	visible bool
	blocks  []blockCall
}

type fakeReports struct {
	filed     []entity.Report
	duplicate bool
}

type fakeMatches struct{ matched bool }

type filedCard struct {
	report   entity.Report
	reporter entity.User
	target   entity.User
}

type fakeModeration struct{ cards []filedCard }

func (f *fakeUsers) ByID(_ context.Context, id uuid.UUID) (entity.User, error) {
	if f.user.ID != id {
		return entity.User{}, entity.ErrNotFound
	}
	return f.user, nil
}

func (f *fakeUsers) VisibleByID(_ context.Context, _, id uuid.UUID) (entity.User, error) {
	if !f.visible || f.user.ID != id {
		return entity.User{}, entity.ErrNotFound
	}
	return f.user, nil
}

func (f *fakeUsers) Block(_ context.Context, blockerID, blockedID uuid.UUID) error {
	f.blocks = append(f.blocks, blockCall{blocker: blockerID, blocked: blockedID})
	return nil
}

func (f *fakeReports) Insert(_ context.Context, r entity.Report) (entity.Report, bool, error) {
	if f.duplicate {
		return entity.Report{}, false, nil
	}
	r.ID = uuid.New()
	f.filed = append(f.filed, r)
	return r, true, nil
}

func (f fakeMatches) IsMatched(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.matched, nil
}

func (f *fakeModeration) ReportFiled(_ context.Context, r entity.Report, reporter, target entity.User) {
	f.cards = append(f.cards, filedCard{report: r, reporter: reporter, target: target})
}

func newTestService(users *fakeUsers, reports *fakeReports, matched bool) (*Service, *fakeModeration) {
	mod := &fakeModeration{}
	return New(users, reports, fakeMatches{matched: matched}, mod), mod
}

func viewer() entity.User {
	return entity.User{ID: uuid.New(), Name: "Арсений", Status: entity.StatusActive}
}

func target() entity.User {
	return entity.User{ID: uuid.New(), Name: "Катя", Status: entity.StatusActive}
}

func TestBlockRefusesToBlockYourself(t *testing.T) {
	users := &fakeUsers{}
	svc, _ := newTestService(users, &fakeReports{}, false)
	me := uuid.New()

	if err := svc.Block(t.Context(), me, me); !errors.Is(err, entity.ErrSelfTarget) {
		t.Errorf("Block(self) = %v, want ErrSelfTarget", err)
	}
	if len(users.blocks) != 0 {
		t.Error("a self-block reached the database")
	}
}

func TestBlockNeedsAnExistingPerson(t *testing.T) {
	users := &fakeUsers{user: target()}
	svc, _ := newTestService(users, &fakeReports{}, false)

	if err := svc.Block(t.Context(), uuid.New(), uuid.New()); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Block(nobody) = %v, want ErrNotFound", err)
	}
	if len(users.blocks) != 0 {
		t.Error("a block of nobody reached the database")
	}
}

func TestBlockWorksOnAlreadyInvisiblePeople(t *testing.T) {
	them := target()
	users := &fakeUsers{user: them, visible: false}
	svc, _ := newTestService(users, &fakeReports{}, false)
	me := uuid.New()

	if err := svc.Block(t.Context(), me, them.ID); err != nil {
		t.Fatalf("Block() failed: %v", err)
	}
	if len(users.blocks) != 1 || users.blocks[0] != (blockCall{blocker: me, blocked: them.ID}) {
		t.Errorf("blocks = %+v, want one row from me to them", users.blocks)
	}
}

func TestReportRefusesToReportYourself(t *testing.T) {
	reports := &fakeReports{}
	me := viewer()
	svc, mod := newTestService(&fakeUsers{user: me, visible: true}, reports, false)

	err := svc.Report(t.Context(), me, me.ID, entity.ReasonSpam, "")
	if !errors.Is(err, entity.ErrSelfTarget) {
		t.Errorf("Report(self) = %v, want ErrSelfTarget", err)
	}
	if len(reports.filed) != 0 || len(mod.cards) != 0 {
		t.Error("a self-report was filed")
	}
}

func TestReportHidesUnrelatedPeople(t *testing.T) {
	them := target()
	reports := &fakeReports{}
	svc, mod := newTestService(&fakeUsers{user: them, visible: false}, reports, false)

	err := svc.Report(t.Context(), viewer(), them.ID, entity.ReasonScam, "")
	if !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Report(stranger) = %v, want ErrNotFound", err)
	}
	if len(reports.filed) != 0 || len(mod.cards) != 0 {
		t.Error("a report on an invisible person was filed")
	}
}

func TestReportSendsTheCardToTheModerator(t *testing.T) {
	me, them := viewer(), target()
	reports := &fakeReports{}
	svc, mod := newTestService(&fakeUsers{user: them, visible: true}, reports, false)

	if err := svc.Report(t.Context(), me, them.ID, entity.ReasonAbuse, "zone:d-khamovniki"); err != nil {
		t.Fatalf("Report() failed: %v", err)
	}

	if len(reports.filed) != 1 {
		t.Fatalf("filed %d reports, want 1", len(reports.filed))
	}
	filed := reports.filed[0]
	if filed.ReporterID != me.ID || filed.TargetID != them.ID {
		t.Errorf("filed report = %+v, want it from me about them", filed)
	}
	if filed.Reason != entity.ReasonAbuse || filed.ContextKey != "zone:d-khamovniki" {
		t.Errorf("filed report = %+v, want the reason and the context kept", filed)
	}

	if len(mod.cards) != 1 {
		t.Fatalf("sent %d cards to the moderator, want 1", len(mod.cards))
	}
	card := mod.cards[0]
	if card.report.ID != filed.ID || card.reporter.ID != me.ID || card.target.ID != them.ID {
		t.Errorf("card = %+v, want the stored report with both people", card)
	}
}

func TestReportOfTheSamePairSendsNoSecondCard(t *testing.T) {
	them := target()
	svc, mod := newTestService(&fakeUsers{user: them, visible: true}, &fakeReports{duplicate: true}, false)

	if err := svc.Report(t.Context(), viewer(), them.ID, entity.ReasonSpam, ""); err != nil {
		t.Fatalf("Report() failed: %v", err)
	}
	if len(mod.cards) != 0 {
		t.Error("a repeated complaint about the same person woke the moderator again")
	}
}

func TestReportReachesAMatchedPersonOutsideThePools(t *testing.T) {
	them := target()
	reports := &fakeReports{}
	svc, mod := newTestService(&fakeUsers{user: them, visible: false}, reports, true)

	if err := svc.Report(t.Context(), viewer(), them.ID, entity.ReasonScam, "match:"+uuid.NewString()); err != nil {
		t.Fatalf("Report() failed: %v", err)
	}
	if len(reports.filed) != 1 || len(mod.cards) != 1 {
		t.Errorf("filed %d reports and %d cards, want 1 and 1", len(reports.filed), len(mod.cards))
	}
}
