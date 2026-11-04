package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// API is the read side of the CLI the Source needs.
type API interface {
	Get(ctx context.Context, path string) (json.RawMessage, error)
}

// Receive takes one Trigger; the engine's Receive fits it.
type Receive func(context.Context, *ticket.Trigger) error

// Bind makes a Ticket follow a pull request Thread; the engine's BindThread fits it.
type Bind func(ctx context.Context, ticketID, threadKey string) error

const (
	// overlap re-reads the last minutes of the previous poll: GitHub's since filters by
	// updated_at, and a comment can land while a poll runs. Re-read events are
	// Duplicates and cost nothing.
	overlap = 2 * time.Minute
	// perPage is GitHub's largest page; a pull request with more comments or check runs
	// than that is not paged in the MVP (the newest page holds what changed).
	perPage = 100
)

// Source polls one repository's Ticket pull requests.
type Source struct {
	API  API
	DB   store.Querier
	Repo string // owner/name
	// Now stamps what GitHub gives no time for: a CI result, a close without closed_at.
	Now func() time.Time
}

func (s *Source) threadKey(n int) string { return fmt.Sprintf("github:%s#%d", s.Repo, n) }

func cursorKey(thread string) string { return "github:cursor:" + thread }

// Sync binds the pull requests of open Tickets' branches, then reads every open pull
// request a Ticket follows.
func (s *Source) Sync(ctx context.Context, bind Bind, receive Receive) error {
	open, err := store.OpenTickets(ctx, s.DB)
	if err != nil {
		return err
	}
	var errs []error
	for _, t := range open {
		if t.Execution != nil && len(s.pulls(t)) == 0 {
			if err := s.bindBranch(ctx, t, bind); err != nil {
				errs = append(errs, fmt.Errorf("ticket %s: %w", t.ID, err))
			}
		}
	}
	if open, err = store.OpenTickets(ctx, s.DB); err != nil {
		return err
	}
	seen := map[int]bool{}
	for _, t := range open {
		for _, n := range s.pulls(t) {
			if seen[n] {
				continue
			}
			seen[n] = true
			if err := s.syncPull(ctx, n, receive); err != nil {
				errs = append(errs, fmt.Errorf("pull %d: %w", n, err))
			}
		}
	}
	return errors.Join(errs...)
}

