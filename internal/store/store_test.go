package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

var ctx = context.Background()

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOpenLocksDirectory(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	if _, err := Open(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Open = %v, want ErrLocked", err)
	}
	s.Close()
	open(t, dir).Close() // the lock is released on Close
}

func TestRefusesNewerSchema(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	if _, err := s.DB().Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := Open(dir); err == nil {
		t.Fatal("Open must refuse a schema newer than the build")
	}
}

func TestReopenKeepsData(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	tk := &ticket.Ticket{ID: "T1", Title: "fix", CreatedAt: now, Execution: &ticket.Execution{Name: "fix-login"}}
	if err := PutTicket(ctx, s.DB(), tk); err != nil {
		t.Fatal(err)
	}
	if err := SetKV(ctx, s.DB(), "cursor:c1", "42"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s = open(t, dir)
	defer s.Close()
	got, err := GetTicket(ctx, s.DB(), "T1")
	if err != nil || got.Execution.Name != "fix-login" || !got.CreatedAt.Equal(now) {
		t.Fatalf("GetTicket = %+v, %v", got, err)
	}
	if v, ok, err := GetKV(ctx, s.DB(), "cursor:c1"); v != "42" || !ok || err != nil {
		t.Fatalf("GetKV = %q %v %v", v, ok, err)
	}
	if _, ok, _ := GetKV(ctx, s.DB(), "missing"); ok {
		t.Fatal("unset key reported as set")
	}
	if _, err := GetTicket(ctx, s.DB(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing ticket err = %v", err)
	}
}

func TestOpenTicketsUsesGeneratedColumn(t *testing.T) {
	s := open(t, t.TempDir())
	defer s.Close()
	base := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"A", "B", "C"} {
		tk := &ticket.Ticket{ID: id, CreatedAt: base.Add(time.Duration(i) * time.Minute)}
		if id == "B" {
			tk.Ended = &ticket.End{Outcome: ticket.OutcomeMerged, By: ticket.EndByExecutor}
		}
		if err := PutTicket(ctx, s.DB(), tk); err != nil {
			t.Fatal(err)
		}
	}
	open, err := OpenTickets(ctx, s.DB())
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 || open[0].ID != "A" || open[1].ID != "C" {
		t.Fatalf("OpenTickets = %v", ids(open))
	}
}

func TestShortNameIsUnique(t *testing.T) {
	s := open(t, t.TempDir())
	defer s.Close()
	for _, id := range []string{"T1", "T2"} {
		err := PutTicket(ctx, s.DB(), &ticket.Ticket{ID: id, Execution: &ticket.Execution{Name: "same"}})
		if id == "T2" && err == nil {
			t.Fatal("two Tickets must not share a short name: it names their branch agent/<short>")
		}
	}
}

func TestTriggerDuplicate(t *testing.T) {
	s := open(t, t.TempDir())
	defer s.Close()
	tr := &ticket.Trigger{ID: ticket.Key("buzz", "e1"), Source: "buzz",
		Threads: []ticket.ThreadRef{{Source: "buzz", ID: "c1:root"}}, Text: "first"}
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		if r, err := PutTrigger(ctx, tx, tr); r != TriggerStored || err != nil {
			t.Fatalf("first = %s %v", r, err)
		}
		tr.Text = "changed"
		if r, err := PutTrigger(ctx, tx, tr); r != TriggerDuplicate || err != nil {
			t.Fatalf("second = %s %v", r, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var key, body string
	s.DB().QueryRow(`SELECT thread_key, body FROM trigger`).Scan(&key, &body)
	if key != "buzz:c1:root" {
		t.Errorf("thread_key = %q", key)
	}
	if want := `"text":"first"`; !strings.Contains(body, want) {
		t.Errorf("a duplicate must not overwrite the stored Trigger: %s", body)
	}
}

func ids(ts []*ticket.Ticket) (out []string) {
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return
}
