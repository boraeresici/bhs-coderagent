package ticket

import (
	"regexp"
	"strings"
)

// Reason is a structured reason code with template values; TriggerID names the Trigger of
// a batch it applies to. Gates and Source scopes may emit their own literal codes.
type Reason struct {
	Code      string            `json:"code"`
	Values    map[string]string `json:"values,omitempty"`
	TriggerID string            `json:"triggerId,omitempty"`
}

// Reason templates. The upstream codes keep their upstream names (src/core/reasons.ts);
// codes only this project uses follow them.
var reasonTemplates = map[string]string{
	"said":            "{text}",
	"noAdmissible":    "No admissible trigger",
	"startedTicket":   "Already started ticket {ticket}",
	"acceptedBefore":  "Trigger {trigger} was accepted before",
	"humanStop":       "Human asked to stop",
	"forwarded":       "Forwarded to the worker of the Ticket already on this Thread",
	"senderNotMember": "The sender is not on the Source's list",
	"retried":         "Retried by a person",
	"hostRestarted":   "bhs-coderagent restarted",
	"stopUnknown":     "Stop target is unknown or already ended",
	"stopNotExplicit": "A stop needs an explicit request from a sender on the list",
	"notAddressed":    "The message does not mention the agent",
	"channelNotHeard": "The channel is not one the Source listens to",
	"ownMessage":      "The agent's own message",
	"loginNotAllowed": "The GitHub login is not on the repository's allowed_logins",
}

// Said wraps free text as a Reason.
func Said(text string) Reason { return Reason{Code: "said", Values: map[string]string{"text": text}} }

var placeholder = regexp.MustCompile(`\{(\w+)\}`)

// ReasonText formats reasons in English for agent consumption, prefixed by TriggerID when set.
func ReasonText(reasons ...Reason) string {
	parts := make([]string, 0, len(reasons))
	for _, r := range reasons {
		tmpl, ok := reasonTemplates[r.Code]
		if !ok {
			tmpl = r.Code
		}
		text := placeholder.ReplaceAllStringFunc(tmpl, func(m string) string {
			if v, ok := r.Values[m[1:len(m)-1]]; ok {
				return v
			}
			return m
		})
		if r.TriggerID != "" {
			text = r.TriggerID + ": " + text
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "; ")
}
