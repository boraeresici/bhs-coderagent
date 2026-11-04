package github

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

var ctx = context.Background()

const (
	merged = 14804 // cli/cli: merged; bot comment, bot review, review with a line comment, approval
	open   = 14927 // cli/cli: open draft; bot review, CI with one failed run
)

// fakeAPI answers recorded `gh api` responses (testdata, read-only from cli/cli on
// 2026-11-04) and records what was asked. An unknown path is an error.
type fakeAPI struct {
	files map[string]string
	asked []string
}

func (f *fakeAPI) Get(_ context.Context, path string) (json.RawMessage, error) {
	f.asked = append(f.asked, path)
	base, _, _ := strings.Cut(path, "&since=")
	name, ok := f.files[base]
	if !ok {
		return nil, &Error{Status: 404, Message: "Not Found: " + path}
	}
	return os.ReadFile("testdata/" + name)
}

func recorded() *fakeAPI {
	const r = "repos/cli/cli/"
	return &fakeAPI{files: map[string]string{
		r + "pulls/14804":                                                              "pull.json",
		r + "issues/14804/comments?per_page=100":                                       "issue_comments.json",
		r + "pulls/14804/reviews?per_page=100":                                         "reviews.json",
		r + "pulls/14804/reviews/5632655130/comments?per_page=100":                     "review_5632655130_comments.json",
		r + "pulls/14804/reviews/5632961408/comments?per_page=100":                     "review_5632961408_comments.json",
		r + "pulls/14804/reviews/5633437083/comments?per_page=100":                     "review_5633437083_comments.json",
		r + "pulls/14927":                                                              "pull_open.json",
		r + "issues/14927/comments?per_page=100":                                       "open_issue_comments.json",
		r + "pulls/14927/reviews?per_page=100":                                         "open_reviews.json",
		r + "pulls/14927/reviews/5759718474/comments?per_page=100":                     "open_review_5759718474_comments.json",
		r + "commits/d43c4e3ad3c34332c803a568f85f116e3ee58f19/check-runs?per_page=100": "open_check_runs.json",
		r + "pulls?head=cli:agent/gh-help-env-1a2b3c&state=all&per_page=5":             "pulls_by_head.json",
		r + "pulls?head=cli:agent/yok-boyle-dal&state=all&per_page=5":                  "pulls_by_head_empty.json",
	}}
}

type fixture struct {
	t   *testing.T
	st  *store.Store
	api *fakeAPI
	src *Source
	got []*ticket.Trigger
}

func setup(t *testing.T, tickets ...*ticket.Ticket) *fixture {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, tk := range tickets {
		if err := store.PutTicket(ctx, st.DB(), tk); err != nil {
			t.Fatal(err)
		}
	}
	api := recorded()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	return &fixture{t: t, st: st, api: api, src: &Source{API: api, DB: st.DB(), Repo: "cli/cli", Now: func() time.Time { return now }}}
}

