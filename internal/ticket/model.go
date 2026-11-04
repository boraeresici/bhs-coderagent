// Package ticket is the domain model of the orchestrator: Triggers, Threads, Mandates,
// Tickets and their Assignments, independent of any Source or Executor.
//
// It is a concept-level port of dsh-superagent-prototype src/core/model.ts (see
// UPSTREAM.md). Fields the MVP does not use yet are left out; UPSTREAM.md lists them.
package ticket

import "time"

// ActorRef is an actor identity in the Source's own vocabulary (a Buzz pubkey, a GitHub login).
type ActorRef struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

// ThreadRef is a thread identity; the rule that derives ID from a native event belongs to the Source.
type ThreadRef struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

// Key is the thread's stable key across the whole domain.
func (r ThreadRef) Key() string { return r.Source + ":" + r.ID }

// ChannelRef is a write-only address on a thread; the address format belongs to the Source.
type ChannelRef struct {
	Source  string            `json:"source"`
	Thread  string            `json:"thread"`
	Address map[string]string `json:"address"`
}

// Addressed says how a sender addressed the agent.
type Addressed string

const (
	AddressedNone    Addressed = ""
	AddressedMention Addressed = "mention"
	AddressedDirect  Addressed = "direct"
)

// Trigger is one immutable event produced by a Source. Text and Meta are untrusted external data.
type Trigger struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	// Threads holds every thread the trigger belongs to; Threads[0] is the primary one.
	Threads []ThreadRef `json:"threads"`
	Actor   ActorRef    `json:"actor"`
	// Human is Source-declared: a person wrote it, not a bot, CI or this agent.
	Human     bool        `json:"human"`
	Addressed Addressed   `json:"addressed,omitempty"`
	At        time.Time   `json:"at"`
	Text      string      `json:"text"`
	Channel   *ChannelRef `json:"channel,omitempty"`
	URL       string      `json:"url,omitempty"`
	// Meta holds Source-native details for adapters; the core never interprets them.
	Meta map[string]any `json:"meta,omitempty"`
}

// Strength of a Mandate: an explicit request, or work the agent inferred.
type Strength string

const (
	Explicit Strength = "explicit"
	Inferred Strength = "inferred"
)

// MandateBasis records how the gate admitted the Trigger a Mandate came from, for
// re-validation and audit. The MVP gate only produces Via "addressed".
type MandateBasis struct {
	Via       string    `json:"via"`
	Addressed Addressed `json:"addressed,omitempty"`
	// Scope is the receiving scope as the Source recorded it (for Buzz: the channel).
	Scope  string   `json:"scope,omitempty"`
	Sender string   `json:"sender,omitempty"`
	Rules  []string `json:"rules,omitempty"`
}

// Mandate is the immutable authorization an accept produces; execution is bounded by it.
// A wider scope means a new Ticket, never a changed Mandate.
type Mandate struct {
	Strength      Strength     `json:"strength"`
	PolicyVersion string       `json:"policyVersion"`
	Basis         MandateBasis `json:"basis"`
	Actor         ActorRef     `json:"actor"`
	Scope         string       `json:"scope"`
}

// AssignmentState is the lifecycle of one Executor instance working a Ticket.
type AssignmentState string

const (
	AssignmentPending AssignmentState = "pending"
	AssignmentRunning AssignmentState = "running"
	AssignmentEnded   AssignmentState = "ended"
)

// ReportState is what an Assignment reports: work in progress, waiting for a Trigger,
// a ready result, a person must act (blocked, failed), or nothing left to do (dropped).
type ReportState string

const (
	ReportProgress ReportState = "progress"
	ReportWaiting  ReportState = "waiting"
	ReportReady    ReportState = "ready"
	ReportBlocked  ReportState = "blocked"
	ReportFailed   ReportState = "failed"
	ReportDropped  ReportState = "dropped"
)

// Report is the latest structured status an Assignment's worker gave.
type Report struct {
	State   ReportState `json:"state"`
	Summary string      `json:"summary"`
	At      time.Time   `json:"at"`
}

// Assignment binds a Ticket to one Executor instance; its state is independent of the Ticket's.
type Assignment struct {
	ID          string          `json:"id"`
	State       AssignmentState `json:"state"`
	CreatedAt   time.Time       `json:"createdAt"`
	StartedAt   *time.Time      `json:"startedAt,omitempty"`
	EndedAt     *time.Time      `json:"endedAt,omitempty"`
	EndedReason string          `json:"endedReason,omitempty"`
	Report      *Report         `json:"report,omitempty"`
	// DeliveredAt is when Inputs last reached the worker.
	DeliveredAt *time.Time `json:"deliveredAt,omitempty"`
}

// InputKind discriminates TicketInput.
type InputKind string

const (
	// InputTrigger is a Trigger on one of the Ticket's Threads; Human when a person wrote it.
	InputTrigger InputKind = "trigger"
	// InputResumed records that a person's message resumed the ended Ticket under its unchanged Mandate.
	InputResumed InputKind = "resumed"
	InputText    InputKind = "text"
	// InputNote is context from other work: it asks for nothing and grants nothing.
	InputNote InputKind = "note"
)

