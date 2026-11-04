package ticket

import "time"

// Thread is a time-ordered run of Triggers a Source defines (for Buzz: a channel and a
// root message). Subscriptions name the Tickets its Triggers fan out to; a Subscription
// carries context and grants nothing.
type Thread struct {
	Key           string   `json:"key"`
	Subscriptions []string `json:"subscriptions"`
}

// Subscribed reports whether a Ticket subscribes to the Thread.
func (t *Thread) Subscribed(ticketID string) bool {
	for _, id := range t.Subscriptions {
		if id == ticketID {
			return true
		}
	}
	return false
}

// DecisionKind is what an Eval decided.
type DecisionKind string

const (
	DecisionAccept      DecisionKind = "accept"
	DecisionStop        DecisionKind = "stop"
	DecisionIgnore      DecisionKind = "ignore"
	DecisionUnavailable DecisionKind = "unavailable"
)

// Decision is an Eval's judgement on a batch of Triggers. In the MVP the Eval is the
// deterministic gate alone and every batch is one Trigger.
type Decision struct {
	ID         string       `json:"id"`
	Kind       DecisionKind `json:"kind"`
	Reason     []Reason     `json:"reason"`
	Strength   Strength     `json:"strength,omitempty"`
	TicketID   string       `json:"ticketId,omitempty"`
	TriggerIDs []string     `json:"triggerIds"`
	At         time.Time    `json:"at"`
}

// Disposition records where a Trigger went after intake: filtered out by the Source's
// scope, decided by an Eval, or fanned out to the Tickets it reached. It grants nothing.
type Disposition struct {
	Filtered   *Reason   `json:"filtered,omitempty"`
	DecisionID string    `json:"decisionId,omitempty"`
	Fanout     []string  `json:"fanout,omitempty"`
	At         time.Time `json:"at"`
}
