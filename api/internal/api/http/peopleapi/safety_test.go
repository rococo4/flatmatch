package peopleapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

type blockCall struct {
	viewer uuid.UUID
	target uuid.UUID
}

type reportCall struct {
	viewer     uuid.UUID
	target     uuid.UUID
	reason     entity.ReportReason
	contextKey string
}

type fakeService struct {
	blocks  []blockCall
	reports []reportCall
	err     error
}

func (f *fakeService) Block(_ context.Context, viewerID, targetID uuid.UUID) error {
	f.blocks = append(f.blocks, blockCall{viewer: viewerID, target: targetID})
	return f.err
}

func (f *fakeService) Report(_ context.Context, viewer entity.User, targetID uuid.UUID,
	reason entity.ReportReason, contextKey string,
) error {
	f.reports = append(f.reports, reportCall{
		viewer: viewer.ID, target: targetID, reason: reason, contextKey: contextKey,
	})
	return f.err
}

func newSafetyRouter(service *fakeService, viewerID uuid.UUID) chi.Router {
	handler := New(service, &fakePeople{}, &fakeLikes{}, fakePhotos{},
		fakeSearches{}, fakePoints{}, fakeMatches{}, &fakePasses{}, fakeDir{})
	return newRouter(handler, viewerID)
}

func post(router chi.Router, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "tma stub")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestBlockStoresThePairAndAnswersEmpty(t *testing.T) {
	service := &fakeService{}
	viewer := uuid.New()
	them := uuid.New()

	rec := post(newSafetyRouter(service, viewer), "/people/"+them.String()+"/block", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNoContent, rec.Body)
	}
	if len(service.blocks) != 1 || service.blocks[0] != (blockCall{viewer: viewer, target: them}) {
		t.Errorf("blocks = %+v, want one call from the viewer about them", service.blocks)
	}
}

func TestBlockAnswersTheServiceVerdicts(t *testing.T) {
	tests := map[string]struct {
		err  error
		path string
		want int
	}{
		"себя":          {err: entity.ErrSelfTarget, want: http.StatusBadRequest},
		"нет человека":  {err: entity.ErrNotFound, want: http.StatusNotFound},
		"кривой id":     {path: "/people/not-a-uuid/block", want: http.StatusBadRequest},
		"всё в порядке": {want: http.StatusNoContent},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			path := tt.path
			if path == "" {
				path = "/people/" + uuid.NewString() + "/block"
			}
			rec := post(newSafetyRouter(&fakeService{err: tt.err}, uuid.New()), path, "")

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestReportCarriesTheReasonAndTheContext(t *testing.T) {
	zone := "d-khamovniki"
	point := uuid.New()
	match := uuid.New()

	tests := map[string]struct {
		body string
		want string
	}{
		"зона":        {body: `{"reason":"scam","context":{"zone_id":"` + zone + `"}}`, want: "zone:" + zone},
		"точка":       {body: `{"reason":"spam","context":{"point_id":"` + point.String() + `"}}`, want: "point:" + point.String()},
		"мэтч":        {body: `{"reason":"abuse","context":{"match_id":"` + match.String() + `"}}`, want: "match:" + match.String()},
		"без деталей": {body: `{"reason":"other"}`, want: ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			service := &fakeService{}
			viewer, them := uuid.New(), uuid.New()

			rec := post(newSafetyRouter(service, viewer), "/people/"+them.String()+"/report", tt.body)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNoContent, rec.Body)
			}
			if len(service.reports) != 1 {
				t.Fatalf("filed %d reports, want 1", len(service.reports))
			}
			filed := service.reports[0]
			if filed.viewer != viewer || filed.target != them {
				t.Errorf("report = %+v, want it from the viewer about them", filed)
			}
			if filed.contextKey != tt.want {
				t.Errorf("context key = %q, want %q", filed.contextKey, tt.want)
			}
			if !filed.reason.Known() {
				t.Errorf("reason = %q, want a known one", filed.reason)
			}
		})
	}
}

func TestReportRefusesAReasonOutsideTheChips(t *testing.T) {
	for _, body := range []string{`{"reason":"whatever"}`, `{"reason":""}`, `{}`, `{"reason":"scam","extra":1}`, ``} {
		service := &fakeService{}

		rec := post(newSafetyRouter(service, uuid.New()), "/people/"+uuid.NewString()+"/report", body)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status for %q = %d, want %d (%s)", body, rec.Code, http.StatusBadRequest, rec.Body)
		}
		if len(service.reports) != 0 {
			t.Errorf("body %q reached the service", body)
		}
	}
}

func TestReportAnswersTheServiceVerdicts(t *testing.T) {
	tests := map[string]struct {
		err  error
		want int
	}{
		"себя":         {err: entity.ErrSelfTarget, want: http.StatusBadRequest},
		"нет человека": {err: entity.ErrNotFound, want: http.StatusNotFound},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec := post(newSafetyRouter(&fakeService{err: tt.err}, uuid.New()),
				"/people/"+uuid.NewString()+"/report", `{"reason":"spam"}`)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
		})
	}
}