// TicketInput is something that reached a Ticket for its worker to read.
type TicketInput struct {
	ID        string    `json:"id"`
	Kind      InputKind `json:"kind"`
	At        time.Time `json:"at"`
	TriggerID string    `json:"triggerId,omitempty"`
	Human     bool      `json:"human,omitempty"`
	// Context marks a Trigger that reached the Ticket only on a Thread followed for context.
	Context     bool   `json:"context,omitempty"`
	Text        string `json:"text,omitempty"`
	DeliveredTo string `json:"deliveredTo,omitempty"`
}

// EndBy says who ended a Ticket.
type EndBy string

const (
	EndByExecutor EndBy = "executor"
	EndByHuman    EndBy = "human"
)

// Outcome says how a Ticket ended.
type Outcome string

const (
	OutcomeCompleted Outcome = "completed"
	OutcomeMerged    Outcome = "merged"
	OutcomeClosed    Outcome = "closed"
	OutcomeDropped   Outcome = "dropped"
	OutcomeStopped   Outcome = "stopped"
)

// End records when, by whom and how a Ticket ended.
type End struct {
	At      time.Time `json:"at"`
	By      EndBy     `json:"by"`
	Outcome Outcome   `json:"outcome"`
	Reason  string    `json:"reason,omitempty"`
}

// Attention is an open question for a person; TriggerID is the message that raised it.
type Attention struct {
	ID        string `json:"id"`
	Question  string `json:"question"`
	TriggerID string `json:"triggerId,omitempty"`
}

// Follow is a Thread a Ticket follows, such as its pull request; Own when the Ticket opened it.
type Follow struct {
	Thread string        `json:"thread"`
	Own    bool          `json:"own"`
	Closed *FollowClosed `json:"closed,omitempty"`
}

// FollowClosed records how a followed Thread closed.
type FollowClosed struct {
	At      time.Time `json:"at"`
	Outcome Outcome   `json:"outcome"` // merged | closed
}

// Execution is what the Executor works in: the Ticket's short name (which names its
// branch agent/<Name>), its directory and the assignment text.
type Execution struct {
	Name       string `json:"name"`
	Directory  string `json:"directory"`
	Assignment string `json:"assignment"`
}

// DeliveryFailure is the latest failed delivery of an Input, kept until one succeeds.
type DeliveryFailure struct {
	Error string    `json:"error"`
	At    time.Time `json:"at"`
}

// Origin names the Triggers a Ticket was accepted from.
type Origin struct {
	TriggerID  string   `json:"triggerId"`
	TriggerIDs []string `json:"triggerIds"`
}

// Ticket is an authorized unit of work. Only the gate (Eval) creates one; it tracks its
// own state and never follows the Executor's turns.
type Ticket struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	DecisionID string       `json:"decisionId"`
	Mandate    Mandate      `json:"mandate"`
	Channels   []ChannelRef `json:"channels"`
	// Ended is nil while the Ticket is open.
	Ended       *End          `json:"ended,omitempty"`
	Attention   *Attention    `json:"attention,omitempty"`
	Assignments []Assignment  `json:"assignments"`
	Inputs      []TicketInput `json:"inputs"`
	// Undelivered lists Input IDs not yet handed to a running Assignment.
	Undelivered []string         `json:"undelivered"`
	Follows     []Follow         `json:"follows,omitempty"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	Origin      Origin           `json:"origin"`
	Execution   *Execution       `json:"execution,omitempty"`
	Delivery    *DeliveryFailure `json:"delivery,omitempty"`
}

// Live reports whether the Ticket is open.
func (t *Ticket) Live() bool { return t.Ended == nil }

// LatestAssignment returns the newest Assignment, or nil.
func (t *Ticket) LatestAssignment() *Assignment {
	if len(t.Assignments) == 0 {
		return nil
	}
	return &t.Assignments[len(t.Assignments)-1]
}

// RunningAssignment returns the running Assignment, or nil.
func (t *Ticket) RunningAssignment() *Assignment {
	for i := range t.Assignments {
		if t.Assignments[i].State == AssignmentRunning {
			return &t.Assignments[i]
		}
	}
	return nil
}

// WaitsForPerson reports whether the latest Assignment reported blocked or failed and no
// person's message reached the Ticket since. A later report, a person's message, or a
// retry (which starts a new Assignment) lifts it.
func (t *Ticket) WaitsForPerson() bool {
	a := t.LatestAssignment()
	if a == nil || a.Report == nil {
		return false
	}
	r := a.Report
	if r.State != ReportBlocked && r.State != ReportFailed {
		return false
	}
	for _, in := range t.Inputs {
		if in.Kind == InputTrigger && in.Human && !in.Context && in.At.After(r.At) {
			return false
		}
	}
	return true
}
