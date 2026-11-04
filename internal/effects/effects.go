// Package effects is the durable work queue behind every side effect the engine decides
// on: launching an Assignment, delivering an Input, sending a reply (ANALIZ.md §3.8,
// upstream src/app/effects.ts).
//
// The engine records an effect in the same transaction as the decision that needs it,
// so a crash never loses one. Effects run at least once: a claimed effect is leased, and
// a lease that runs out puts it back in the queue. Handlers must therefore be idempotent,
// which the hidden markers of delivery and the outbox provide (§5.4).
package effects

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/store"
)

// Kinds of effects (upstream effect types).
const (
	Launch   = "launch"
	Delivery = "delivery"
	Reply    = "reply"
	Notice   = "notice"
)

// Policy is how one kind runs and retries. Backoff doubles from Base up to Max;
// MaxAttempts zero retries forever.
type Policy struct {
	Concurrency int
	Base, Max   time.Duration
	MaxAttempts int
}

// DefaultPolicies follow upstream: delivery 4 at a time 2s→60s, launch 5s→300s with
// three attempts, replies and notices one at a time and in order.
var DefaultPolicies = map[string]Policy{
	Launch:   {Concurrency: 1, Base: 5 * time.Second, Max: 300 * time.Second, MaxAttempts: 3},
	Delivery: {Concurrency: 4, Base: 2 * time.Second, Max: 60 * time.Second},
	Reply:    {Concurrency: 1, Base: 5 * time.Second, Max: 300 * time.Second},
	Notice:   {Concurrency: 1, Base: 5 * time.Second, Max: 300 * time.Second},
}

// Lease is how long a claimed effect stays out of the queue before it is retried.
const Lease = 5 * time.Minute

// Effect is one queued unit of work.
type Effect struct {
	ID       int64
	Kind     string
	Key      string
	Payload  json.RawMessage
	Attempts int
}

// Queue is the effect table with a clock and per-kind policies.
type Queue struct {
	db       *sql.DB
	now      func() time.Time
	policies map[string]Policy
}

// New returns a Queue; now is the clock (time.Now in production, a fake in tests).
func New(db *sql.DB, now func() time.Time, policies map[string]Policy) *Queue {
	return &Queue{db: db, now: now, policies: policies}
}

// Policy returns the policy of a kind.
func (q *Queue) Policy(kind string) (Policy, error) {
	p, ok := q.policies[kind]
	if !ok {
		return Policy{}, fmt.Errorf("effects: unknown kind %q", kind)
	}
	return p, nil
}

// Enqueue records an effect once per key; a second Enqueue with the same key changes
// nothing and reports false. Pass the engine's transaction as tx so the effect commits
// with the decision that needs it.
func (q *Queue) Enqueue(ctx context.Context, tx store.Querier, kind, key string, payload any) (bool, error) {
	if _, err := q.Policy(kind); err != nil {
		return false, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	now := q.now().UnixMilli()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO effect(kind, key, payload, next_at, created_at) VALUES(?, ?, ?, ?, ?) ON CONFLICT(key) DO NOTHING`,
		kind, key, string(body), now, now)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Claim leases up to limit due effects of a kind, oldest due first.
func (q *Queue) Claim(ctx context.Context, kind string, limit int) ([]Effect, error) {
	now := q.now()
	var out []Effect
	err := withTx(ctx, q.db, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id, kind, key, payload, attempts FROM effect
			 WHERE state = 'pending' AND kind = ? AND next_at <= ? ORDER BY next_at, id LIMIT ?`,
			kind, now.UnixMilli(), limit)
		if err != nil {
			return err
		}
		for rows.Next() {
			var e Effect
			var payload string
			if err := rows.Scan(&e.ID, &e.Kind, &e.Key, &payload, &e.Attempts); err != nil {
				rows.Close()
				return err
			}
			e.Payload = json.RawMessage(payload)
			out = append(out, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, e := range out {
			if _, err := tx.ExecContext(ctx, `UPDATE effect SET next_at = ?, leased = 1 WHERE id = ?`, now.Add(Lease).UnixMilli(), e.ID); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// Done marks an effect finished.
func (q *Queue) Done(ctx context.Context, id int64) error {
	return q.set(ctx, `UPDATE effect SET state = 'done', leased = 0, last_error = NULL WHERE id = ? AND state = 'pending'`, id, id)
}

// Fail counts a failed attempt and schedules the next one by the kind's backoff. It
// reports dead once the kind's MaxAttempts is used up; the engine then asks a person.
func (q *Queue) Fail(ctx context.Context, id int64, cause error) (dead bool, err error) {
	err = withTx(ctx, q.db, func(tx *sql.Tx) error {
		var kind string
		var attempts int
		if err := tx.QueryRowContext(ctx, `SELECT kind, attempts FROM effect WHERE id = ? AND state = 'pending'`, id).
			Scan(&kind, &attempts); err != nil {
			return err
		}
		p, err := q.Policy(kind)
		if err != nil {
			return err
		}
		attempts++
		dead = p.MaxAttempts > 0 && attempts >= p.MaxAttempts
		state, next := "pending", q.now().Add(Backoff(p, attempts)).UnixMilli()
		if dead {
			state = "dead"
		}
		_, err = tx.ExecContext(ctx, `UPDATE effect SET state = ?, attempts = ?, next_at = ?, leased = 0, last_error = ? WHERE id = ?`,
			state, attempts, next, cause.Error(), id)
		return err
	})
	return dead, err
}

// Later puts an effect back for a later time without counting an attempt: what it needs
// is not ready yet, which is no failure (upstream Later).
func (q *Queue) Later(ctx context.Context, id int64, at time.Time) error {
	return q.set(ctx, `UPDATE effect SET next_at = ?, leased = 0 WHERE id = ? AND state = 'pending'`, id, at.UnixMilli(), id)
}

// ReleaseLeases hands every claimed effect back to the queue at once. Call it at startup:
// the store's directory lock means no other process holds a lease, so a lease left from
// before a crash would otherwise wait out Lease. It returns how many were released.
func (q *Queue) ReleaseLeases(ctx context.Context) (int, error) {
	res, err := q.db.ExecContext(ctx, `UPDATE effect SET next_at = ?, leased = 0 WHERE state = 'pending' AND leased = 1`, q.now().UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Backoff is the wait after the given number of failed attempts.
func Backoff(p Policy, attempts int) time.Duration {
	d := p.Base
	for i := 1; i < attempts && d < p.Max; i++ {
		d *= 2
	}
	return min(d, p.Max)
}

// set runs an update of one pending effect; id is reported when nothing matched.
func (q *Queue) set(ctx context.Context, query string, id int64, args ...any) error {
	res, err := q.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("effects: no pending effect %d", id)
	}
	return nil
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}
