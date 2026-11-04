package engine

import (
	"errors"
	"testing"

	"github.com/boraeresici/bhs-coderagent/internal/effects"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

func TestBreakerAfterThreeFailedSends(t *testing.T) {
	f := setup(t)
	id, _ := f.start("@coderagent iş")
	cause := errors.New("buzz: exit 2 (network): relay down")
	for n := 1; n <= 2; n++ {
		f.e.SendFailed(ctx, id, n, cause)
		if f.ticket(id).Delivery != nil {
			t.Fatalf("after %d failures the Ticket must not wait on a person yet", n)
		}
	}
	f.e.SendFailed(ctx, id, 3, cause)
	tk := f.ticket(id)
	if s := ticket.StageOf(tk).String(); s != "person/delivery" || tk.Delivery.Error != cause.Error() || !ticket.CanRetry(tk) {
		t.Fatalf("stage %s, delivery %+v", s, tk.Delivery)
	}
	if err := f.e.SendSucceeded(ctx, id); err != nil {
		t.Fatal(err)
	}
	if tk := f.ticket(id); tk.Delivery != nil || ticket.StageOf(tk).String() == "person/delivery" {
		t.Fatalf("a successful send clears the breaker: %+v", tk.Delivery)
	}
}

func TestDeadLaunchWaitsOnPerson(t *testing.T) {
	f := setup(t)
	id, _ := f.start("@coderagent iş")
	a := f.ticket(id).LatestAssignment().ID
	if err := f.e.EffectDead(ctx, effects.Launch, id, a, errors.New("disk full")); err != nil {
		t.Fatal(err)
	}
	tk := f.ticket(id)
	if s := ticket.StageOf(tk).String(); s != "person/failed" || !ticket.CanRetry(tk) {
		t.Fatalf("a dead launch must not look queued: %s", s)
	}
}
