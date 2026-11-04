package engine

import (
	"context"
	"database/sql"
	"errors"

	"github.com/boraeresici/bhs-coderagent/internal/outbox"
	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// ErrConflict means an action does not fit the Ticket's stage: retrying a Ticket that is
// not stalled, stopping one that has ended.
var ErrConflict = errors.New("engine: the ticket's stage does not allow this")

const pausedKey = "engine:paused"

// Retry restarts a stalled Ticket from the panel (F6.3). After a blocked or failed report
// it starts the next Assignment: same Mandate, same short name, same branch. After failed
// sends it clears the breaker and sends the waiting messages now.
func (e *Engine) Retry(ctx context.Context, ticketID string) error {
	return e.st.Tx(ctx, func(tx *sql.Tx) error {
		t, err := store.GetTicket(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		if !ticket.CanRetry(t) {
			return ErrConflict
		}
		now := e.now()
		if t.Delivery != nil {
			t.Delivery = nil
			if _, err := tx.ExecContext(ctx, `UPDATE outbox SET attempts = 0, next_at = 0 WHERE ticket_id = ? AND state = 'pending'`, t.ID); err != nil {
				return err
			}
		}
		if t.WaitsForPerson() && !pending(t) {
			if err := e.newAssignment(ctx, tx, t, now); err != nil {
				return err
			}
		}
		t.UpdatedAt = now
		return store.PutTicket(ctx, tx, t)
	})
}

// Stop ends an open Ticket from the panel: a person stopped it, so a later message does
// not resume it (§5.3). A running Assignment ends and the coder is told to stop; the
// branch stays.
func (e *Engine) Stop(ctx context.Context, ticketID string) error {
	return e.st.Tx(ctx, func(tx *sql.Tx) error {
		t, err := store.GetTicket(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		if !t.Live() {
			return ErrConflict
		}
		now := e.now()
		running := t.RunningAssignment() != nil
		for i := range t.Assignments {
			if a := &t.Assignments[i]; a.State != ticket.AssignmentEnded {
				a.State, a.EndedAt, a.EndedReason = ticket.AssignmentEnded, &now, "stopped"
			}
		}
		t.Ended = &ticket.End{At: now, By: ticket.EndByHuman, Outcome: ticket.OutcomeStopped, Reason: "panel"}
		t.Attention, t.Delivery, t.UpdatedAt = nil, nil, now
		if err := store.PutTicket(ctx, tx, t); err != nil {
			return err
		}
		id := "stop:" + t.ID
		m := outbox.Message{ID: id, TicketID: t.ID, Conversation: t.Channels[0].Thread,
			Body: outbox.WithMarker(id, "Ticket `"+t.Execution.Name+"` panelden durduruldu.")}
		if running {
			m.Mentions = []string{e.coder}
			m.Body = outbox.WithMarker(id, "@"+e.coderName+" Ticket `"+t.Execution.Name+"` panelden durduruldu: bu iş için çalışmayı bırak, rapor gerekmez. Dal `agent/"+t.Execution.Name+"` olduğu gibi kalsın.")
		}
		_, err = outbox.Enqueue(ctx, tx, now, m)
		return err
	})
}

// SetPaused stops or restarts launching Assignments. While paused, launches wait; running
// work, Inputs to it and replies go on.
func (e *Engine) SetPaused(ctx context.Context, paused bool) error {
	v := "0"
	if paused {
		v = "1"
	}
	return store.SetKV(ctx, e.st.DB(), pausedKey, v)
}

// Paused reports whether launching is paused.
func (e *Engine) Paused(ctx context.Context) (bool, error) {
	v, _, err := store.GetKV(ctx, e.st.DB(), pausedKey)
	return v == "1", err
}
