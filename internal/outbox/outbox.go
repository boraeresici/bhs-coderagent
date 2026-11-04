// Package outbox holds the messages the orchestrator owes a conversation: acks, answers,
// results and closing notices (ANALIZ.md §5.4).
//
// A message id enters the outbox at most once, and the messages of one conversation go
// out in the order they were queued: only a conversation's oldest pending message may be
// sent. Each body carries a hidden marker with its id, so after a crash between sending
// and recording the receipt the sender can find the message in the thread instead of
// posting it twice.
package outbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/store"
)

// Message is one reply owed to a conversation.
type Message struct {
	// ID is stable for what the message says, e.g. "ack:<ticket>"; it names the marker.
	ID       string
	TicketID string
	// Conversation is where it goes, e.g. a Buzz thread key; order holds within it.
	Conversation string
	Body         string
	Seq          int64
}

// Marker is the hidden tag appended to a message body (§5.4). Buzz renders message text
// as Markdown, where an HTML comment does not show.
func Marker(id string) string { return "<!-- coderagent:" + id + " -->" }

var markerRE = regexp.MustCompile(`<!-- coderagent:([A-Za-z0-9:._-]+) -->`)

// Markers returns every marker id found in a text, in order.
func Markers(text string) []string {
	var ids []string
	for _, m := range markerRE.FindAllStringSubmatch(text, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

// WithMarker returns the body with its marker appended.
func WithMarker(id, body string) string { return body + "\n\n" + Marker(id) }

var validID = regexp.MustCompile(`^[A-Za-z0-9:._-]+$`)

// Enqueue adds a message at the end of its conversation. It reports false and changes
// nothing when the id was queued before. Pass the engine's transaction as tx.
func Enqueue(ctx context.Context, tx store.Querier, now time.Time, m Message) (bool, error) {
	if !validID.MatchString(m.ID) {
		return false, fmt.Errorf("outbox: id %q cannot be a marker", m.ID)
	}
	if m.Conversation == "" {
		return false, errors.New("outbox: message has no conversation")
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO outbox(id, ticket_id, conversation, seq, body, created_at)
VALUES(?, NULLIF(?, ''), ?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM outbox WHERE conversation = ?), ?, ?)
ON CONFLICT(id) DO NOTHING`,
		m.ID, m.TicketID, m.Conversation, m.Conversation, m.Body, now.UnixMilli())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Heads returns, for each conversation with pending messages, its oldest one: the only
// message of that conversation that may be sent now.
func Heads(ctx context.Context, q store.Querier) ([]Message, error) {
	rows, err := q.QueryContext(ctx, `
SELECT o.id, COALESCE(o.ticket_id, ''), o.conversation, o.body, o.seq FROM outbox o
WHERE o.state = 'pending'
  AND o.seq = (SELECT MIN(seq) FROM outbox WHERE conversation = o.conversation AND state = 'pending')
ORDER BY o.created_at, o.conversation`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.TicketID, &m.Conversation, &m.Body, &m.Seq); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ErrNotHead means a message was marked sent while an older one of its conversation is
// still pending, which would break the conversation's order.
var ErrNotHead = errors.New("outbox: an older message of this conversation is still pending")

// MarkSent records that a message went out, with the Source's receipt (e.g. the event id).
func MarkSent(ctx context.Context, q store.Querier, now time.Time, id, receipt string) error {
	var conversation string
	var seq int64
	var state string
	err := q.QueryRowContext(ctx, `SELECT conversation, seq, state FROM outbox WHERE id = ?`, id).Scan(&conversation, &seq, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("outbox: no message %q", id)
	}
	if err != nil {
		return err
	}
	if state == "sent" {
		return nil
	}
	var older int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox WHERE conversation = ? AND state = 'pending' AND seq < ?`,
		conversation, seq).Scan(&older); err != nil {
		return err
	}
	if older > 0 {
		return ErrNotHead
	}
	_, err = q.ExecContext(ctx, `UPDATE outbox SET state = 'sent', receipt = ?, sent_at = ? WHERE id = ?`, receipt, now.UnixMilli(), id)
	return err
}
