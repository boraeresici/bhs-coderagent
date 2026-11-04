// Package engine is the single writer of Triggers, Threads, Decisions and Tickets
// (ANALIZ.md §3.8, upstream src/core/engine.ts). Every method runs in one store
// transaction, together with the effects and outbox messages it decides on, so a crash
// leaves either all of a decision or none of it.
//
// The MVP Eval is the deterministic gate alone (ANALIZ.md §10): a person on a channel
// the Source listens to who mentions the agent makes an explicit request. There is no
// model assessment and no inferred work.
package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/effects"
	"github.com/boraeresici/bhs-coderagent/internal/outbox"
	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// PolicyVersion names the gate's rules; a Mandate records it.
const PolicyVersion = "gate-v1"

// Config is what the gate checks.
type Config struct {
	// Self is the orchestrator's own actor id; its messages never become Triggers.
	Self string
	// Channels are the receiving scopes the Source listens to (Buzz channel UUIDs).
	Channels []string
}

// Engine decides what each Trigger does.
type Engine struct {
	st       *store.Store
	effects  *effects.Queue
	now      func() time.Time
	self     string
	channels map[string]bool
}

// New returns an Engine over an open store.
func New(st *store.Store, q *effects.Queue, now func() time.Time, cfg Config) *Engine {
	ch := map[string]bool{}
	for _, c := range cfg.Channels {
		ch[c] = true
	}
	return &Engine{st: st, effects: q, now: now, self: cfg.Self, channels: ch}
}

// Outcome is what receiving a Trigger did.
type Outcome string

const (
	// Duplicate: the Trigger was received before; nothing changed.
	Duplicate Outcome = "duplicate"
	// Filtered: outside the Source's scope; recorded, never decided.
	Filtered Outcome = "filtered"
	// FannedOut: it reached Tickets on its Thread as Input (a follow-up or a resume).
	FannedOut Outcome = "fanout"
	// Accepted: the gate started a Ticket.
	Accepted Outcome = "accepted"
	// Ignored: the gate admitted nothing.
	Ignored Outcome = "ignored"
)

// Result reports what Receive did.
type Result struct {
	Outcome  Outcome
	Reason   *ticket.Reason
	TicketID string   // Accepted
	Fanout   []string // FannedOut
}

// Scope returns the receiving scope a Trigger came in on: its Channel's "channel".
func Scope(tr *ticket.Trigger) string {
	if tr.Channel == nil {
		return ""
	}
	return tr.Channel.Address["channel"]
}

// Receive stores a Trigger and decides it. Receiving the same Trigger twice is harmless:
// the second time is a Duplicate and changes nothing, so accepting a Trigger twice
// creates one Ticket.
func (e *Engine) Receive(ctx context.Context, tr *ticket.Trigger) (Result, error) {
	if len(tr.Threads) == 0 || tr.ID == "" {
		return Result{}, errors.New("engine: a Trigger needs an id and a thread")
	}
	var res Result
	err := e.st.Tx(ctx, func(tx *sql.Tx) error {
		stored, err := store.PutTrigger(ctx, tx, tr)
		if err != nil {
			return err
		}
		if stored == store.TriggerDuplicate {
			res = Result{Outcome: Duplicate}
			return nil
		}
		now := e.now()
		if reason := e.filter(tr); reason != nil {
			res = Result{Outcome: Filtered, Reason: reason}
			return store.SetDisposition(ctx, tx, tr.ID, &ticket.Disposition{Filtered: reason, At: now})
		}
		thread, err := store.GetThread(ctx, tx, tr.Threads[0].Key())
		if err != nil {
			return err
		}
		reached, err := e.fanout(ctx, tx, tr, thread, now)
		if err != nil {
			return err
		}
		if len(reached) > 0 {
			res = Result{Outcome: FannedOut, Fanout: reached}
			return store.SetDisposition(ctx, tx, tr.ID, &ticket.Disposition{Fanout: reached, At: now})
		}
		decision, err := e.gate(ctx, tx, tr, thread, now)
		if err != nil {
			return err
		}
		res = Result{Outcome: Ignored, TicketID: decision.TicketID}
		if decision.Kind == ticket.DecisionAccept {
			res.Outcome = Accepted
		} else {
			res.Reason = &decision.Reason[0]
		}
		return store.SetDisposition(ctx, tx, tr.ID, &ticket.Disposition{DecisionID: decision.ID, At: now})
	})
	return res, err
}

// filter is the Source scope: the agent's own messages and channels it does not listen
// to never enter the domain. They are recorded, not silently dropped (upstream cookbook).
func (e *Engine) filter(tr *ticket.Trigger) *ticket.Reason {
	switch {
	case tr.Actor.ID == e.self:
		return &ticket.Reason{Code: "ownMessage"}
	case !e.channels[Scope(tr)]:
		return &ticket.Reason{Code: "channelNotHeard"}
	}
	return nil
}

// fanout hands the Trigger to the Tickets subscribed to its Thread, without checking the
// sender: a Subscription carries context and grants nothing. A live Ticket takes it as
// Input. An ended Ticket resumes under its unchanged Mandate when a person wrote it,
// unless a person stopped that Ticket; bots and CI never wake one (ANALIZ.md §5.3).
func (e *Engine) fanout(ctx context.Context, tx *sql.Tx, tr *ticket.Trigger, th *ticket.Thread, now time.Time) ([]string, error) {
	var reached []string
	for _, id := range th.Subscriptions {
		t, err := store.GetTicket(ctx, tx, id)
		if err != nil {
			return nil, fmt.Errorf("engine: subscribed ticket %s: %w", id, err)
		}
		if !t.Live() {
			if !tr.Human || t.Ended.By == ticket.EndByHuman {
				continue
			}
			if err := e.resume(ctx, tx, t, tr, now); err != nil {
				return nil, err
			}
		}
		addInput(t, ticket.TicketInput{
			ID: ticket.Key("input", t.ID, tr.ID), Kind: ticket.InputTrigger, TriggerID: tr.ID, Human: tr.Human, At: tr.At,
		})
		t.UpdatedAt = now
		if err := store.PutTicket(ctx, tx, t); err != nil {
			return nil, err
		}
		reached = append(reached, t.ID)
	}
	return reached, nil
}

