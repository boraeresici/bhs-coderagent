package outbox

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/store"
)

var (
	ctx = context.Background()
	t0  = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
)

func db(t *testing.T) store.Querier {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s.DB()
}

func enqueue(t *testing.T, q store.Querier, id, conv string, min int) bool {
	t.Helper()
	added, err := Enqueue(ctx, q, t0.Add(time.Duration(min)*time.Minute), Message{ID: id, TicketID: "T1", Conversation: conv, Body: id})
	if err != nil {
		t.Fatal(err)
	}
	return added
}

func headIDs(t *testing.T, q store.Querier) []string {
	t.Helper()
	hs, err := Heads(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, h := range hs {
		ids = append(ids, h.ID)
	}
	return ids
}

func TestIDQueuedOnce(t *testing.T) {
	q := db(t)
	if !enqueue(t, q, "ack:T1", "c1", 0) {
		t.Fatal("first enqueue not added")
	}
	if enqueue(t, q, "ack:T1", "c1", 1) {
		t.Fatal("the same id must never be queued twice")
	}
	if err := MarkSent(ctx, q, t0, "ack:T1", "ev1"); err != nil {
		t.Fatal(err)
	}
	if enqueue(t, q, "ack:T1", "c1", 2) {
		t.Fatal("a sent id must not be queued again either")
	}
}

func TestConversationOrder(t *testing.T) {
	q := db(t)
	enqueue(t, q, "ack:T1", "c1", 0)
	enqueue(t, q, "result:T1", "c1", 1)
	enqueue(t, q, "ack:T2", "c2", 2)

	if got, want := headIDs(t, q), []string{"ack:T1", "ack:T2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("heads = %v, want %v", got, want)
	}
	if err := MarkSent(ctx, q, t0, "result:T1", "x"); !errors.Is(err, ErrNotHead) {
		t.Fatalf("sending out of order = %v, want ErrNotHead", err)
	}
	if err := MarkSent(ctx, q, t0, "ack:T1", "ev1"); err != nil {
		t.Fatal(err)
	}
	if err := MarkSent(ctx, q, t0, "ack:T1", "ev1"); err != nil {
		t.Fatalf("marking sent twice must be harmless: %v", err)
	}
	if got, want := headIDs(t, q), []string{"result:T1", "ack:T2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("heads = %v, want %v", got, want)
	}
}

func TestMarkers(t *testing.T) {
	body := WithMarker("asg:T1:A1", "@coder fix it")
	if got := Markers("quoted " + body + " and " + Marker("ack:T1")); !reflect.DeepEqual(got, []string{"asg:T1:A1", "ack:T1"}) {
		t.Fatalf("Markers = %v", got)
	}
	if Markers("<!-- coderagent:bad id -->") != nil {
		t.Fatal("a marker with a space is not one of ours")
	}
	q := db(t)
	if _, err := Enqueue(ctx, q, t0, Message{ID: "has space", Conversation: "c"}); err == nil {
		t.Fatal("ids that cannot round-trip through a marker must be rejected")
	}
}

func TestFailBacksOffAndCounts(t *testing.T) {
	db := db(t)
	enqueue(t, db, "ack:t1", "c", 0)
	for i, wait := range []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second} {
		n, err := Fail(ctx, db, t0, "ack:t1", errors.New("relay down"))
		if err != nil || n != i+1 {
			t.Fatalf("fail %d: %d %v", i+1, n, err)
		}
		heads, _ := Heads(ctx, db)
		if len(heads) != 1 || heads[0].Attempts != i+1 || !heads[0].NextAt.Equal(t0.Add(wait)) {
			t.Fatalf("after %d failures: %+v", i+1, heads)
		}
	}
	if Backoff(20) != 5*time.Minute {
		t.Errorf("backoff caps at 5m: %v", Backoff(20))
	}
	if err := MarkSent(ctx, db, t0, "ack:t1", "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := Fail(ctx, db, t0, "ack:t1", errors.New("x")); err == nil {
		t.Error("a sent message cannot fail")
	}
}

func TestRateWait(t *testing.T) {
	db := db(t)
	for i := range RateLimit + 1 {
		id := "m" + string(rune('a'+i))
		enqueue(t, db, id, "c", 0)
		if i < RateLimit {
			if err := MarkSent(ctx, db, t0.Add(time.Duration(i)*time.Minute), id, "r"); err != nil {
				t.Fatal(err)
			}
		}
	}
	now := t0.Add(9*time.Minute + 30*time.Second)
	if w, _ := RateWait(ctx, db, now, "c"); w != 30*time.Second {
		t.Errorf("ten sends in ten minutes: wait %v, want until the first leaves the window", w)
	}
	if w, _ := RateWait(ctx, db, now, "other"); w != 0 {
		t.Errorf("the limit is per conversation: %v", w)
	}
	if w, _ := RateWait(ctx, db, t0.Add(10*time.Minute+time.Second), "c"); w != 0 {
		t.Errorf("after the window: %v", w)
	}
}
