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
