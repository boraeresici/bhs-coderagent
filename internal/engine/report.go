package engine

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// Report is what the coder states in the block at the end of its message (ANALIZ.md
// §5.6, option A).
type Report struct {
	State       ticket.ReportState `yaml:"state"`
	Summary     string             `yaml:"summary"`
	PR          int                `yaml:"pr"`
	HumanAction string             `yaml:"human_action"`
}

var reportBlock = regexp.MustCompile("(?s)```coderagent-report[ \\t]*\\n(.*?)\\n?```")

// ParseReport reads the last report block of a message. A message without a valid block
// reports nothing: ok is false and the caller treats the turn as waiting (§5.6).
func ParseReport(text string) (Report, bool) {
	blocks := reportBlock.FindAllStringSubmatch(text, -1)
	if len(blocks) == 0 {
		return Report{}, false
	}
	var r Report
	if err := yaml.Unmarshal([]byte(blocks[len(blocks)-1][1]), &r); err != nil {
		return Report{}, false
	}
	switch r.State {
	case ticket.ReportProgress, ticket.ReportWaiting, ticket.ReportReady,
		ticket.ReportBlocked, ticket.ReportFailed, ticket.ReportDropped:
	default:
		return Report{}, false
	}
	if r.PR < 0 {
		r.PR = 0
	}
	r.Summary = strings.TrimSpace(r.Summary)
	r.HumanAction = strings.TrimSpace(r.HumanAction)
	return r, true
}

// applyReport records a report on the running Assignment. The engine calls it only for
// a message the coder signed on the Ticket's own thread, so a report block anyone else
// writes, or the coder writes elsewhere, changes nothing (A10).
//
// ready without a pull request completes the Ticket; ready with one waits for review
// until the pull request closes (F5). dropped ends it without a message. blocked and
// failed wait for a person; they end the Assignment, and a person's message or a retry
// starts the next one. waiting keeps the Assignment, but frees the coder for other work.
func (e *Engine) applyReport(t *ticket.Ticket, tr *ticket.Trigger, now time.Time) bool {
	a := t.RunningAssignment()
	if a == nil {
		return false
	}
	r, ok := ParseReport(tr.Text)
	if !ok {
		r = Report{State: ticket.ReportWaiting, Summary: "rapor bloğu yok"}
	}
	a.Report = &ticket.Report{State: r.State, Summary: r.Summary, At: tr.At}
	t.Attention = nil
	if r.State == ticket.ReportWaiting && r.HumanAction != "" {
		t.Attention = &ticket.Attention{ID: ticket.Key("attention", tr.ID), Question: r.HumanAction, TriggerID: tr.ID}
	}
	if r.PR > 0 {
		follow := "github:" + e.repo.GitHub + "#" + strconv.Itoa(r.PR)
		known := false
		for _, f := range t.Follows {
			known = known || f.Thread == follow
		}
		if !known {
			t.Follows = append(t.Follows, ticket.Follow{Thread: follow, Own: true})
		}
	}
	switch r.State {
	case ticket.ReportReady, ticket.ReportBlocked, ticket.ReportFailed, ticket.ReportDropped:
		a.State, a.EndedAt, a.EndedReason = ticket.AssignmentEnded, &now, "report "+string(r.State)
	}
	switch {
	case r.State == ticket.ReportDropped:
		t.Ended = &ticket.End{At: now, By: ticket.EndByExecutor, Outcome: ticket.OutcomeDropped}
	case r.State == ticket.ReportReady && len(t.Follows) == 0:
		t.Ended = &ticket.End{At: now, By: ticket.EndByExecutor, Outcome: ticket.OutcomeCompleted}
	}
	t.UpdatedAt = now
	return true
}
