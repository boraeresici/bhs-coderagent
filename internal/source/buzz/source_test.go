package buzz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/boraeresici/bhs-coderagent/internal/store"
	"github.com/boraeresici/bhs-coderagent/internal/ticket"
)

// Pubkeys and channel of the local relay the fixtures were recorded on (scripts/dev-relay.sh).
const (
	humanPK = "4d186827c93ba3441e55489ef1e9c8a28f62526750eda1535355c0cf558e0613"
	orchPK  = "f84fd274c2604556ee01060605d3fb9026ce10e7e360eb4bfa65baca445805dd"
	coderPK = "f72b1bccd44f1fcb602d4ca1b83e3a67a7abbd1d5cee73782d8ec88967e6e8d0"
	devChan = "6d1f11c7-ebb0-461b-810f-df39b67cdbd0"
	rootID  = "c0fc2bce61150c003ef585b7a05f5db08dc79138181ecee3ed3032b65ad20950"
)

var ident = Identity{Self: orchPK, Agents: map[string]bool{coderPK: true}}

func fixture(t *testing.T, name string) []Event {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := ParseEvents(raw)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestRecordedEventsBecomeTriggers(t *testing.T) {
	evs := fixture(t, "messages_get_same_second.json")
	if evs[0].ID == rootID {
		t.Fatal("fixture no longer shows the relay listing a reply before its root")
	}
	Order(evs)
	if evs[0].ID != rootID {
		t.Fatalf("Order must put the root first within a second, got %s", evs[0].ID[:8])
	}

	root := ident.Trigger(evs[0])
	if !root.Human || root.Addressed != ticket.AddressedMention || root.Text != "@coderagent Login zaman aşımını düzelt\n" {
		t.Errorf("root trigger = %+v", root)
	}
	if root.Threads[0].Key() != "buzz:"+devChan+":"+rootID || root.Channel.Thread != root.Threads[0].Key() {
		t.Errorf("root thread = %s / %s", root.Threads[0].Key(), root.Channel.Thread)
	}
	if root.Channel.Address["channel"] != devChan || root.Channel.Address["reply_to"] != rootID {
		t.Errorf("address = %v", root.Channel.Address)
	}
	if root.ID != ticket.Key("buzz", rootID) || !root.At.Equal(time.Unix(1793788590, 0)) {
		t.Errorf("id/at = %s %v", root.ID, root.At)
	}

	for _, e := range evs[1:] {
		tr := ident.Trigger(e)
		if tr.Threads[0] != root.Threads[0] {
			t.Errorf("reply %s not on the root's thread: %v", e.ID[:8], tr.Threads[0])
		}
		if tr.Addressed != ticket.AddressedNone {
			t.Errorf("reply %s is not addressed", e.ID[:8])
		}
		if wantHuman := e.Pubkey == humanPK; tr.Human != wantHuman {
			t.Errorf("reply %s human = %v", e.ID[:8], tr.Human)
		}
	}
}

func TestFeedAndEditFixtures(t *testing.T) {
	feed := fixture(t, "feed_mentions.json")
	if len(feed) != 1 || feed[0].ID != rootID || !feed[0].Mentions(orchPK) {
		t.Errorf("feed = %+v", feed)
	}
	edit := fixture(t, "messages_get_edit_40003.json")
	if len(edit) != 1 || edit[0].Kind != 40003 {
		t.Fatalf("edit fixture = %+v", edit)
	}
}

func TestRoot(t *testing.T) {
	a := "1111111111111111111111111111111111111111111111111111111111111111"
	b := "2222222222222222222222222222222222222222222222222222222222222222"
	self := "3333333333333333333333333333333333333333333333333333333333333333"
	cases := []struct {
		name string
		tags [][]string
		want string
	}{
		{"no e tag starts a thread", nil, self},
		{"root marker wins", [][]string{{"e", b, "", "reply"}, {"e", a, "", "root"}}, a},
		{"reply marker as buzz writes it", [][]string{{"e", b, "", "reply"}}, b},
		{"positional e", [][]string{{"e", a}}, a},
		{"malformed id ignored", [][]string{{"e", "--relay"}}, self},
	}
	for _, c := range cases {
		e := Event{ID: self, Tags: c.tags}
		if got := e.Root(); got != c.want {
			t.Errorf("%s: Root = %s", c.name, got[:4])
		}
	}
}

// relay simulates `messages get` as measured: inclusive bounds, the newest limit events,
// returned oldest first.
type relay struct {
	evs   []Event
	calls int
	fail  error
}

func (r *relay) Messages(_ context.Context, ch string, since, before int64, limit int) (json.RawMessage, error) {
	r.calls++
	if r.fail != nil {
		return nil, r.fail
	}
	var in []Event
	for _, e := range r.evs {
		if e.CreatedAt >= since && (before == 0 || e.CreatedAt <= before) {
			in = append(in, e)
		}
	}
	sort.SliceStable(in, func(i, j int) bool { return in[i].CreatedAt > in[j].CreatedAt })
	if len(in) > limit {
		in = in[:limit]
	}
	sort.SliceStable(in, func(i, j int) bool { return in[i].CreatedAt < in[j].CreatedAt })
	return json.Marshal(in)
}

func chat(n int, at int64) Event {
	id := fmt.Sprintf("%064x", n)
	return Event{ID: id, Pubkey: humanPK, CreatedAt: at, Kind: KindChat, Content: "m" + strconv.Itoa(n),
		Tags: [][]string{{"h", devChan}}}
}

type rig struct {
	src  *Source
	rel  *relay
	got  []string
	now  time.Time
	fail error
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := &rig{rel: &relay{}, now: time.Unix(1000, 0)}
	r.src = &Source{Reader: r.rel, DB: st.DB(), Identity: ident, Channels: []string{devChan}, Now: func() time.Time { return r.now }}
	return r
}

func (r *rig) sync(t *testing.T) error {
	t.Helper()
	return r.src.Sync(context.Background(), func(_ context.Context, tr *ticket.Trigger) error {
		if r.fail != nil {
			return r.fail
		}
		r.got = append(r.got, tr.Text)
		return nil
	})
}

func TestFirstSyncSkipsHistory(t *testing.T) {
	r := newRig(t)
	r.rel.evs = []Event{chat(1, 900), chat(2, 999)}
	if err := r.sync(t); err != nil || len(r.got) != 0 || r.rel.calls != 0 {
		t.Fatalf("first sync read history: got=%v calls=%d err=%v", r.got, r.rel.calls, err)
	}
	r.rel.evs = append(r.rel.evs, chat(3, 1001))
	if err := r.sync(t); err != nil {
		t.Fatal(err)
	}
	// 999 falls inside the overlap and is read again; the engine sees it as a Duplicate.
	if fmt.Sprint(r.got) != "[m2 m3]" {
		t.Fatalf("got %v", r.got)
	}
}

func TestPagesBackWhenAPageIsFull(t *testing.T) {
	r := newRig(t)
	r.sync(t) // cursor = 1000
	for i := 0; i < 250; i++ {
		r.rel.evs = append(r.rel.evs, chat(i, 1001+int64(i/3)))
	}
	if err := r.sync(t); err != nil {
		t.Fatal(err)
	}
	if len(r.got) != 250 || r.got[0] != "m0" || r.got[249] != "m249" {
		t.Fatalf("read %d events (first %v): the relay's newest-first limit dropped some", len(r.got), r.got[:1])
	}
}

func TestSecondOverflow(t *testing.T) {
	r := newRig(t)
	r.sync(t)
	for i := 0; i < PageSize+1; i++ {
		r.rel.evs = append(r.rel.evs, chat(i, 1001))
	}
	if err := r.sync(t); !errors.Is(err, ErrSecondOverflow) {
		t.Fatalf("err = %v", err)
	}
}

func TestCursorHoldsOnFailure(t *testing.T) {
	r := newRig(t)
	r.sync(t)
	r.rel.evs = []Event{chat(1, 1010)}
	r.fail = errors.New("store busy")
	if err := r.sync(t); err == nil {
		t.Fatal("receive error must surface")
	}
	r.fail = nil
	r.now = time.Unix(5000, 0)
	if err := r.sync(t); err != nil || fmt.Sprint(r.got) != "[m1]" {
		t.Fatalf("event lost after a failed receive: %v %v", r.got, err)
	}
}

func TestSkipsOtherKindsAndChannels(t *testing.T) {
	r := newRig(t)
	r.sync(t)
	edit := chat(1, 1001)
	edit.Kind = 40003
	other := chat(2, 1001)
	other.Tags = [][]string{{"h", "aaaaaaaa-0000-4000-8000-000000000000"}}
	r.rel.evs = []Event{edit, other, chat(3, 1002)}
	if err := r.sync(t); err != nil || fmt.Sprint(r.got) != "[m3]" {
		t.Fatalf("got %v %v", r.got, err)
	}
}
