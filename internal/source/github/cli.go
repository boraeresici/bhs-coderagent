// Package github is the GitHub Source (ANALIZ.md §5.8, §5.8a). It reads only the pull
// requests of Ticket branches (agent/<short name>) and the pull requests Tickets follow:
// no notifications, no other pull request or issue. GitHub carries context to Tickets;
// Buzz stays the only command channel, and the engine decides who may reach the coder.
//
// It talks to GitHub through `gh api`, as the Buzz Source talks to its relay through the
// buzz CLI; the response shapes are those recorded in testdata (gh 2.97, 2026-11-04).
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Runner runs the gh binary. Production uses Exec; tests use a fake API instead.
type Runner interface {
	Run(ctx context.Context, args []string) (stdout, stderr []byte, code int, err error)
}

// Exec runs gh at Path without a shell. The token comes from GH_TOKEN in the process
// environment, never from the command line.
type Exec struct{ Path string }

// Run implements Runner.
func (x Exec) Run(ctx context.Context, args []string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, x.Path, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.Bytes(), errb.Bytes(), exit.ExitCode(), nil
	}
	return out.Bytes(), errb.Bytes(), 0, err
}

// Error is a failed API call. gh exits 1 for every failure; the HTTP status is only in
// its stderr line, "gh: Not Found (HTTP 404)".
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("github: HTTP %d: %s", e.Status, e.Message) }

var httpStatus = regexp.MustCompile(`\(HTTP (\d{3})\)`)

// CLI reads the GitHub REST API through `gh api`.
type CLI struct{ Runner Runner }

// Get returns the JSON body of GET path (relative, e.g. repos/o/r/pulls/1).
func (c *CLI) Get(ctx context.Context, path string) (json.RawMessage, error) {
	if strings.HasPrefix(path, "-") || strings.Contains(path, "://") {
		return nil, fmt.Errorf("github: refusing path %q", path)
	}
	out, errb, code, err := c.Runner.Run(ctx, []string{"api", "-H", "Accept: application/vnd.github+json", path})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		msg := strings.TrimSpace(string(errb))
		status := 0
		if m := httpStatus.FindStringSubmatch(msg); m != nil {
			status, _ = strconv.Atoi(m[1])
		}
		return nil, &Error{Status: status, Message: strings.TrimPrefix(msg, "gh: ")}
	}
	return out, nil
}
