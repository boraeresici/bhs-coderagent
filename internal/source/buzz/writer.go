package buzz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/boraeresici/bhs-coderagent/internal/outbox"
)

// Client is the CLI surface the Writer needs.
type Client interface {
	Send(ctx context.Context, p SendParams) (json.RawMessage, error)
	Thread(ctx context.Context, channel, root string, limit int) (json.RawMessage, error)
}

// Writer posts outbox messages to their Buzz thread.
type Writer struct {
	Client Client
	// Self is the orchestrator's pubkey; only its own messages count as already sent.
	Self string
}

// ThreadAddress splits a Buzz conversation key "buzz:<channel>:<root>".
func ThreadAddress(conversation string) (channel, root string, err error) {
	rest, ok := strings.CutPrefix(conversation, "buzz:")
	if ok {
		channel, root, ok = strings.Cut(rest, ":")
	}
	if !ok || !uuidRE.MatchString(channel) || !hex64.MatchString(root) {
		return "", "", fmt.Errorf("buzz: %q is not a Buzz thread", conversation)
	}
	return channel, root, nil
}

// Deliver posts a message once and returns its event id as the receipt. A crash between
// posting and recording the receipt would post it again, so the thread is searched for
// the message's marker first, among the orchestrator's own messages only: anyone can
// quote a marker, only the orchestrator can sign as itself (ANALIZ.md §5.4).
func (w *Writer) Deliver(ctx context.Context, m outbox.Message) (string, error) {
	channel, root, err := ThreadAddress(m.Conversation)
	if err != nil {
		return "", err
	}
	if id, err := w.find(ctx, channel, root, m.ID); err != nil || id != "" {
		return id, err
	}
	raw, err := w.Client.Send(ctx, SendParams{Channel: channel, ReplyTo: root, Mentions: m.Mentions, Content: m.Body})
	if err != nil {
		return "", err
	}
	var ans struct {
		Accepted bool   `json:"accepted"`
		EventID  string `json:"event_id"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(raw, &ans); err != nil {
		return "", fmt.Errorf("buzz: send answer: %w", err)
	}
	if !ans.Accepted || !hex64.MatchString(ans.EventID) {
		return "", fmt.Errorf("buzz: relay did not accept the message: %s", ans.Message)
	}
	return ans.EventID, nil
}

func (w *Writer) find(ctx context.Context, channel, root, id string) (string, error) {
	raw, err := w.Client.Thread(ctx, channel, root, 500)
	if err != nil {
		return "", err
	}
	evs, err := ParseEvents(raw)
	if err != nil {
		return "", err
	}
	for _, e := range evs {
		if e.Pubkey != w.Self {
			continue
		}
		for _, marker := range outbox.Markers(e.Content) {
			if marker == id {
				return e.ID, nil
			}
		}
	}
	return "", nil
}
