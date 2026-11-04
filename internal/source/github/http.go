package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxBody bounds one response; a page of 100 check runs is about 300 KB.
const maxBody = 8 << 20

// HTTP reads the GitHub REST API over HTTPS, without the gh binary. Without a token it
// reads public repositories only, at 60 requests an hour; with one, 5000 (measured
// 2026-11-04). The responses are those gh api returns, so the Source does not change.
type HTTP struct {
	// BaseURL is the API root, https://api.github.com (or a GitHub Enterprise API URL).
	BaseURL string
	// Token is sent as a bearer token when set; it never appears in a URL or a log.
	Token  string
	Client *http.Client
}

// Get returns the JSON body of GET path (relative, e.g. repos/o/r/pulls/1).
func (h *HTTP) Get(ctx context.Context, path string) (json.RawMessage, error) {
	if strings.HasPrefix(path, "-") || strings.HasPrefix(path, "/") || strings.Contains(path, "://") {
		return nil, fmt.Errorf("github: refusing path %q", path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(h.BaseURL, "/")+"/"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "bhs-coderagent")
	if h.Token != "" {
		req.Header.Set("Authorization", "Bearer "+h.Token)
	}
	c := h.Client
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("github: %s: response over %d bytes", path, maxBody)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct{ Message string }
		json.Unmarshal(body, &e)
		if e.Message == "" {
			e.Message = resp.Status
		}
		return nil, &Error{Status: resp.StatusCode, Message: e.Message}
	}
	return body, nil
}
