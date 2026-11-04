package engine

import (
	"context"
	"database/sql"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/outbox"
	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// BindThread makes an open Ticket follow a Thread it opened, such as the pull request of
// its branch, and subscribes it there (upstream bindThread own=true). The GitHub Source
// binds a pull request it finds on agent/<short name> even when the coder's report never
// named it (PLAN.md F5.2). Binding twice changes nothing.
func (e *Engine) BindThread(ctx context.Context, ticketID, threadKey string) error {
	return e.st.Tx(ctx, func(tx *sql.Tx) error {
		t, err := store.GetTicket(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		if !t.Live() {
			return nil
		}
		for _, f := range t.Follows {
			if f.Thread == threadKey {
				return e.subscribeFollows(ctx, tx, t)
			}
		}
		t.Follows = append(t.Follows, ticket.Follow{Thread: threadKey, Own: true})
		t.UpdatedAt = e.now()
		if err := store.PutTicket(ctx, tx, t); err != nil {
			return err
		}
		return e.subscribeFollows(ctx, tx, t)
	})
}

// subscribeFollows subscribes a Ticket to every Thread it follows, so what happens on its
// pull request fans out to it.
func (e *Engine) subscribeFollows(ctx context.Context, tx *sql.Tx, t *ticket.Ticket) error {
	for _, f := range t.Follows {
		th, err := store.GetThread(ctx, tx, f.Thread)
		if err != nil {
			return err
		}
		if th.Subscribed(t.ID) {
			continue
		}
		th.Subscriptions = append(th.Subscriptions, t.ID)
		if err := store.PutThread(ctx, tx, th); err != nil {
			return err
		}
	}
	return nil
}

// pullClosed records that a followed pull request merged or closed. Once every pull
// request the Ticket opened has closed, the Ticket ends and its Buzz thread gets one
// closing message (§5.8 step 4). A person merges or closes, so a person ended it: a later
// message on the thread does not resume it. It reports whether the Ticket changed.
func (e *Engine) pullClosed(ctx context.Context, tx *sql.Tx, t *ticket.Ticket, tr *ticket.Trigger, now time.Time) (bool, error) {
	outcome := ticket.OutcomeClosed
	if tr.Event == ticket.EventMerged {
		outcome = ticket.OutcomeMerged
	}
	key := tr.Threads[0].Key()
	changed := false
	for i := range t.Follows {
		if f := &t.Follows[i]; f.Thread == key && f.Closed == nil {
			f.Closed, changed = &ticket.FollowClosed{At: tr.At, Outcome: outcome}, true
		}
	}
	if !changed {
		return false, nil
	}
	t.UpdatedAt = now
	if !t.Live() {
		return true, nil
	}
	for _, f := range t.Follows {
		if f.Own && f.Closed == nil {
			return true, nil
		}
	}
	if a := t.RunningAssignment(); a != nil {
		a.State, a.EndedAt, a.EndedReason = ticket.AssignmentEnded, &now, "pull request "+string(outcome)
	}
	t.Ended = &ticket.End{At: now, By: ticket.EndByHuman, Outcome: outcome}
	t.Attention = nil
	verb := "merge edildi"
	if outcome == ticket.OutcomeClosed {
		verb = "merge edilmeden kapatıldı"
	}
	id := "end:" + t.ID
	_, err := outbox.Enqueue(ctx, tx, now, outbox.Message{
		ID: id, TicketID: t.ID, Conversation: t.Channels[0].Thread,
		Body: outbox.WithMarker(id, "PR "+tr.URL+" "+verb+"; Ticket `"+t.Execution.Name+"` kapandı."),
	})
	return true, err
}
