package engine

import (
	"context"
	"database/sql"

	"github.com/boraeresici/bhs-coderagent/internal/effects"
	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// breakerAttempts is how many sends in a row may fail before a person is asked (§5.7,
// upstream circuit breaker). Sending goes on at the outbox's longest backoff meanwhile.
const breakerAttempts = 3

// SendFailed records that a message for a Ticket could not be sent after attempts tries
// in a row. From breakerAttempts on the Ticket waits on a person (person/delivery), with
// the latest error.
func (e *Engine) SendFailed(ctx context.Context, ticketID string, attempts int, cause error) error {
	if ticketID == "" || attempts < breakerAttempts {
		return nil
	}
	return e.st.Tx(ctx, func(tx *sql.Tx) error {
		t, err := store.GetTicket(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		now := e.now()
		t.Delivery = &ticket.DeliveryFailure{Error: cause.Error(), At: now}
		t.UpdatedAt = now
		return store.PutTicket(ctx, tx, t)
	})
}

// SendSucceeded clears a Ticket's delivery failure once one of its messages went out.
func (e *Engine) SendSucceeded(ctx context.Context, ticketID string) error {
	if ticketID == "" {
		return nil
	}
	return e.st.Tx(ctx, func(tx *sql.Tx) error {
		t, err := store.GetTicket(ctx, tx, ticketID)
		if err != nil || t.Delivery == nil {
			return err
		}
		t.Delivery = nil
		t.UpdatedAt = e.now()
		return store.PutTicket(ctx, tx, t)
	})
}

// EffectDead records that an effect used up its attempts, so the Ticket waits on a person
// instead of looking queued forever: a dead launch fails its Assignment (person/failed),
// a dead delivery trips the breaker (person/delivery).
func (e *Engine) EffectDead(ctx context.Context, kind, ticketID, assignmentID string, cause error) error {
	return e.st.Tx(ctx, func(tx *sql.Tx) error {
		t, err := store.GetTicket(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		now := e.now()
		switch kind {
		case effects.Launch:
			a := assignment(t, assignmentID)
			if a == nil || a.State == ticket.AssignmentEnded {
				return nil
			}
			a.State, a.EndedAt, a.EndedReason = ticket.AssignmentEnded, &now, "launch failed"
			a.Report = &ticket.Report{State: ticket.ReportFailed, Summary: "devir gönderilemedi: " + cause.Error(), At: now}
		case effects.Delivery:
			t.Delivery = &ticket.DeliveryFailure{Error: cause.Error(), At: now}
		default:
			return nil
		}
		t.UpdatedAt = now
		return store.PutTicket(ctx, tx, t)
	})
}
