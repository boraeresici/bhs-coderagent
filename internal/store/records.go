package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// ErrNotFound means no record has that key.
var ErrNotFound = errors.New("store: not found")

// Querier is what record helpers need; both *sql.DB and *sql.Tx satisfy it, so the
// engine can group writes in one transaction.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// PutTicket inserts or replaces a Ticket.
func PutTicket(ctx context.Context, q Querier, t *ticket.Ticket) error {
	body, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx,
		`INSERT INTO ticket(id, body) VALUES(?, ?) ON CONFLICT(id) DO UPDATE SET body = excluded.body`,
		t.ID, string(body))
	return err
}

// GetTicket reads one Ticket.
func GetTicket(ctx context.Context, q Querier, id string) (*ticket.Ticket, error) {
	var body string
	err := q.QueryRowContext(ctx, `SELECT body FROM ticket WHERE id = ?`, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t ticket.Ticket
	return &t, json.Unmarshal([]byte(body), &t)
}

// OpenTickets lists open Tickets, oldest first.
func OpenTickets(ctx context.Context, q Querier) ([]*ticket.Ticket, error) {
	rows, err := q.QueryContext(ctx, `SELECT body FROM ticket WHERE open = 1 ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ticket.Ticket
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var t ticket.Ticket
		if err := json.Unmarshal([]byte(body), &t); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// GetTrigger reads one Trigger.
func GetTrigger(ctx context.Context, q Querier, id string) (*ticket.Trigger, error) {
	var body string
	err := q.QueryRowContext(ctx, `SELECT body FROM trigger WHERE id = ?`, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t ticket.Trigger
	return &t, json.Unmarshal([]byte(body), &t)
}

// TriggerResult is what receiving a Trigger did (upstream Engine.receive).
type TriggerResult string

const (
	TriggerStored    TriggerResult = "stored"
	TriggerDuplicate TriggerResult = "duplicate"
)

// PutTrigger stores a Trigger once; a second Trigger with the same id is a duplicate and
// changes nothing, since accepting a Trigger twice must create one Ticket.
func PutTrigger(ctx context.Context, q Querier, tr *ticket.Trigger) (TriggerResult, error) {
	body, err := json.Marshal(tr)
	if err != nil {
		return "", err
	}
	res, err := q.ExecContext(ctx, `INSERT INTO trigger(id, body) VALUES(?, ?) ON CONFLICT(id) DO NOTHING`, tr.ID, string(body))
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return TriggerDuplicate, nil
	}
	return TriggerStored, nil
}

// SetDisposition records where a stored Trigger went.
func SetDisposition(ctx context.Context, q Querier, triggerID string, d *ticket.Disposition) error {
	body, err := json.Marshal(d)
	if err != nil {
		return err
	}
	res, err := q.ExecContext(ctx, `UPDATE trigger SET disposition = ? WHERE id = ?`, string(body), triggerID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetDisposition reads where a Trigger went; nil when it has none yet.
func GetDisposition(ctx context.Context, q Querier, triggerID string) (*ticket.Disposition, error) {
	var body sql.NullString
	err := q.QueryRowContext(ctx, `SELECT disposition FROM trigger WHERE id = ?`, triggerID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil || !body.Valid {
		return nil, err
	}
	var d ticket.Disposition
	return &d, json.Unmarshal([]byte(body.String), &d)
}

// GetThread reads a Thread; a Thread never seen before comes back empty, not as an error.
func GetThread(ctx context.Context, q Querier, key string) (*ticket.Thread, error) {
	var body string
	err := q.QueryRowContext(ctx, `SELECT body FROM thread WHERE key = ?`, key).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return &ticket.Thread{Key: key}, nil
	}
	if err != nil {
		return nil, err
	}
	var t ticket.Thread
	return &t, json.Unmarshal([]byte(body), &t)
}

// PutThread inserts or replaces a Thread.
func PutThread(ctx context.Context, q Querier, t *ticket.Thread) error {
	body, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx,
		`INSERT INTO thread(key, body) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET body = excluded.body`, t.Key, string(body))
	return err
}

// PutDecision stores a Decision once; Decisions never change.
func PutDecision(ctx context.Context, q Querier, d *ticket.Decision) error {
	body, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO decision(id, body) VALUES(?, ?) ON CONFLICT(id) DO NOTHING`, d.ID, string(body))
	return err
}

// GetDecision reads one Decision.
func GetDecision(ctx context.Context, q Querier, id string) (*ticket.Decision, error) {
	var body string
	err := q.QueryRowContext(ctx, `SELECT body FROM decision WHERE id = ?`, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d ticket.Decision
	return &d, json.Unmarshal([]byte(body), &d)
}

// GetKV reads a value; ok is false when the key is unset.
func GetKV(ctx context.Context, q Querier, k string) (v string, ok bool, err error) {
	err = q.QueryRowContext(ctx, `SELECT v FROM kv WHERE k = ?`, k).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

// SetKV writes a value.
func SetKV(ctx context.Context, q Querier, k, v string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO kv(k, v) VALUES(?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`, k, v)
	return err
}
