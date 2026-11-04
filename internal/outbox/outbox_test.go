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
