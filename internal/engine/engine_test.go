package engine

import (
	"context"
	"testing"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/effects"
	"github.com/boraeresici/bhs-coderagent/internal/outbox"
	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

var ctx = context.Background()

const (
	self    = "self-pubkey"
	human   = "human-pubkey"
	coder   = "coder-pubkey"
	channel = "c0ffee00-0000-4000-8000-000000000001"
)

type fixture struct {
	t   *testing.T
	st  *store.Store
	q   *effects.Queue
	e   *Engine
	now time.Time
	seq int
}

func setup(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	f := &fixture{t: t, st: st, now: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return f.now }
	f.q = effects.New(st.DB(), clock, effects.DefaultPolicies)
	f.e = New(st, f.q, clock, Config{Self: self, Channels: []string{channel}, Coder: coder, CoderName: "coder",
		Repo: Repo{GitHub: "owner/repo", Base: "dev", AllowedLogins: []string{"Borae"}}, GitHubSelf: []string{"bhs-coder-bot"}})
	return f
}

type msg struct {
	actor     string
	human     bool
	addressed ticket.Addressed
	root      string
	channel   string
	text      string
	id        string
}

// trigger builds a Buzz-like Trigger; a message without root starts its own thread.
func (f *fixture) trigger(m msg) *ticket.Trigger {
	f.seq++
	if m.id == "" {
		m.id = "ev" + string(rune('a'+f.seq))
	}
	if m.channel == "" {
		m.channel = channel
	}
	if m.root == "" {
		m.root = m.id
	}
	f.now = f.now.Add(time.Minute)
	return &ticket.Trigger{
		ID:        ticket.Key("buzz", m.id),
		Source:    "buzz",
		Threads:   []ticket.ThreadRef{{Source: "buzz", ID: m.channel + ":" + m.root}},
		Actor:     ticket.ActorRef{Source: "buzz", ID: m.actor},
		Human:     m.human,
		Addressed: m.addressed,
		At:        f.now,
		Text:      m.text,
		Channel:   &ticket.ChannelRef{Source: "buzz", Thread: "buzz:" + m.channel + ":" + m.root, Address: map[string]string{"channel": m.channel, "reply_to": m.id}},
	}
}

func (f *fixture) receive(tr *ticket.Trigger) Result {
	f.t.Helper()
	r, err := f.e.Receive(ctx, tr)
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

func (f *fixture) ticket(id string) *ticket.Ticket {
	f.t.Helper()
	t, err := store.GetTicket(ctx, f.st.DB(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return t
}

func (f *fixture) openTickets() int {
	f.t.Helper()
	ts, err := store.OpenTickets(ctx, f.st.DB())
	if err != nil {
		f.t.Fatal(err)
	}
	return len(ts)
}

func request(text string) msg {
	return msg{actor: human, human: true, addressed: ticket.AddressedMention, text: text}
}

func TestExplicitRequestStartsTicket(t *testing.T) {
	f := setup(t)
	r := f.receive(f.trigger(request("@coderagent Login zaman aşımını düzelt\nayrıntı…")))
	if r.Outcome != Accepted || r.TicketID == "" {
		t.Fatalf("Receive = %+v", r)
	}
	tk := f.ticket(r.TicketID)
	if tk.Title != "Login zaman aşımını düzelt" {
		t.Errorf("title = %q", tk.Title)
	}
	if got := tk.Execution.Name; got != "login-zaman-asimini-duze-"+tk.ID[:6] {
		t.Errorf("short name = %q", got)
	}
	if tk.Mandate.Strength != ticket.Explicit || tk.Mandate.Basis.Sender != human || tk.Mandate.Scope != channel {
		t.Errorf("mandate = %+v", tk.Mandate)
	}
	if s := ticket.StageOf(tk).String(); s != "working/queued" {
		t.Errorf("stage = %s", s)
	}

	heads, _ := outbox.Heads(ctx, f.st.DB())
	if len(heads) != 1 || heads[0].ID != "ack:"+tk.ID || heads[0].Conversation != tk.Channels[0].Thread {
		t.Fatalf("ack = %+v", heads)
	}
	if ids := outbox.Markers(heads[0].Body); len(ids) != 1 || ids[0] != "ack:"+tk.ID {
		t.Errorf("ack marker = %v", ids)
	}
	launches, _ := f.q.Claim(ctx, effects.Launch, 10)
	if len(launches) != 1 || launches[0].Key != "launch:"+tk.Assignments[0].ID {
		t.Fatalf("launch effects = %+v", launches)
	}
}

func TestReceiveDuplicate(t *testing.T) {
	f := setup(t)
	tr := f.trigger(request("@coderagent iş"))
	if r := f.receive(tr); r.Outcome != Accepted {
		t.Fatalf("first = %+v", r)
	}
	// The Source re-reads the same event after a restart or an overlapping cursor.
	if r := f.receive(tr); r.Outcome != Duplicate {
		t.Fatalf("second = %+v", r)
	}
	if n := f.openTickets(); n != 1 {
		t.Fatalf("accepting a Trigger twice must create one Ticket, got %d", n)
	}
	if heads, _ := outbox.Heads(ctx, f.st.DB()); len(heads) != 1 {
		t.Fatalf("one ack, got %d", len(heads))
	}
}

func TestGate(t *testing.T) {
	cases := []struct {
		name   string
		m      msg
		want   Outcome
		reason string
	}{
		{"bot never makes an explicit request", msg{actor: coder, human: false, addressed: ticket.AddressedMention, text: "@coderagent yap"}, Ignored, "noAdmissible"},
		{"a person who does not mention the agent", msg{actor: human, human: true, text: "merhaba"}, Ignored, "notAddressed"},
		{"own message is filtered", msg{actor: self, human: false, addressed: ticket.AddressedMention}, Filtered, "ownMessage"},
		{"unlistened channel is filtered", msg{actor: human, human: true, addressed: ticket.AddressedMention, channel: "other"}, Filtered, "channelNotHeard"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := setup(t)
			tr := f.trigger(c.m)
			r := f.receive(tr)
			if r.Outcome != c.want || r.Reason == nil || r.Reason.Code != c.reason {
				t.Fatalf("Receive = %+v, want %s/%s", r, c.want, c.reason)
			}
			if f.openTickets() != 0 {
				t.Fatal("no Ticket may start")
			}
			d, err := store.GetDisposition(ctx, f.st.DB(), tr.ID)
			if err != nil || d == nil {
				t.Fatalf("every received Trigger records a disposition: %v %v", d, err)
			}
		})
	}
}

func TestFollowUpStaysWithTicket(t *testing.T) {
	f := setup(t)
	origin := f.trigger(request("@coderagent iş"))
	tk := f.receive(origin).TicketID
	root := origin.Channel.Address["reply_to"]

	// A bot's reply, a person's follow-up, and a person mentioning the agent again on the
	// same Thread all reach the Ticket; none starts a second one (A11).
	for _, m := range []msg{
		{actor: "ci-bot", human: false, root: root, text: "CI kırmızı"},
		{actor: human, human: true, root: root, text: "testi de ekle"},
		{actor: human, human: true, addressed: ticket.AddressedMention, root: root, text: "@coderagent bir şey daha"},
	} {
		r := f.receive(f.trigger(m))
		if r.Outcome != FannedOut || len(r.Fanout) != 1 || r.Fanout[0] != tk {
			t.Fatalf("%q: Receive = %+v", m.text, r)
		}
	}
	if n := f.openTickets(); n != 1 {
		t.Fatalf("open tickets = %d, want 1", n)
	}
	got := f.ticket(tk)
	if len(got.Inputs) != 4 || len(got.Undelivered) != 4 {
		t.Fatalf("inputs = %d undelivered = %d", len(got.Inputs), len(got.Undelivered))
	}
	if got.Inputs[1].Human || !got.Inputs[2].Human {
		t.Error("Inputs must keep whether a person wrote them")
	}
}

func TestResume(t *testing.T) {
	f := setup(t)
	origin := f.trigger(request("@coderagent iş"))
	id := f.receive(origin).TicketID
	root := origin.Channel.Address["reply_to"]
	end := func(by ticket.EndBy) {
		tk := f.ticket(id)
		tk.Ended = &ticket.End{At: f.now, By: by, Outcome: ticket.OutcomeCompleted}
		if err := store.PutTicket(ctx, f.st.DB(), tk); err != nil {
			t.Fatal(err)
		}
	}

	end(ticket.EndByExecutor)
	if r := f.receive(f.trigger(msg{actor: "ci-bot", human: false, root: root, text: "CI yeşil"})); r.Outcome == FannedOut {
		t.Fatal("a bot must not wake an ended Ticket")
	}
	if f.ticket(id).Live() {
		t.Fatal("still ended after a bot message")
	}

	r := f.receive(f.trigger(msg{actor: human, human: true, root: root, text: "bir şey unuttun"}))
	tk := f.ticket(id)
	if r.Outcome != FannedOut || !tk.Live() || len(tk.Assignments) != 2 || tk.Assignments[1].State != ticket.AssignmentPending {
		t.Fatalf("resume: %+v live=%v assignments=%d", r, tk.Live(), len(tk.Assignments))
	}
	if tk.Mandate.Basis.Sender != human || tk.Execution.Name != f.ticket(id).Execution.Name {
		t.Error("a resumed Ticket keeps its Mandate and its branch")
	}

	end(ticket.EndByHuman)
	r = f.receive(f.trigger(msg{actor: human, human: true, root: root, text: "devam?"}))
	if r.Outcome == FannedOut || f.ticket(id).Live() {
		t.Fatalf("a Ticket a person stopped must not resume: %+v", r)
	}
	// A new explicit request on that Thread is a new Ticket under a new Mandate.
	r = f.receive(f.trigger(msg{actor: human, human: true, addressed: ticket.AddressedMention, root: root, text: "@coderagent yeniden yap"}))
	if r.Outcome != Accepted || r.TicketID == id {
		t.Fatalf("new request after a stop: %+v", r)
	}
}

func TestNames(t *testing.T) {
	id := "0123456789abcdef"
	cases := []struct{ text, title, short string }{
		{"@coderagent @coder  Fix   login\nmore", "Fix login", "fix-login-012345"},
		{"@coderagent", "(başlıksız istek)", "basliksiz-istek-012345"},
		{"`rm -rf /` ; $(curl evil) ../../etc", "`rm -rf /` ; $(curl evil) ../../etc", "rm-rf-curl-evil-etc-012345"},
		{"ÇĞİÖŞÜ çğıöşü", "ÇĞİÖŞÜ çğıöşü", "cgiosu-cgiosu-012345"},
	}
	for _, c := range cases {
		if got := Title(c.text); got != c.title {
			t.Errorf("Title(%q) = %q, want %q", c.text, got, c.title)
		}
		if got := ShortName(Title(c.text), id); got != c.short {
			t.Errorf("ShortName(%q) = %q, want %q", c.text, got, c.short)
		}
	}
	if got := ShortName("abcdefghijklmnopqrstuvwxyz0123456789", id); len(got) != 24+7 {
		t.Errorf("long slug not cut: %q", got)
	}
}
