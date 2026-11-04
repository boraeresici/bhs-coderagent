package buzz

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/boraeresici/bhs-coderagent/internal/outbox"
)

type fakeClient struct {
	thread []Event
	sent   []SendParams
}

func (f *fakeClient) Send(_ context.Context, p SendParams) (json.RawMessage, error) {
	f.sent = append(f.sent, p)
	return json.RawMessage(`{"accepted":true,"event_id":"` + chat(99, 1).ID + `","mention_pubkeys":[],"message":""}`), nil
}

func (f *fakeClient) Thread(context.Context, string, string, int) (json.RawMessage, error) {
	return json.Marshal(f.thread)
}

func TestDeliverPostsOnceToTheRoot(t *testing.T) {
	f := &fakeClient{}
	w := &Writer{Client: f, Self: orchPK}
	msg := outbox.Message{ID: "ack:T1", Conversation: "buzz:" + devChan + ":" + rootID, Body: outbox.WithMarker("ack:T1", "Aldım.")}

	receipt, err := w.Deliver(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 || f.sent[0].ReplyTo != rootID || f.sent[0].Channel != devChan || receipt != chat(99, 1).ID {
		t.Fatalf("sent = %+v receipt = %s", f.sent, receipt)
	}

	// After a crash the message is already in the thread: no second post.
	posted := Event{ID: chat(7, 1).ID, Pubkey: orchPK, CreatedAt: 1, Kind: KindChat, Content: msg.Body}
	f.thread = []Event{posted}
	receipt, err = w.Deliver(context.Background(), msg)
	if err != nil || len(f.sent) != 1 || receipt != posted.ID {
		t.Fatalf("re-delivery posted again: sent=%d receipt=%s err=%v", len(f.sent), receipt, err)
	}
}

func TestDeliverIgnoresQuotedMarkers(t *testing.T) {
	msg := outbox.Message{ID: "ack:T1", Conversation: "buzz:" + devChan + ":" + rootID, Body: "x"}
	// A person quoting the marker must not make the orchestrator think it already spoke.
	f := &fakeClient{thread: []Event{{ID: chat(7, 1).ID, Pubkey: humanPK, CreatedAt: 1, Kind: KindChat, Content: outbox.Marker("ack:T1")}}}
	w := &Writer{Client: f, Self: orchPK}
	if _, err := w.Deliver(context.Background(), msg); err != nil || len(f.sent) != 1 {
		t.Fatalf("sent = %d err = %v", len(f.sent), err)
	}
}

func TestThreadAddress(t *testing.T) {
	if _, _, err := ThreadAddress("github:o/r#1"); err == nil {
		t.Error("non-Buzz conversation accepted")
	}
	ch, root, err := ThreadAddress("buzz:" + devChan + ":" + rootID)
	if err != nil || ch != devChan || root != rootID {
		t.Errorf("= %s %s %v", ch, root, err)
	}
}
