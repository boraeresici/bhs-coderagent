package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/engine"
	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

func setup(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	t0 := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	ready := ticket.Assignment{ID: "a", State: ticket.AssignmentEnded, Report: &ticket.Report{State: ticket.ReportReady, At: t0}}
	blocked := ticket.Assignment{ID: "a", State: ticket.AssignmentEnded, Report: &ticket.Report{State: ticket.ReportBlocked, At: t0}}
	for _, tk := range []*ticket.Ticket{
		{ID: "T1", Title: "queued", CreatedAt: t0, Execution: &ticket.Execution{Name: "queued-aaaaaa"}},
		{ID: "T2", Title: "blocked", CreatedAt: t0.Add(time.Minute), Assignments: []ticket.Assignment{blocked}},
		{ID: "T3", Title: "review", CreatedAt: t0.Add(2 * time.Minute), Assignments: []ticket.Assignment{ready},
			Follows: []ticket.Follow{{Thread: "github:o/r#1", Own: true}}},
		{ID: "T4", Title: "done", CreatedAt: t0, Ended: &ticket.End{Outcome: ticket.OutcomeCompleted}},
	} {
		if err := store.PutTicket(context.Background(), st.DB(), tk); err != nil {
			t.Fatal(err)
		}
	}
	h, err := New(st, &fakeActions{}, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(h http.Handler, path, token string) *httptest.ResponseRecorder {
	return call(h, "GET", path, token)
}

func call(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestNeedsToken(t *testing.T) {
	if _, err := New(nil, nil, ""); err == nil {
		t.Fatal("an API without a token must not start")
	}
	h := setup(t)
	for _, tok := range []string{"", "wrong", "s3cre"} {
		if w := get(h, Prefix+"state", tok); w.Code != http.StatusUnauthorized {
			t.Errorf("token %q: %d", tok, w.Code)
		}
	}
}

func TestState(t *testing.T) {
	w := get(setup(t), Prefix+"state", "s3cret")
	var got struct {
		Open        int
		NeedsPerson int `json:"needs_person"`
		Stages      map[string]int
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || got.NeedsPerson != 2 || got.Stages["working/queued"] != 1 || got.Stages["person/review"] != 1 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestPersonFilterOrdersByAction(t *testing.T) {
	w := get(setup(t), Prefix+"tickets?filter=person", "s3cret")
	var got struct{ Tickets []TicketView }
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Tickets) != 2 || got.Tickets[0].ID != "T3" || got.Tickets[1].ID != "T2" {
		t.Fatalf("review must come before blocked: %s", w.Body)
	}
	if got.Tickets[1].Stage != "person/blocked" || !got.Tickets[1].CanRetry {
		t.Errorf("blocked view = %+v", got.Tickets[1])
	}
	if w := get(setup(t), Prefix+"tickets?filter=all", "s3cret"); w.Code != 400 {
		t.Errorf("unknown filter = %d", w.Code)
	}
}

func TestTicket(t *testing.T) {
	h := setup(t)
	w := get(h, Prefix+"tickets/T1", "s3cret")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"branch":"agent/queued-aaaaaa"`) || !strings.Contains(w.Body.String(), `"stage":"working/queued"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := get(h, Prefix+"tickets/T4", "s3cret"); !strings.Contains(w.Body.String(), `"stage":"ended"`) {
		t.Errorf("ended ticket readable by id: %s", w.Body)
	}
	if w := get(h, Prefix+"tickets/nope", "s3cret"); w.Code != 404 {
		t.Errorf("missing = %d", w.Code)
	}
}

// fakeActions answers like the engine: T2 (blocked) may be retried, T4 (ended) not
// stopped, an unknown id is not found.
type fakeActions struct {
	paused bool
	did    []string
}

func (f *fakeActions) act(name, id string) error {
	switch id {
	case "T1", "T2", "T3":
	case "T4":
		return engine.ErrConflict
	default:
		return store.ErrNotFound
	}
	if name == "retry" && id != "T2" {
		return engine.ErrConflict
	}
	f.did = append(f.did, name+":"+id)
	return nil
}
func (f *fakeActions) Retry(_ context.Context, id string) error { return f.act("retry", id) }
func (f *fakeActions) Stop(_ context.Context, id string) error  { return f.act("stop", id) }
func (f *fakeActions) SetPaused(_ context.Context, p bool) error {
	f.paused = p
	return nil
}
func (f *fakeActions) Paused(context.Context) (bool, error) { return f.paused, nil }

func TestActions(t *testing.T) {
	h := setup(t)
	for _, c := range []struct {
		method, path, token string
		code                int
	}{
		{"POST", "tickets/T2/stop", "", 401},
		{"GET", "tickets/T2/retry", "s3cret", 405},
		{"POST", "tickets/T2/retry", "s3cret", 200},
		{"POST", "tickets/T1/retry", "s3cret", 409},
		{"POST", "tickets/T1/stop", "s3cret", 200},
		{"POST", "tickets/T4/stop", "s3cret", 409},
		{"POST", "tickets/nope/stop", "s3cret", 404},
		{"POST", "pause", "s3cret", 200},
	} {
		if w := call(h, c.method, Prefix+c.path, c.token); w.Code != c.code {
			t.Errorf("%s %s: %d %s, want %d", c.method, c.path, w.Code, w.Body, c.code)
		}
	}
	if w := get(h, Prefix+"state", "s3cret"); !strings.Contains(w.Body.String(), `"paused":true`) {
		t.Errorf("state after pause: %s", w.Body)
	}
	if w := call(h, "POST", Prefix+"tickets/T2/retry", "s3cret"); !strings.Contains(w.Body.String(), `"id":"T2"`) {
		t.Errorf("an action answers with the Ticket: %s", w.Body)
	}
}
