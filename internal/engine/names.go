package engine

import (
	"strings"
	"unicode"
)

// Title is the first line of a request without leading mentions, trimmed to 80 runes.
// Trigger text is untrusted; the title is only ever shown, never interpreted.
func Title(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	words := strings.Fields(line)
	for len(words) > 0 && strings.HasPrefix(words[0], "@") {
		words = words[1:]
	}
	title := strings.Join(words, " ")
	if r := []rune(title); len(r) > 80 {
		title = strings.TrimSpace(string(r[:79])) + "…"
	}
	if title == "" {
		return "(başlıksız istek)"
	}
	return title
}

var fold = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "İ", "i", "ö", "o", "ş", "s", "ü", "u",
	"Ç", "c", "Ğ", "g", "Ö", "o", "Ş", "s", "Ü", "u")

// ShortName names a Ticket's branch agent/<short> and directory: a slug of the title and
// the first six digits of the Ticket id, so two Tickets with the same title differ. It
// only ever holds [a-z0-9-], whatever the request said.
func ShortName(title, ticketID string) string {
	var b strings.Builder
	dash := false
	for _, r := range fold.Replace(strings.ToLower(title)) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			if b.Len() >= 24 {
				break
			}
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "ticket"
	}
	return slug + "-" + ticketID[:6]
}
