package effects

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/store"
)

var ctx = context.Background()

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func setup(t *testing.T) (*Queue, *clock, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c := &clock{time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)}
	return New(s.DB(), c.now, DefaultPolicies), c, s, dir
}

func claimOne(t *testing.T, q *Queue, kind string) *Effect {
	t.Helper()
	es, err := q.Claim(ctx, kind, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) == 0 {
		return nil
	}
	return &es[0]
}

func TestEnqueueIsIdempotentByKey(t *testing.T) {
	q, _, _, _ := setup(t)
	for i, want := range []bool{true, false} {
		added, err := q.Enqueue(ctx, q.db, Delivery, "T1:A1:I1", map[string]int{"n": i})
		if err != nil || added != want {
			t.Fatalf("Enqueue #%d = %v, %v", i, added, err)
		}
	}
	e := claimOne(t, q, Delivery)
	if e == nil || string(e.Payload) != `{"n":0}` {
		t.Fatalf("claimed %+v: the first payload must win", e)
	}
}

func TestClaimLeasesAndExpiredLeaseRetries(t *testing.T) {
	q, c, _, _ := setup(t)
	q.Enqueue(ctx, q.db, Reply, "r1", nil)
	if claimOne(t, q, Reply) == nil {
		t.Fatal("due effect not claimed")
	}
	if claimOne(t, q, Reply) != nil {
		t.Fatal("a leased effect must not be claimed twice")
	}
	c.advance(Lease)
	if claimOne(t, q, Reply) == nil {
		t.Fatal("an effect whose lease ran out must come back (at-least-once)")
	}
}

func TestFailBacksOffAndLaunchDiesAfterThree(t *testing.T) {
	q, c, _, _ := setup(t)
	q.Enqueue(ctx, q.db, Launch, "launch:T1", nil)
	waits := []time.Duration{5 * time.Second, 10 * time.Second}
	for attempt := 1; attempt <= 3; attempt++ {
		e := claimOne(t, q, Launch)
		if e == nil {
			t.Fatalf("attempt %d: nothing due", attempt)
		}
		dead, err := q.Fail(ctx, e.ID, errors.New("boom"))
		if err != nil {
			t.Fatal(err)
		}
		if dead != (attempt == 3) {
			t.Fatalf("attempt %d: dead = %v", attempt, dead)
		}
		if attempt < 3 {
			c.advance(waits[attempt-1] - time.Millisecond)
			if claimOne(t, q, Launch) != nil {
				t.Fatalf("attempt %d: retried before its backoff", attempt)
			}
			c.advance(time.Millisecond)
		}
	}
	c.advance(time.Hour)
	if claimOne(t, q, Launch) != nil {
		t.Fatal("a dead effect must not run again")
	}
}

func TestBackoffCaps(t *testing.T) {
	p := DefaultPolicies[Delivery]
	for attempts, want := range map[int]time.Duration{1: 2 * time.Second, 2: 4 * time.Second, 5: 32 * time.Second, 6: 60 * time.Second, 50: 60 * time.Second} {
		if got := Backoff(p, attempts); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempts, got, want)
		}
	}
}

func TestLaterDoesNotCountAnAttempt(t *testing.T) {
	q, c, _, _ := setup(t)
	q.Enqueue(ctx, q.db, Delivery, "d1", nil)
	e := claimOne(t, q, Delivery)
	if err := q.Later(ctx, e.ID, c.t.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	c.advance(30 * time.Second)
	e = claimOne(t, q, Delivery)
	if e == nil || e.Attempts != 0 {
		t.Fatalf("after Later: %+v", e)
	}
}

func TestDoneAndUnknownKind(t *testing.T) {
	q, c, _, _ := setup(t)
	q.Enqueue(ctx, q.db, Notice, "n1", nil)
	e := claimOne(t, q, Notice)
	if err := q.Done(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Done(ctx, e.ID); err == nil {
		t.Fatal("finishing an effect twice must be an error")
	}
	c.advance(time.Hour)
	if claimOne(t, q, Notice) != nil {
		t.Fatal("a done effect must not run again")
	}
	if _, err := q.Enqueue(ctx, q.db, "assessment", "x", nil); err == nil {
		t.Fatal("unknown kinds must be rejected")
	}
}

func TestQueueSurvivesRestart(t *testing.T) {
	q, c, s, dir := setup(t)
	q.Enqueue(ctx, q.db, Delivery, "d1", nil)
	s.Close() // the process stops before running it

	s2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	q2 := New(s2.DB(), c.now, DefaultPolicies)
	if claimOne(t, q2, Delivery) == nil {
		t.Fatal("a queued effect must survive a restart")
	}
}
