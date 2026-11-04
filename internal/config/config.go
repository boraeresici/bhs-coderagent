// Package config loads the orchestrator's settings and per-repository profiles
// (ANALIZ.md §5.5a). Nothing here is inferred from code or a model: every value a
// profile needs is written down, and secrets are named, never stored.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is the whole settings file.
type Config struct {
	// StateDir holds the SQLite store and its lock.
	StateDir string `yaml:"state_dir"`
	API      API    `yaml:"api"`
	Buzz     Buzz   `yaml:"buzz"`
	Repos    []Repo `yaml:"repos"`
}

// API is the orchestrator's HTTP API (§5.9).
type API struct {
	Listen string `yaml:"listen"`
	// TokenEnv names the environment variable that holds the bearer token.
	TokenEnv string `yaml:"token_env"`
}

// Buzz is the Buzz Source (§5.2).
type Buzz struct {
	// CLI is the path of the buzz binary the Source reads and writes through.
	CLI string `yaml:"cli"`
	// SelfPubkey is the orchestrator's own pubkey (hex); its messages never become Triggers.
	SelfPubkey string `yaml:"self_pubkey"`
	// CoderPubkey is the pubkey whose report blocks are accepted (§5.6, A10).
	CoderPubkey string `yaml:"coder_pubkey"`
	// CoderName is the display name used to mention the coder (@Name).
	CoderName string `yaml:"coder_name"`
	// AgentPubkeys are every BHS agent; their messages are human:false (§5.7).
	AgentPubkeys []string `yaml:"agent_pubkeys"`
	// Channels are the channel ids the Source listens to; a mention elsewhere is filtered.
	Channels []string      `yaml:"channels"`
	Poll     time.Duration `yaml:"poll"`
}

// Repo is one repository's profile (§5.5a).
type Repo struct {
	Name string `yaml:"name"`
	// GitHub is owner/name.
	GitHub string `yaml:"github"`
	// Base is the branch tickets branch from, PRs target and protection guards.
	Base string `yaml:"base"`
	// Run is how the application starts for QA: native, compose, or empty when it does not.
	Run    string `yaml:"run"`
	Up     string `yaml:"up"`
	Down   string `yaml:"down"`
	Health string `yaml:"health"`
	// AllowedLogins are the GitHub logins whose comments reach the coder. Empty means none
	// do: there is no collaborator fallback (§5.8a, A16).
	AllowedLogins []string `yaml:"allowed_logins"`
	// UATPaket is the uat-kabul-testi package id; empty means the repo has no QA.
	UATPaket string `yaml:"uat_paket"`
	// Defter is where the test notebook copy lives: repo, local, or empty to ask on the
	// first Ticket (§5.12).
	Defter              string   `yaml:"defter"`
	SensitivePathsExtra []string `yaml:"sensitive_paths_extra"`
}

// AllowsLogin reports whether a GitHub login may send the coder Input on this repo.
// GitHub logins are case-insensitive.
func (r Repo) AllowsLogin(login string) bool {
	for _, l := range r.AllowedLogins {
		if strings.EqualFold(l, login) {
			return true
		}
	}
	return false
}

// Repo returns the profile for owner/name, or false.
func (c *Config) Repo(github string) (Repo, bool) {
	for _, r := range c.Repos {
		if strings.EqualFold(r.GitHub, github) {
			return r, true
		}
	}
	return Repo{}, false
}

// Load reads and validates a settings file. Warnings are returned for settings that are
// valid but probably not what was meant.
func Load(path string) (*Config, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return Parse(data)
}

// Parse decodes and validates settings; unknown keys are errors so a typo cannot silently
// drop a safety setting such as allowed_logins.
func Parse(data []byte) (*Config, []string, error) {
	c := &Config{Buzz: Buzz{Poll: 60 * time.Second}}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	warnings, err := c.validate()
	if err != nil {
		return nil, nil, err
	}
	return c, warnings, nil
}

var (
	ownerName = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)
	hexKey    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func (c *Config) validate() (warnings []string, err error) {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if c.StateDir == "" {
		fail("state_dir is required")
	}
	if c.API.Listen == "" || c.API.TokenEnv == "" {
		fail("api.listen and api.token_env are required")
	}
	for name, key := range map[string]string{"self_pubkey": c.Buzz.SelfPubkey, "coder_pubkey": c.Buzz.CoderPubkey} {
		if !hexKey.MatchString(key) {
			fail("buzz.%s must be a 64-digit lowercase hex pubkey", name)
		}
	}
	if c.Buzz.CLI == "" || c.Buzz.CoderName == "" {
		fail("buzz.cli and buzz.coder_name are required")
	}
	if len(c.Buzz.Channels) == 0 {
		fail("buzz.channels must list at least one channel")
	}
	if c.Buzz.Poll < 5*time.Second {
		fail("buzz.poll must be at least 5s")
	}
	if len(c.Repos) == 0 {
		fail("repos must list at least one repository")
	}

	seen := map[string]bool{}
	for i, r := range c.Repos {
		at := fmt.Sprintf("repos[%d] (%s)", i, r.Name)
		switch {
		case r.Name == "":
			fail("repos[%d]: name is required", i)
		case seen[r.Name]:
			fail("%s: duplicate name", at)
		}
		seen[r.Name] = true
		if !ownerName.MatchString(r.GitHub) {
			fail("%s: github must be owner/name, got %q", at, r.GitHub)
		}
		if r.Base == "" {
			fail("%s: base is required (main, dev, …)", at)
		}
		switch r.Run {
		case "":
		case "native", "compose":
			if r.Up == "" || r.Health == "" {
				fail("%s: run %s needs up and health", at, r.Run)
			}
		default:
			fail("%s: run must be native or compose, got %q", at, r.Run)
		}
		switch r.Defter {
		case "", "repo", "local":
		default:
			fail("%s: defter must be repo or local, got %q", at, r.Defter)
		}
		if r.UATPaket != "" && r.Run == "" {
			fail("%s: uat_paket needs run, up and health", at)
		}
		if len(r.AllowedLogins) == 0 {
			warnings = append(warnings, at+": allowed_logins is empty, so no GitHub comment reaches the coder")
		}
	}
	return warnings, errors.Join(errs...)
}
