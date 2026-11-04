package ticket

import "testing"

// Vectors produced by upstream src/core/ids.ts (Node 22) at commit 8c32a9d; a mismatch
// means ids stored by one implementation would not be found by the other.
func TestKeyMatchesUpstream(t *testing.T) {
	cases := []struct {
		values []string
		want   string
	}{
		{nil, "4f53cda18c2baa0c0354bb5f9a3ecbe5"},
		{[]string{"buzz", "abc123"}, "9001a3a27997cd46ca5c983b3327b28e"},
		{[]string{"github", "42", "2026-11-04T10:00:00.000Z"}, "57c5605ea1c9e3475e2c78054d77e1be"},
		{[]string{"<a&b>", " ", "tab\there", `quote"back\slash`, "\u0001ctl", "ğüşİöç 🚀"}, "c2bedbc4c7c1498d01e36bd4a02e12c5"},
	}
	for _, c := range cases {
		if got := Key(c.values...); got != c.want {
			t.Errorf("Key(%q) = %s, want %s", c.values, got, c.want)
		}
	}
}
