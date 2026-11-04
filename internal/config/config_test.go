package config

import (
	"strings"
	"testing"
	"time"
)

func TestExampleLoads(t *testing.T) {
	c, warnings, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if c.Buzz.Poll != 60*time.Second {
		t.Errorf("poll = %v", c.Buzz.Poll)
	}
	r, ok := c.Repo("OWNER/tms")
	if !ok || r.Base != "dev" {
		t.Fatalf("Repo lookup = %+v, %v", r, ok)
	}
	if !r.AllowsLogin("Borae") || r.AllowsLogin("someone") {
		t.Error("AllowsLogin must match listed logins case-insensitively and nothing else")
	}
}

const valid = `
state_dir: /tmp/s
api: {listen: "127.0.0.1:3082", token_env: T}
buzz:
  cli: buzz
  self_pubkey: "0000000000000000000000000000000000000000000000000000000000000001"
  coder_pubkey: "0000000000000000000000000000000000000000000000000000000000000002"
  coder_name: coder
  channels: [c1]
github: {cli: gh, self_logins: [bot]}
repos:
`

func TestRepoValidation(t *testing.T) {
	cases := []struct {
		name, repo, wantErr string
	}{
		{"missing base", `- {name: a, github: o/a}`, "base is required"},
		{"bad github", `- {name: a, github: a, base: main}`, "github must be owner/name"},
		{"unknown run", `- {name: a, github: o/a, base: main, run: k8s}`, "run must be native or compose"},
		{"run without up", `- {name: a, github: o/a, base: main, run: native}`, "needs up and health"},
		{"unknown defter", `- {name: a, github: o/a, base: main, defter: s3}`, "defter must be repo or local"},
		{"uat without run", `- {name: a, github: o/a, base: main, uat_paket: p}`, "uat_paket needs run"},
		{"duplicate", "- {name: a, github: o/a, base: main}\n- {name: a, github: o/b, base: main}", "duplicate name"},
		{"typo is not ignored", `- {name: a, github: o/a, base: main, allowed_login: [x]}`, "field allowed_login not found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := Parse([]byte(valid + c.repo))
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, c.wantErr)
			}
		})
	}
}

func TestEmptyAllowedLoginsFailsClosed(t *testing.T) {
	c, warnings, err := Parse([]byte(valid + `- {name: a, github: o/a, base: main}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "allowed_logins is empty") {
		t.Errorf("warnings = %v", warnings)
	}
	if c.Repos[0].AllowsLogin("") || c.Repos[0].AllowsLogin("anyone") {
		t.Error("an empty allowed_logins must allow no one")
	}
}

func TestBuzzValidation(t *testing.T) {
	bad := strings.Replace(valid, `"0000000000000000000000000000000000000000000000000000000000000002"`, "npub1xyz", 1)
	_, _, err := Parse([]byte(bad + `- {name: a, github: o/a, base: main}`))
	if err == nil || !strings.Contains(err.Error(), "coder_pubkey must be") {
		t.Errorf("err = %v", err)
	}
}

func TestGitHubValidation(t *testing.T) {
	repo := `- {name: a, github: o/a, base: main, allowed_logins: [x]}`
	off := strings.Replace(valid, "github: {cli: gh, self_logins: [bot]}\n", "", 1)
	c, warnings, err := Parse([]byte(off + repo))
	if err != nil || c.GitHub.CLI != "" || len(warnings) != 1 || !strings.Contains(warnings[0], "github.cli and github.api_url are empty") {
		t.Errorf("github off: %v %v", warnings, err)
	}
	for _, c := range []struct{ github, wantErr string }{
		{"github: {cli: gh}", "github.self_logins"},
		{"github: {cli: gh, self_logins: [bot], poll: 10s}", "github.poll must be at least 30s"},
		{"github: {cli: gh, api_url: \"https://api.github.com\", self_logins: [bot]}", "not both"},
		{"github: {api_url: \"http://api.github.com\", self_logins: [bot]}", "must be https://"},
	} {
		bad := strings.Replace(valid, "github: {cli: gh, self_logins: [bot]}", c.github, 1)
		if _, _, err := Parse([]byte(bad + repo)); err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: err = %v", c.github, err)
		}
	}
}

func TestGitHubHTTPSWithoutTokenWarns(t *testing.T) {
	repo := `- {name: a, github: o/a, base: main, allowed_logins: [x]}`
	https := strings.Replace(valid, "github: {cli: gh, self_logins: [bot]}", `github: {api_url: "https://api.github.com", self_logins: [bot]}`, 1)
	_, warnings, err := Parse([]byte(https + repo))
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "60 requests an hour") {
		t.Errorf("no token: %v %v", warnings, err)
	}
	withToken := strings.Replace(https, "self_logins", "token_env: GH_TOKEN, self_logins", 1)
	if _, warnings, err := Parse([]byte(withToken + repo)); err != nil || len(warnings) != 0 {
		t.Errorf("token: %v %v", warnings, err)
	}
}