// pulls returns the numbers of this repository's open pull requests a Ticket opened.
func (s *Source) pulls(t *ticket.Ticket) []int {
	var out []int
	prefix := "github:" + s.Repo + "#"
	for _, f := range t.Follows {
		var n int
		if f.Own && f.Closed == nil && strings.HasPrefix(f.Thread, prefix) {
			if _, err := fmt.Sscan(f.Thread[len(prefix):], &n); err == nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// bindBranch finds the pull request of the Ticket's branch. Only pull requests from the
// repository itself match (head=owner:branch); a fork's pull request with the same
// branch name does not (measured on cli/cli, 2026-11-04).
func (s *Source) bindBranch(ctx context.Context, t *ticket.Ticket, bind Bind) error {
	owner, _, _ := strings.Cut(s.Repo, "/")
	raw, err := s.API.Get(ctx, fmt.Sprintf("repos/%s/pulls?head=%s:agent/%s&state=all&per_page=5", s.Repo, owner, t.Execution.Name))
	if err != nil {
		return err
	}
	var prs []pull
	if err := json.Unmarshal(raw, &prs); err != nil {
		return err
	}
	if len(prs) == 0 {
		return nil
	}
	return bind(ctx, t.ID, s.threadKey(prs[0].Number))
}

type user struct {
	Login string `json:"login"`
	Type  string `json:"type"` // User | Bot | Organization
}

type pull struct {
	Number   int        `json:"number"`
	State    string     `json:"state"`
	Merged   bool       `json:"merged"`
	ClosedAt *time.Time `json:"closed_at"`
	MergedBy *user      `json:"merged_by"`
	HTMLURL  string     `json:"html_url"`
	Head     struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

type comment struct {
	ID        int64     `json:"id"`
	User      user      `json:"user"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	HTMLURL   string    `json:"html_url"`
}

type review struct {
	ID          int64     `json:"id"`
	User        user      `json:"user"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
	HTMLURL     string    `json:"html_url"`
}

type reviewComment struct {
	Path         string `json:"path"`
	Line         *int   `json:"line"`
	OriginalLine *int   `json:"original_line"`
	Body         string `json:"body"`
}

type checkRuns struct {
	Total int `json:"total_count"`
	Runs  []struct {
		Name       string  `json:"name"`
		Status     string  `json:"status"`
		Conclusion *string `json:"conclusion"`
		HTMLURL    string  `json:"html_url"`
	} `json:"check_runs"`
}

func (s *Source) get(ctx context.Context, path string, v any) error {
	raw, err := s.API.Get(ctx, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// syncPull reads one pull request: comments and reviews since its cursor, the CI result
// of its head commit while it is open, and its closing. The cursor moves only after every
// Trigger was received, so a failure re-reads the same events next time.
func (s *Source) syncPull(ctx context.Context, n int, receive Receive) error {
	var pr pull
	if err := s.get(ctx, fmt.Sprintf("repos/%s/pulls/%d", s.Repo, n), &pr); err != nil {
		return err
	}
	key := s.threadKey(n)
	var since time.Time
	if v, ok, err := store.GetKV(ctx, s.DB, cursorKey(key)); err != nil {
		return err
	} else if ok {
		if since, err = time.Parse(time.RFC3339, v); err != nil {
			return fmt.Errorf("cursor %q: %w", v, err)
		}
		since = since.Add(-overlap)
	}
	next := since

	var trs []*ticket.Trigger
	q := fmt.Sprintf("repos/%s/issues/%d/comments?per_page=%d", s.Repo, n, perPage)
	if !since.IsZero() {
		q += "&since=" + since.UTC().Format(time.RFC3339)
	}
	var comments []comment
	if err := s.get(ctx, q, &comments); err != nil {
		return err
	}
	for _, c := range comments {
		trs = append(trs, s.trigger(n, "comment:"+fmt.Sprint(c.ID), ticket.EventComment, c.User, c.CreatedAt, c.Body, c.HTMLURL))
		next = later(next, c.CreatedAt)
	}

	var reviews []review
	if err := s.get(ctx, fmt.Sprintf("repos/%s/pulls/%d/reviews?per_page=%d", s.Repo, n, perPage), &reviews); err != nil {
		return err
	}
	for _, r := range reviews {
		if r.State == "PENDING" || r.SubmittedAt.Before(since) {
			continue
		}
		var lines []reviewComment
		if err := s.get(ctx, fmt.Sprintf("repos/%s/pulls/%d/reviews/%d/comments?per_page=%d", s.Repo, n, r.ID, perPage), &lines); err != nil {
			return err
		}
		trs = append(trs, s.trigger(n, "review:"+fmt.Sprint(r.ID), ticket.EventReview, r.User, r.SubmittedAt, reviewText(r, lines), r.HTMLURL))
		next = later(next, r.SubmittedAt)
	}
	sort.SliceStable(trs, func(i, j int) bool { return trs[i].At.Before(trs[j].At) })

	if pr.State == "open" && pr.Head.SHA != "" {
		tr, err := s.ci(ctx, n, pr)
		if err != nil {
			return err
		}
		if tr != nil {
			trs = append(trs, tr)
		}
	}
	if pr.State == "closed" {
		event, by, at := ticket.EventClosed, user{Login: "github", Type: "Bot"}, s.Now()
		if pr.Merged {
			event = ticket.EventMerged
			if pr.MergedBy != nil {
				by = *pr.MergedBy
			}
		}
		if pr.ClosedAt != nil {
			at = *pr.ClosedAt
		}
		trs = append(trs, s.trigger(n, "closed", event, by, at, "", pr.HTMLURL))
	}

	for _, tr := range trs {
		if err := receive(ctx, tr); err != nil {
			return err
		}
	}
	if next.IsZero() || !next.After(since.Add(overlap)) {
		return nil
	}
	return store.SetKV(ctx, s.DB, cursorKey(key), next.UTC().Format(time.RFC3339))
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// ci summarizes the check runs of the head commit once all have completed: the result and
// links only, never logs (§5.8a). One Trigger per commit and result.
func (s *Source) ci(ctx context.Context, n int, pr pull) (*ticket.Trigger, error) {
	var cr checkRuns
	if err := s.get(ctx, fmt.Sprintf("repos/%s/commits/%s/check-runs?per_page=%d", s.Repo, pr.Head.SHA, perPage), &cr); err != nil {
		return nil, err
	}
	if len(cr.Runs) == 0 {
		return nil, nil
	}
	count := map[string]int{}
	var failed []string
	for _, r := range cr.Runs {
		if r.Status != "completed" || r.Conclusion == nil {
			return nil, nil
		}
		switch c := *r.Conclusion; c {
		case "failure", "timed_out", "cancelled", "action_required", "startup_failure", "stale":
			count["başarısız"]++
			failed = append(failed, fmt.Sprintf("- %s (%s): %s", r.Name, c, r.HTMLURL))
		case "skipped":
			count["atlandı"]++
		default:
			count["başarılı"]++
		}
	}
	event, verdict := ticket.EventCISuccess, "başarılı"
	if len(failed) > 0 {
		event, verdict = ticket.EventCIFailure, "başarısız"
	}
	sha := pr.Head.SHA[:min(7, len(pr.Head.SHA))]
	text := fmt.Sprintf("CI %s — %s: %d başarılı, %d atlandı, %d başarısız", verdict, sha, count["başarılı"], count["atlandı"], count["başarısız"])
	if len(failed) > 0 {
		text += "\n" + strings.Join(failed, "\n")
	}
	tr := s.trigger(n, "ci:"+pr.Head.SHA+":"+event, event, user{Login: "ci", Type: "Bot"}, s.Now(), text, pr.HTMLURL+"/checks")
	return tr, nil
}

// reviewText is a review as one message: its verdict, its body and its line comments.
func reviewText(r review, lines []reviewComment) string {
	var b strings.Builder
	b.WriteString(r.State)
	if body := strings.TrimSpace(r.Body); body != "" {
		b.WriteString("\n\n" + body)
	}
	for _, l := range lines {
		at := l.Path
		if l.Line != nil {
			at += fmt.Sprintf(":%d", *l.Line)
		} else if l.OriginalLine != nil {
			at += fmt.Sprintf(":%d (eski)", *l.OriginalLine)
		}
		b.WriteString("\n\n" + at + "\n" + strings.TrimSpace(l.Body))
	}
	return b.String()
}

func (s *Source) trigger(n int, native, event string, u user, at time.Time, text, url string) *ticket.Trigger {
	thread := ticket.ThreadRef{Source: "github", ID: fmt.Sprintf("%s#%d", s.Repo, n)}
	return &ticket.Trigger{
		ID:      ticket.Key("github", s.Repo, fmt.Sprint(n), native),
		Source:  "github",
		Threads: []ticket.ThreadRef{thread},
		Actor:   ticket.ActorRef{Source: "github", ID: u.Login},
		Human:   u.Type == "User",
		At:      at.UTC(),
		Text:    text,
		Event:   event,
		URL:     url,
		Channel: &ticket.ChannelRef{Source: "github", Thread: thread.Key(),
			Address: map[string]string{"repo": s.Repo, "pr": fmt.Sprint(n)}},
	}
}