// resume reopens an ended Ticket for a person's message with a new Assignment, the same
// way a retry does: same Mandate, same short name, same branch.
func (e *Engine) resume(ctx context.Context, tx *sql.Tx, t *ticket.Ticket, tr *ticket.Trigger, now time.Time) error {
	t.Ended = nil
	addInput(t, ticket.TicketInput{ID: ticket.Key("resumed", t.ID, tr.ID), Kind: ticket.InputResumed, TriggerID: tr.ID, At: tr.At})
	return e.newAssignment(ctx, tx, t, now)
}

// gate is the MVP Eval. Only a person who mentions the agent on a listened channel makes
// an explicit request; a bot never does, whatever it writes (§5.7).
func (e *Engine) gate(ctx context.Context, tx *sql.Tx, tr *ticket.Trigger, th *ticket.Thread, now time.Time) (*ticket.Decision, error) {
	d := &ticket.Decision{ID: ticket.Key("decision", tr.ID), TriggerIDs: []string{tr.ID}, At: now}
	switch {
	case !tr.Human:
		d.Kind, d.Reason = ticket.DecisionIgnore, []ticket.Reason{{Code: "noAdmissible"}}
	case tr.Addressed == ticket.AddressedNone:
		d.Kind, d.Reason = ticket.DecisionIgnore, []ticket.Reason{{Code: "notAddressed"}}
	default:
		t, err := e.start(ctx, tx, tr, th, d.ID, now)
		if err != nil {
			return nil, err
		}
		d.Kind, d.Strength, d.TicketID = ticket.DecisionAccept, ticket.Explicit, t.ID
		d.Reason = []ticket.Reason{{Code: "startedTicket", Values: map[string]string{"ticket": t.ID}}}
	}
	return d, store.PutDecision(ctx, tx, d)
}

// start creates the Ticket an accept decides on, subscribes it to the Thread, queues the
// ack and the launch of its first Assignment.
func (e *Engine) start(ctx context.Context, tx *sql.Tx, tr *ticket.Trigger, th *ticket.Thread, decisionID string, now time.Time) (*ticket.Ticket, error) {
	id := ticket.Key("ticket", tr.ID)
	title := Title(tr.Text)
	t := &ticket.Ticket{
		ID:         id,
		Title:      title,
		DecisionID: decisionID,
		Mandate: ticket.Mandate{
			Strength:      ticket.Explicit,
			PolicyVersion: PolicyVersion,
			Basis:         ticket.MandateBasis{Via: "addressed", Addressed: tr.Addressed, Scope: Scope(tr), Sender: tr.Actor.ID},
			Actor:         tr.Actor,
			Scope:         Scope(tr),
		},
		Channels:  []ticket.ChannelRef{*tr.Channel},
		CreatedAt: now,
		UpdatedAt: now,
		Origin:    ticket.Origin{TriggerID: tr.ID, TriggerIDs: []string{tr.ID}},
		Execution: &ticket.Execution{Name: ShortName(title, id)},
	}
	addInput(t, ticket.TicketInput{ID: ticket.Key("input", id, tr.ID), Kind: ticket.InputTrigger, TriggerID: tr.ID, Human: true, At: tr.At})
	if err := e.newAssignment(ctx, tx, t, now); err != nil {
		return nil, err
	}
	if err := store.PutTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	th.Subscriptions = append(th.Subscriptions, id)
	if err := store.PutThread(ctx, tx, th); err != nil {
		return nil, err
	}
	ack := "Aldım. Ticket `" + t.Execution.Name + "` açıldı, coder'a iletiyorum."
	_, err := outbox.Enqueue(ctx, tx, now, outbox.Message{
		ID: "ack:" + id, TicketID: id, Conversation: th.Key, Body: outbox.WithMarker("ack:"+id, ack),
	})
	return t, err
}

// newAssignment appends a pending Assignment and queues its launch.
func (e *Engine) newAssignment(ctx context.Context, tx *sql.Tx, t *ticket.Ticket, now time.Time) error {
	a := ticket.Assignment{
		ID:        ticket.Key("assignment", t.ID, fmt.Sprint(len(t.Assignments)+1)),
		State:     ticket.AssignmentPending,
		CreatedAt: now,
	}
	t.Assignments = append(t.Assignments, a)
	_, err := e.effects.Enqueue(ctx, tx, effects.Launch, "launch:"+a.ID, map[string]string{"ticket": t.ID, "assignment": a.ID})
	return err
}

// addInput records an Input once and marks it undelivered; the delivery effect (PLAN.md
// F4.1) hands undelivered Inputs to the running Assignment.
func addInput(t *ticket.Ticket, in ticket.TicketInput) {
	for _, x := range t.Inputs {
		if x.ID == in.ID {
			return
		}
	}
	t.Inputs = append(t.Inputs, in)
	if in.Kind == ticket.InputTrigger {
		t.Undelivered = append(t.Undelivered, in.ID)
	}
}
