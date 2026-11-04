package buzz

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// KindChat is the Buzz channel message kind. Edits are kind 40003 and never become
// Triggers: an edit does not change an accepted request (ANALIZ.md §9.1 A6).
const KindChat = 9

// Event is a Nostr event as `buzz --format json messages get` returns it (measured on
// the pinned relay, ANALIZ.md §9.1 A1).
type Event struct {
	ID        string     `json:"id"`
	Pubkey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Content   string     `json:"content"`
	Tags      [][]string `json:"tags"`
}

// ParseEvents decodes a messages listing and rejects events without the fields a Trigger
// is built from.
func ParseEvents(raw []byte) ([]Event, error) {
	var evs []Event
	if err := json.Unmarshal(raw, &evs); err != nil {
		return nil, fmt.Errorf("buzz: messages listing: %w", err)
	}
	for _, e := range evs {
		if !hex64.MatchString(e.ID) || !hex64.MatchString(e.Pubkey) || e.CreatedAt <= 0 {
			return nil, fmt.Errorf("buzz: malformed event %q", e.ID)
		}
	}
	return evs, nil
}

func (e *Event) tag(name string) []string {
	for _, t := range e.Tags {
		if len(t) >= 2 && t[0] == name {
			return t
		}
	}
	return nil
}

// Channel is the channel UUID from the "h" tag.
func (e *Event) Channel() string {
	if t := e.tag("h"); t != nil {
		return t[1]
	}
	return ""
}

// Root is the id of the message that starts the event's thread. NIP-10 marks it "root",
// but `buzz messages send --reply-to` marks the root it answers "reply", so a "reply"
// tag counts when there is no "root"; an unmarked "e" is the old positional form. An
// event without "e" tags starts its own thread.
func (e *Event) Root() string {
	var reply, positional string
	for _, t := range e.Tags {
		if len(t) < 2 || t[0] != "e" || !hex64.MatchString(t[1]) {
			continue
		}
		marker := ""
		if len(t) >= 4 {
			marker = t[3]
		}
		switch marker {
		case "root":
			return t[1]
		case "reply":
			if reply == "" {
				reply = t[1]
			}
		case "":
			if positional == "" {
				positional = t[1]
			}
		}
	}
	switch {
	case reply != "":
		return reply
	case positional != "":
		return positional
	}
	return e.ID
}

// Mentions reports whether a "p" tag names pubkey.
func (e *Event) Mentions(pubkey string) bool {
	for _, t := range e.Tags {
		if len(t) >= 2 && t[0] == "p" && t[1] == pubkey {
			return true
		}
	}
	return false
}

// Identity is who the Source reads as and which authors are not people.
type Identity struct {
	// Self is the orchestrator's pubkey; a "p" tag naming it addresses the agent.
	Self string
	// Agents are the BHS agents' pubkeys; with Self, their messages are human:false.
	Agents map[string]bool
}

// Trigger turns a chat event into a Trigger. Its id derives from the event id, so the
// same event read twice is one Trigger. Content stays untrusted text.
func (id Identity) Trigger(e Event) *ticket.Trigger {
	root, ch := e.Root(), e.Channel()
	thread := ticket.ThreadRef{Source: "buzz", ID: ch + ":" + root}
	tr := &ticket.Trigger{
		ID:      ticket.Key("buzz", e.ID),
		Source:  "buzz",
		Threads: []ticket.ThreadRef{thread},
		Actor:   ticket.ActorRef{Source: "buzz", ID: e.Pubkey},
		Human:   e.Pubkey != id.Self && !id.Agents[e.Pubkey],
		At:      time.Unix(e.CreatedAt, 0).UTC(),
		Text:    e.Content,
		Channel: &ticket.ChannelRef{Source: "buzz", Thread: thread.Key(),
			Address: map[string]string{"channel": ch, "reply_to": root}},
		Meta: map[string]any{"event_id": e.ID, "kind": e.Kind},
	}
	if e.Mentions(id.Self) {
		tr.Addressed = ticket.AddressedMention
	}
	return tr
}

// Order sorts events the way the engine must see them: by time, and within one second a
// thread's root before its replies, so a reply never arrives before the Ticket its root
// starts (the relay lists same-second events in no useful order, ANALIZ.md §9.1).
func Order(evs []Event) {
	sort.SliceStable(evs, func(i, j int) bool {
		a, b := evs[i], evs[j]
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		if ar, br := a.Root() == a.ID, b.Root() == b.ID; ar != br {
			return ar
		}
		return a.ID < b.ID
	})
}
