// Package buzz is the Buzz Source: it reads channel messages through the buzz CLI, turns
// them into Triggers, and writes the orchestrator's messages back (ANALIZ.md §5.2, §9.1).
//
// The CLI is the contract, as upstream Sources talk to their hosts through CLIs: the
// subcommands and flags used here are those of the buzz binary pinned in
// buzz-hermes-stack (measured 2026-11-04, ANALIZ.md §9.1 A2).
package buzz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

// Runner runs the buzz binary. Production uses Exec; tests use a fake.
type Runner interface {
	Run(ctx context.Context, args []string, stdin []byte) (stdout, stderr []byte, code int, err error)
}

// Exec runs the binary at Path without a shell. The identity comes from the process
// environment (BUZZ_PRIVATE_KEY), never from the command line, where ps would show it.
type Exec struct{ Path string }

// Run implements Runner.
func (x Exec) Run(ctx context.Context, args []string, stdin []byte) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, x.Path, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.Bytes(), errb.Bytes(), exit.ExitCode(), nil
	}
	return out.Bytes(), errb.Bytes(), 0, err
}

// ErrorKind classifies a CLI failure by its exit code.
type ErrorKind int

const (
	ErrInput    ErrorKind = 1 // bad input: retrying cannot help
	ErrNetwork  ErrorKind = 2 // relay or network: back off and retry
	ErrAuth     ErrorKind = 3 // identity or membership: a person must act
	ErrOther    ErrorKind = 4
	ErrConflict ErrorKind = 5 // write conflict: re-read, then retry
)

// Error is a failed CLI call with the JSON error body the CLI writes to stderr.
type Error struct {
	Kind      ErrorKind
	Category  string
	Message   string
	retryable *bool
}

func (e *Error) Error() string {
	return fmt.Sprintf("buzz: exit %d (%s): %s", e.Kind, e.Category, e.Message)
}

// Retryable reports whether trying again later may succeed. The CLI's own "retryable"
// flag wins when present; otherwise network errors and write conflicts are retryable.
func (e *Error) Retryable() bool {
	if e.retryable != nil {
		return *e.retryable
	}
	return e.Kind == ErrNetwork || e.Kind == ErrConflict
}

// CLI calls buzz with JSON output.
type CLI struct {
	Runner Runner
	// Relay is the relay's https:// URL; empty uses BUZZ_RELAY_URL.
	Relay string
}

var (
	uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	hex64  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// SendParams is one message to post.
type SendParams struct {
	Channel string
	// ReplyTo is the event the message answers; for a thread reply, its root.
	ReplyTo string
	// Mentions are pubkeys (hex) to tag; a tag needs no display-name resolution.
	Mentions []string
	Content  string
}

// Send posts a message and returns the CLI's JSON answer (it carries the new event).
func (c *CLI) Send(ctx context.Context, p SendParams) (json.RawMessage, error) {
	if err := check("channel", p.Channel, uuidRE); err != nil {
		return nil, err
	}
	args := []string{"messages", "send", "--channel", p.Channel, "--content", "-"}
	if p.ReplyTo != "" {
		if err := check("reply-to", p.ReplyTo, hex64); err != nil {
			return nil, err
		}
		args = append(args, "--reply-to", p.ReplyTo)
	}
	for _, m := range p.Mentions {
		if err := check("mention", m, hex64); err != nil {
			return nil, err
		}
		args = append(args, "--mention", m)
	}
	if p.Content == "" {
		return nil, errors.New("buzz: empty message")
	}
	return c.run(ctx, args, []byte(p.Content))
}

// Messages lists a channel's messages after since (unix seconds; 0 for no bound).
func (c *CLI) Messages(ctx context.Context, channel string, since int64, limit int) (json.RawMessage, error) {
	if err := check("channel", channel, uuidRE); err != nil {
		return nil, err
	}
	args := []string{"messages", "get", "--channel", channel, "--limit", strconv.Itoa(limit)}
	if since > 0 {
		args = append(args, "--since", strconv.FormatInt(since, 10))
	}
	return c.run(ctx, args, nil)
}

// Thread lists the replies to a root message.
func (c *CLI) Thread(ctx context.Context, channel, root string, limit int) (json.RawMessage, error) {
	if err := check("channel", channel, uuidRE); err != nil {
		return nil, err
	}
	if err := check("event", root, hex64); err != nil {
		return nil, err
	}
	return c.run(ctx, []string{"messages", "thread", "--channel", channel, "--event", root, "--limit", strconv.Itoa(limit)}, nil)
}

// Mentions lists the identity's own mentions after since (unix seconds).
func (c *CLI) Mentions(ctx context.Context, since int64, limit int) (json.RawMessage, error) {
	args := []string{"feed", "get", "--types", "mentions", "--limit", strconv.Itoa(limit)}
	if since > 0 {
		args = append(args, "--since", strconv.FormatInt(since, 10))
	}
	return c.run(ctx, args, nil)
}

func (c *CLI) run(ctx context.Context, args []string, stdin []byte) (json.RawMessage, error) {
	full := []string{"--format", "json"}
	if c.Relay != "" {
		full = append(full, "--relay", c.Relay)
	}
	full = append(full, args...)
	out, errOut, code, err := c.Runner.Run(ctx, full, stdin)
	if err != nil {
		return nil, fmt.Errorf("buzz: %w", err)
	}
	if code != 0 {
		return nil, parseError(code, errOut)
	}
	if !json.Valid(out) {
		return nil, &Error{Kind: ErrOther, Category: "bad_output", Message: "stdout is not JSON"}
	}
	return json.RawMessage(out), nil
}

func parseError(code int, stderr []byte) *Error {
	e := &Error{Kind: ErrorKind(code), Category: "unknown", Message: string(bytes.TrimSpace(stderr))}
	if e.Kind < ErrInput || e.Kind > ErrConflict {
		e.Kind = ErrOther
	}
	var body struct {
		Error     string `json:"error"`
		Message   string `json:"message"`
		Retryable *bool  `json:"retryable"`
	}
	if json.Unmarshal(bytes.TrimSpace(stderr), &body) == nil && body.Error != "" {
		e.Category, e.Message, e.retryable = body.Error, body.Message, body.Retryable
	}
	return e
}

// check rejects values that do not have their expected shape before they reach the
// command line: an id never starts with '-', so it can never be read as a flag.
func check(name, v string, re *regexp.Regexp) error {
	if !re.MatchString(v) {
		return fmt.Errorf("buzz: %s %q has the wrong shape", name, v)
	}
	return nil
}
