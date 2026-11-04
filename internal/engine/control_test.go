package engine

import (
	"errors"
	"strings"
	"testing"

	"github.com/boraeresici/bhs-coderagent/internal/outbox"
	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

func TestRetryBlockedStartsNextAssignmentOnSameBranch(t *testing.T) {
	f := setup(t)
	id, root := f.start("@coderagent iş")
	f.launch(id)
	if err := f.e.Retry(ctx, id); !errors.Is(err, ErrConflict) {
		t.Fatalf("a working Ticket is not retried: %v", err)
	}
	f.coderSays(root, report("blocked", "human_action: token yok"))
	name := f.ticket(id).Execution.Name
	if err := f.e.Retry(ctx, id); err != nil {
		t.Fatal(err)
	}
	tk := f.ticket(id)
	if len(tk.Assignments) != 2 || tk.LatestAssignment().State != ticket.AssignmentPending || tk.Execution.Name != name {
		t.Fatalf("assignments %+v", tk.Assignments)
	}
	if err := f.e.Retry(ctx, id); !errors.Is(err, ErrConflict) {
		t.Errorf("a second retry before the launch must not queue another: %v", err)
	}
	f.outbox()
	f.launch(id)
	if ms := f.outbox(); len(ms) != 1 || !strings.Contains(ms[0].Body, "Devam") || !strings.Contains(ms[0].Body, "agent/"+name) ||
		!strings.Contains(ms[0].Body, "tekrarlamadan önce dizini, git geçmişini ve GitHub") {
		t.Fatalf("relaunch = %+v", ms)
	}
}

func TestRetryDeliveryResendsNow(t *testing.T) {
	f := setup(t)
	id, _ := f.start("@coderagent iş")
	heads, _ := outbox.Heads(ctx, f.st.DB())
	for n := 1; n <= 3; n++ {
		outbox.Fail(ctx, f.st.DB(), f.now, heads[0].ID, errors.New("down"))
		f.e.SendFailed(ctx, id, n, errors.New("down"))
	}
	if err := f.e.Retry(ctx, id); err != nil {
		t.Fatal(err)
	}
	heads, _ = outbox.Heads(ctx, f.st.DB())
	if f.ticket(id).Delivery != nil || heads[0].Attempts != 0 || heads[0].NextAt.UnixMilli() != 0 {
		t.Fatalf("retry clears the breaker and sends now: %+v", heads[0])
	}
}

func TestStop(t *testing.T) {
	f := setup(t)
	running, root := f.start("@coderagent çalışan iş")
	f.launch(running)
	idle, _ := f.reviewing()
	f.outbox()
	for _, id := range []string{running, idle} {
		if err := f.e.Stop(ctx, id); err != nil {
			t.Fatal(err)
		}
		tk := f.ticket(id)
		if tk.Ended == nil || tk.Ended.By != ticket.EndByHuman || tk.Ended.Outcome != ticket.OutcomeStopped || tk.RunningAssignment() != nil {
			t.Fatalf("stopped = %+v", tk.Ended)
		}
	}
	ms := f.outbox()
	if len(ms) != 2 || len(ms[0].Mentions) != 1 || !strings.Contains(ms[0].Body, "çalışmayı bırak") || len(ms[1].Mentions) != 0 {
		t.Fatalf("stop messages: the running one tells the coder, the idle one only the thread: %+v", ms)
	}
	if err := f.e.Stop(ctx, running); !errors.Is(err, ErrConflict) {
		t.Errorf("stopping twice: %v", err)
	}
	if r := f.receive(f.trigger(msg{actor: human, human: true, root: root, text: "devam et"})); r.Outcome == FannedOut {
		t.Error("a Ticket a person stopped is not resumed by a message")
	}
	if err := f.e.Stop(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

func TestPauseHoldsLaunches(t *testing.T) {
	f := setup(t)
	id, _ := f.start("@coderagent iş")
	f.e.SetPaused(ctx, true)
	var later *LaterError
	if err := f.launch(id); !errors.As(err, &later) || later.Reason != "launching is paused" {
		t.Fatalf("paused launch: %v", err)
	}
	f.e.SetPaused(ctx, false)
	if err := f.launch(id); err != nil || f.ticket(id).RunningAssignment() == nil {
		t.Fatalf("resumed launch: %v", err)
	}
}
