package ticket

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

func at(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

func assignment(state AssignmentState, report ReportState, reportAt int) Assignment {
	a := Assignment{ID: "a", State: state, CreatedAt: t0}
	if report != "" {
		a.Report = &Report{State: report, At: at(reportAt)}
	}
	return a
}

func humanInput(min int) TicketInput {
	return TicketInput{ID: "i", Kind: InputTrigger, TriggerID: "tr", Human: true, At: at(min)}
}

func TestStageOf(t *testing.T) {
	cases := []struct {
		name string
		t    Ticket
		want string
	}{
		{"ended wins over everything",
			Ticket{Ended: &End{Outcome: OutcomeMerged}, Delivery: &DeliveryFailure{}, Attention: &Attention{}},
			"ended"},
		{"failed delivery before attention",
			Ticket{Delivery: &DeliveryFailure{}, Attention: &Attention{}},
			"person/delivery"},
		{"attention before a ready report",
			Ticket{Attention: &Attention{}, Assignments: []Assignment{assignment(AssignmentRunning, ReportReady, 1)}},
			"person/answer"},
		{"ready report waits for review",
			Ticket{Assignments: []Assignment{assignment(AssignmentEnded, ReportReady, 1)}},
			"person/review"},
		{"blocked waits for a person",
			Ticket{Assignments: []Assignment{assignment(AssignmentEnded, ReportBlocked, 1)}},
			"person/blocked"},
		{"failed waits for a person",
			Ticket{Assignments: []Assignment{assignment(AssignmentEnded, ReportFailed, 1)}},
			"person/failed"},
		{"a later human message lifts blocked",
			Ticket{Assignments: []Assignment{assignment(AssignmentRunning, ReportBlocked, 1)}, Inputs: []TicketInput{humanInput(2)}},
			"working"},
		{"an earlier human message does not lift blocked",
			Ticket{Assignments: []Assignment{assignment(AssignmentEnded, ReportBlocked, 2)}, Inputs: []TicketInput{humanInput(1)}},
			"person/blocked"},
		{"no assignment is queued",
			Ticket{},
			"working/queued"},
		{"pending assignment is queued",
			Ticket{Assignments: []Assignment{assignment(AssignmentPending, "", 0)}},
			"working/queued"},
		{"waiting report is idle",
			Ticket{Assignments: []Assignment{assignment(AssignmentRunning, ReportWaiting, 1)}},
			"idle"},
		{"ended assignment without report is idle",
			Ticket{Assignments: []Assignment{assignment(AssignmentEnded, "", 0)}},
			"idle"},
		{"running assignment is working",
			Ticket{Assignments: []Assignment{assignment(AssignmentRunning, ReportProgress, 1)}},
			"working"},
		{"only the latest assignment counts",
			Ticket{Assignments: []Assignment{assignment(AssignmentEnded, ReportReady, 1), assignment(AssignmentPending, "", 0)}},
			"working/queued"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StageOf(&c.t).String(); got != c.want {
				t.Errorf("StageOf = %s, want %s", got, c.want)
			}
		})
	}
}

func TestWaitsForPersonIgnoresContextAndBots(t *testing.T) {
	tk := Ticket{
		Assignments: []Assignment{assignment(AssignmentEnded, ReportFailed, 1)},
		Inputs: []TicketInput{
			{Kind: InputTrigger, Human: false, At: at(2)},               // bot or CI
			{Kind: InputTrigger, Human: true, Context: true, At: at(3)}, // context-only thread
			{Kind: InputNote, At: at(4)},
		},
	}
	if !tk.WaitsForPerson() {
		t.Fatal("bot, context and note inputs must not lift a failed report")
	}
}

func TestCanRetry(t *testing.T) {
	cases := []struct {
		name string
		t    Ticket
		want bool
	}{
		{"blocked", Ticket{Assignments: []Assignment{assignment(AssignmentEnded, ReportBlocked, 1)}}, true},
		{"failed delivery", Ticket{Delivery: &DeliveryFailure{}}, true},
		{"working", Ticket{Assignments: []Assignment{assignment(AssignmentRunning, ReportProgress, 1)}}, false},
		{"ended", Ticket{Ended: &End{}, Delivery: &DeliveryFailure{}}, false},
	}
	for _, c := range cases {
		if got := CanRetry(&c.t); got != c.want {
			t.Errorf("%s: CanRetry = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestReasonText(t *testing.T) {
	got := ReasonText(
		Reason{Code: "startedTicket", Values: map[string]string{"ticket": "T1"}},
		Reason{Code: "custom", TriggerID: "tr"},
		Reason{Code: "startedTicket"},
	)
	want := "Already started ticket T1; tr: custom; Already started ticket {ticket}"
	if got != want {
		t.Errorf("ReasonText = %q, want %q", got, want)
	}
}
