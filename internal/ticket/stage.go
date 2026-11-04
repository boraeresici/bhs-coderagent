package ticket

// StageKind is the high-level execution stage of a Ticket.
type StageKind string

const (
	StagePerson  StageKind = "person"
	StageWorking StageKind = "working"
	StageIdle    StageKind = "idle"
	StageEnded   StageKind = "ended"
)

// Actions for StagePerson and StageWorking.
const (
	ActionDelivery = "delivery"
	ActionAnswer   = "answer"
	ActionReview   = "review"
	ActionBlocked  = "blocked"
	ActionFailed   = "failed"
	ActionQueued   = "queued"
)

// Stage is a Ticket's derived stage: waiting on a person (and for what), working or
// queued, idle between turns, or ended. It is never stored; it is read from the Ticket.
type Stage struct {
	Kind   StageKind `json:"stage"`
	Action string    `json:"action,omitempty"`
}

func (s Stage) String() string {
	if s.Action == "" {
		return string(s.Kind)
	}
	return string(s.Kind) + "/" + s.Action
}

// StageOf classifies a Ticket. The rules follow the stage table (ANALIZ.md §3.4, §5.3;
// upstream docs/003) in order, and the first matching row wins.
func StageOf(t *Ticket) Stage {
	if t.Ended != nil {
		return Stage{Kind: StageEnded}
	}
	if t.Delivery != nil {
		return Stage{StagePerson, ActionDelivery}
	}
	if t.Attention != nil {
		return Stage{StagePerson, ActionAnswer}
	}
	a := t.LatestAssignment()
	var report ReportState
	if a != nil && a.Report != nil {
		report = a.Report.State
	}
	if report == ReportReady {
		return Stage{StagePerson, ActionReview}
	}
	// A person's follow-up after a blocked or failed report returns the Ticket to its worker.
	if (report == ReportBlocked || report == ReportFailed) && t.WaitsForPerson() {
		return Stage{StagePerson, string(report)}
	}
	if a == nil || a.State == AssignmentPending {
		return Stage{StageWorking, ActionQueued}
	}
	if report == ReportWaiting || a.State == AssignmentEnded {
		return Stage{Kind: StageIdle}
	}
	return Stage{Kind: StageWorking}
}

// CanRetry reports whether an open Ticket is stalled on a blocked or failed report with no
// newer human follow-up, or has a failed Input delivery.
func CanRetry(t *Ticket) bool {
	return t.Ended == nil && (t.WaitsForPerson() || t.Delivery != nil)
}
