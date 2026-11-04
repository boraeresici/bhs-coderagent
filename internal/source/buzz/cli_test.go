package buzz

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

var ctx = context.Background()

type fake struct {
	args   []string
	stdin  string
	stdout string
	stderr string
	code   int
}

func (f *fake) Run(_ context.Context, args []string, stdin []byte) ([]byte, []byte, int, error) {
	f.args, f.stdin = args, string(stdin)
	return []byte(f.stdout), []byte(f.stderr), f.code, nil
}

const (
	chanID = "c0ffee00-0000-4000-8000-000000000001"
	root   = "1111111111111111111111111111111111111111111111111111111111111111"
	coder  = "2222222222222222222222222222222222222222222222222222222222222222"
)

func TestSendArgsAndStdin(t *testing.T) {
	f := &fake{stdout: `{"id":"x"}`}
	c := &CLI{Runner: f, Relay: "https://buzz.example"}
	content := "@coder `$(rm -rf /)` --channel evil\n\n<!-- coderagent:asg:T:A -->"
	out, err := c.Send(ctx, SendParams{Channel: chanID, ReplyTo: root, Mentions: []string{coder}, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--format", "json", "--relay", "https://buzz.example",
		"messages", "send", "--channel", chanID, "--content", "-", "--reply-to", root, "--mention", coder}
	if !reflect.DeepEqual(f.args, want) {
		t.Errorf("args = %q\nwant  %q", f.args, want)
	}
	if f.stdin != content {
		t.Error("content must go through stdin unchanged, never as an argument")
	}
	if string(out) != `{"id":"x"}` {
		t.Errorf("out = %s", out)
	}
}

func TestRejectsMalformedIDs(t *testing.T) {
	f := &fake{stdout: "{}"}
	c := &CLI{Runner: f}
	for name, p := range map[string]SendParams{
		"channel":  {Channel: "--relay=https://evil", Content: "x"},
		"reply-to": {Channel: chanID, ReplyTo: "-1", Content: "x"},
		"mention":  {Channel: chanID, Mentions: []string{"npub1abc"}, Content: "x"},
		"empty":    {Channel: chanID},
	} {
		if _, err := c.Send(ctx, p); err == nil {
			t.Errorf("%s: malformed input accepted", name)
		}
	}
	if _, err := c.Thread(ctx, chanID, "ABC", 10); err == nil {
		t.Error("uppercase or short event id accepted")
	}
	if f.args != nil {
		t.Fatal("the CLI must not run on malformed input")
	}
}

func TestReadArgs(t *testing.T) {
	f := &fake{stdout: "[]"}
	c := &CLI{Runner: f}
	c.Messages(ctx, chanID, 1762250000, 1762250099, 50)
	if got := strings.Join(f.args, " "); got != "--format json messages get --channel "+chanID+" --limit 50 --since 1762250000 --before 1762250099" {
		t.Errorf("messages args = %s", got)
	}
	c.Mentions(ctx, 0, 20)
	if got := strings.Join(f.args, " "); got != "--format json feed get --types mentions --limit 20" {
		t.Errorf("feed args = %s", got)
	}
	c.Thread(ctx, chanID, root, 100)
	if got := strings.Join(f.args, " "); got != "--format json messages thread --channel "+chanID+" --event "+root+" --limit 100" {
		t.Errorf("thread args = %s", got)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		code      int
		stderr    string
		kind      ErrorKind
		category  string
		retryable bool
	}{
		{2, `{"error":"network","message":"connect refused"}`, ErrNetwork, "network", true},
		{3, `{"error":"auth","message":"not a member"}`, ErrAuth, "auth", false},
		{1, `{"error":"user_error","message":"bad flag","retryable":false}`, ErrInput, "user_error", false},
		{5, `{"error":"conflict","message":"stale"}`, ErrConflict, "conflict", true},
		{2, `{"error":"network","message":"tls","retryable":false}`, ErrNetwork, "network", false},
		{9, `segfault`, ErrOther, "unknown", false},
	}
	for _, c := range cases {
		cli := &CLI{Runner: &fake{code: c.code, stderr: c.stderr}}
		_, err := cli.Messages(ctx, chanID, 0, 0, 1)
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("exit %d: err = %v", c.code, err)
		}
		if e.Kind != c.kind || e.Category != c.category || e.Retryable() != c.retryable {
			t.Errorf("exit %d: %+v retryable=%v", c.code, e, e.Retryable())
		}
	}
	cli := &CLI{Runner: &fake{stdout: "not json"}}
	if _, err := cli.Messages(ctx, chanID, 0, 0, 1); err == nil {
		t.Error("non-JSON stdout must be an error")
	}
}
