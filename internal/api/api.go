// Package api is the orchestrator's HTTP API under /orchestrator/api/ (ANALIZ.md §5.9):
// the BHS panel's Tickets page reads it and acts through it (retry, stop, pause; F6.3).
// Every request needs the bearer token; the API is not privileged beyond the
// orchestrator's own store and engine.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/engine"
	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// Prefix is where the API is mounted.
const Prefix = "/orchestrator/api/"

// Actions are what a person may do from the panel; the engine provides them.
type Actions interface {
	Retry(ctx context.Context, ticketID string) error
	Stop(ctx context.Context, ticketID string) error
	SetPaused(ctx context.Context, paused bool) error
	Paused(ctx context.Context) (bool, error)
}

// New returns the API handler. An empty token is refused: an API without one would let
// anyone on the container network read every Ticket and stop them.
func New(st *store.Store, act Actions, token string) (http.Handler, error) {
	if token == "" {
		return nil, errors.New("api: no bearer token")
	}
	mux := http.NewServeMux()
	h := &handler{st: st, act: act}
	mux.HandleFunc("GET "+Prefix+"state", h.state)
	mux.HandleFunc("GET "+Prefix+"tickets", h.tickets)
	mux.HandleFunc("GET "+Prefix+"tickets/{id}", h.ticket)
	mux.HandleFunc("POST "+Prefix+"tickets/{id}/retry", h.do(act.Retry))
	mux.HandleFunc("POST "+Prefix+"tickets/{id}/stop", h.do(act.Stop))
	mux.HandleFunc("POST "+Prefix+"pause", h.pause(true))
	mux.HandleFunc("POST "+Prefix+"resume", h.pause(false))
	return bearer(token, mux), nil
}

func bearer(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="coderagent"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type handler struct {
	st  *store.Store
	act Actions
}

// TicketView is a Ticket as the panel shows it: its derived stage and what a person
// needs to act on, without its Inputs or untrusted message text.
type TicketView struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	ShortName string            `json:"short_name"`
	Branch    string            `json:"branch"`
	Stage     string            `json:"stage"`
	Requester string            `json:"requester"`
	Thread    string            `json:"thread"`
	Report    *ticket.Report    `json:"report,omitempty"`
	Attention *ticket.Attention `json:"attention,omitempty"`
	Follows   []ticket.Follow   `json:"follows,omitempty"`
	Ended     *ticket.End       `json:"ended,omitempty"`
	CanRetry  bool              `json:"can_retry"`
	Inputs    int               `json:"inputs"`
	Undeliv   int               `json:"undelivered"`
	Attempts  int               `json:"assignments"`
	Hops      int               `json:"hops"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func view(t *ticket.Ticket) TicketView {
	v := TicketView{
		ID: t.ID, Title: t.Title, Stage: ticket.StageOf(t).String(), Requester: t.Mandate.Actor.ID,
		Attention: t.Attention, Follows: t.Follows, Ended: t.Ended, CanRetry: ticket.CanRetry(t),
		Inputs: len(t.Inputs), Undeliv: len(t.Undelivered), Attempts: len(t.Assignments), Hops: t.Hops,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
	if t.Execution != nil {
		v.ShortName, v.Branch = t.Execution.Name, "agent/"+t.Execution.Name
	}
	if len(t.Channels) > 0 {
		v.Thread = t.Channels[0].Thread
	}
	if a := t.LatestAssignment(); a != nil {
		v.Report = a.Report
	}
	return v
}

// state answers the panel's first question: how many Tickets wait on a person, work, or
// sit idle (upstream docs/005 "Needs a person").
func (h *handler) state(w http.ResponseWriter, r *http.Request) {
	open, err := store.OpenTickets(r.Context(), h.st.DB())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stages := map[string]int{}
	person := 0
	for _, t := range open {
		s := ticket.StageOf(t)
		stages[s.String()]++
		if s.Kind == ticket.StagePerson {
			person++
		}
	}
	paused, err := h.act.Paused(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"open": len(open), "needs_person": person, "stages": stages, "paused": paused})
}

// tickets lists open Tickets; filter=person keeps those waiting on a person, most urgent
// action first (delivery, answer, review, blocked, failed), then oldest.
func (h *handler) tickets(w http.ResponseWriter, r *http.Request) {
	open, err := store.OpenTickets(r.Context(), h.st.DB())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	filter := r.URL.Query().Get("filter")
	if filter != "" && filter != "person" {
		writeError(w, http.StatusBadRequest, "filter must be person")
		return
	}
	out := []TicketView{}
	for _, t := range open {
		if filter == "person" && ticket.StageOf(t).Kind != ticket.StagePerson {
			continue
		}
		out = append(out, view(t))
	}
	if filter == "person" {
		rank := map[string]int{ticket.ActionDelivery: 0, ticket.ActionAnswer: 1, ticket.ActionReview: 2, ticket.ActionBlocked: 3, ticket.ActionFailed: 4}
		sort.SliceStable(out, func(i, j int) bool {
			_, ai, _ := strings.Cut(out[i].Stage, "/")
			_, aj, _ := strings.Cut(out[j].Stage, "/")
			return rank[ai] < rank[aj]
		})
	}
	writeJSON(w, map[string]any{"tickets": out})
}

func (h *handler) ticket(w http.ResponseWriter, r *http.Request) {
	t, err := store.GetTicket(r.Context(), h.st.DB(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such ticket")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, view(t))
}

// do runs a Ticket action and answers with the Ticket as it is afterwards.
func (h *handler) do(action func(context.Context, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := action(r.Context(), r.PathValue("id"))
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "no such ticket")
			return
		case errors.Is(err, engine.ErrConflict):
			writeError(w, http.StatusConflict, "the ticket's stage does not allow this")
			return
		case err != nil:
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		h.ticket(w, r)
	}
}

func (h *handler) pause(paused bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.act.SetPaused(r.Context(), paused); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"paused": paused})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	writeJSONBody(w, v)
}

func writeJSONBody(w http.ResponseWriter, v any) { json.NewEncoder(w).Encode(v) }

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
