package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/store"
)

// Health answers GET /healthz for the container platform (Coolify): 200 while the store
// answers and a Buzz poll succeeded within Quiet, 503 otherwise. It needs no token and
// says nothing about Tickets.
type Health struct {
	DB      *sql.DB
	Version string
	// Quiet is the longest time without a successful poll that still counts as healthy.
	Quiet time.Duration
	Now   func() time.Time

	started time.Time
	last    atomic.Int64 // unix ms of the last successful poll
}

// NewHealth returns a Health whose quiet period starts now.
func NewHealth(db *sql.DB, version string, quiet time.Duration, now func() time.Time) *Health {
	return &Health{DB: db, Version: version, Quiet: quiet, Now: now, started: now()}
}

// Beat records a successful poll.
func (h *Health) Beat() { h.last.Store(h.Now().UnixMilli()) }

func (h *Health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{"ok": true, "version": h.Version, "schema": store.SchemaVersion}
	since := h.started
	if ms := h.last.Load(); ms != 0 {
		since = time.UnixMilli(ms)
		body["last_poll"] = since.UTC()
	}
	var one int
	switch {
	case h.DB.QueryRowContext(r.Context(), "SELECT 1").Scan(&one) != nil:
		body["ok"], body["reason"] = false, "store does not answer"
	case h.Now().Sub(since) > h.Quiet:
		body["ok"], body["reason"] = false, fmt.Sprintf("no successful Buzz poll for %s", h.Now().Sub(since).Round(time.Second))
	}
	w.Header().Set("Content-Type", "application/json")
	if body["ok"] == false {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeJSONBody(w, body)
}