func following(id string, pr int) *ticket.Ticket {
	return &ticket.Ticket{ID: id, Execution: &ticket.Execution{Name: id},
		Follows: []ticket.Follow{{Thread: "github:cli/cli#" + itoa(pr), Own: true}}}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func (f *fixture) sync(bind Bind) error {
	if bind == nil {
		bind = func(context.Context, string, string) error { f.t.Fatal("unexpected bind"); return nil }
	}
	return f.src.Sync(ctx, bind, func(_ context.Context, tr *ticket.Trigger) error {
		f.got = append(f.got, tr)
		return nil
	})
}

func TestMergedPullRequest(t *testing.T) {
	f := setup(t, following("t1", merged))
	if err := f.sync(nil); err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, tr := range f.got {
		seen = append(seen, tr.Event+":"+tr.Actor.ID)
		if tr.Source != "github" || tr.Threads[0].Key() != "github:cli/cli#14804" || tr.Channel.Address["repo"] != "cli/cli" {
			t.Errorf("trigger address = %+v", tr)
		}
	}
	want := []string{
		"review:copilot-pull-request-reviewer[bot]", // 18:16
		"comment:github-actions[bot]",               // 18:32
		"review:williammartin",                      // 18:45
		"review:BagToad",                            // 19:27
		"merged:BagToad",                            // closing comes last
	}
	if strings.Join(seen, " ") != strings.Join(want, " ") {
		t.Fatalf("triggers =\n%s\nwant\n%s", strings.Join(seen, "\n"), strings.Join(want, "\n"))
	}
	if f.got[0].Human || f.got[1].Human || !f.got[2].Human {
		t.Error("Human must follow the GitHub user type: bots are not people")
	}
	if !strings.HasPrefix(f.got[2].Text, "COMMENTED") || !strings.Contains(f.got[2].Text, "internal/attachments/client.go") {
		t.Errorf("a review carries its line comments:\n%s", f.got[2].Text)
	}
	if f.got[4].URL != "https://github.com/cli/cli/pull/14804" {
		t.Errorf("closing url = %s", f.got[4].URL)
	}
	for _, p := range f.api.asked {
		if strings.Contains(p, "check-runs") {
			t.Error("a closed pull request's CI is not read")
		}
	}
}

func TestSecondSyncSameIDsAndCursor(t *testing.T) {
	f := setup(t, following("t1", merged))
	f.sync(nil)
	first := map[string]bool{}
	for _, tr := range f.got {
		first[tr.ID] = true
	}
	f.got = nil
	f.sync(nil)
	for _, tr := range f.got {
		if !first[tr.ID] {
			t.Errorf("a re-read event must keep its id: %s %s", tr.Event, tr.ID)
		}
	}
	v, ok, _ := store.GetKV(ctx, f.st.DB(), "github:cursor:github:cli/cli#14804")
	if !ok || v != "2026-10-23T19:27:08Z" {
		t.Errorf("cursor = %q, want the newest comment or review", v)
	}
	if !strings.Contains(strings.Join(f.api.asked, " "), "comments?per_page=100&since=2026-10-23T19:25:08Z") {
		t.Errorf("the second read must ask since the cursor minus the overlap: %v", f.api.asked)
	}
}

func TestOpenPullRequestCI(t *testing.T) {
	f := setup(t, following("t1", open))
	if err := f.sync(nil); err != nil {
		t.Fatal(err)
	}
	last := f.got[len(f.got)-1]
	if last.Event != ticket.EventCIFailure || last.Actor.ID != "ci" || last.Human {
		t.Fatalf("ci trigger = %+v", last)
	}
	if !strings.HasPrefix(last.Text, "CI başarısız — d43c4e3: 12 başarılı, 13 atlandı, 1 başarısız\n- ") ||
		!strings.Contains(last.Text, "https://github.com/cli/cli/actions/") || strings.Count(last.Text, "\n") != 1 {
		t.Errorf("CI text is the result and failed run links only:\n%s", last.Text)
	}
	if last.URL != "https://github.com/cli/cli/pull/14927/checks" {
		t.Errorf("url = %s", last.URL)
	}
}

func TestBindsBranchPullRequest(t *testing.T) {
	tk := &ticket.Ticket{ID: "t1", Execution: &ticket.Execution{Name: "gh-help-env-1a2b3c"}}
	none := &ticket.Ticket{ID: "t2", Execution: &ticket.Execution{Name: "yok-boyle-dal"}}
	f := setup(t, tk, none)
	var bound []string
	f.sync(func(_ context.Context, id, thread string) error {
		bound = append(bound, id+"→"+thread)
		return nil
	})
	if len(bound) != 1 || bound[0] != "t1→github:cli/cli#14918" {
		t.Fatalf("bound = %v", bound)
	}
}

func TestFailedReadKeepsCursor(t *testing.T) {
	f := setup(t, following("t1", merged))
	delete(f.api.files, "repos/cli/cli/pulls/14804/reviews?per_page=100")
	err := f.sync(nil)
	var gh *Error
	if !errors.As(err, &gh) || gh.Status != 404 {
		t.Fatalf("err = %v", err)
	}
	if _, ok, _ := store.GetKV(ctx, f.st.DB(), "github:cursor:github:cli/cli#14804"); ok {
		t.Error("the cursor must not move past events that were not received")
	}
}

func TestClosedFollowIsNotRead(t *testing.T) {
	tk := following("t1", merged)
	tk.Follows[0].Closed = &ticket.FollowClosed{Outcome: ticket.OutcomeMerged}
	f := setup(t, tk)
	f.sync(func(context.Context, string, string) error { return nil })
	for _, p := range f.api.asked {
		if strings.Contains(p, "14804") {
			t.Fatalf("a closed pull request is not polled: %v", f.api.asked)
		}
	}
}

func TestCLIError(t *testing.T) {
	c := &CLI{Runner: runner(func([]string) ([]byte, []byte, int) {
		return []byte(`{"message":"Not Found"}`), []byte("gh: Not Found (HTTP 404)\n"), 1
	})}
	_, err := c.Get(ctx, "repos/o/r/pulls/1")
	var gh *Error
	if !errors.As(err, &gh) || gh.Status != 404 || gh.Message != "Not Found (HTTP 404)" {
		t.Fatalf("err = %#v", err)
	}
	if _, err := c.Get(ctx, "--method=DELETE"); err == nil {
		t.Error("a path that reads as a flag must be refused")
	}
}

type runner func([]string) ([]byte, []byte, int)

func (r runner) Run(_ context.Context, args []string) ([]byte, []byte, int, error) {
	o, e, c := r(args)
	return o, e, c, nil
}
