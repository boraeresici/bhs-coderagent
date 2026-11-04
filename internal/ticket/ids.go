package ticket

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Key returns a stable content-derived id: the first 32 hex digits of the SHA-256 of the
// values as a JSON array. Equal inputs always give the same id.
//
// The output matches upstream src/core/ids.ts byte for byte, which serializes with
// JavaScript's JSON.stringify. Go's encoding/json differs from it (it escapes <, >, &,
// U+2028 and U+2029), so the array is written by hand.
func Key(values ...string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		writeJSString(&b, v)
	}
	b.WriteByte(']')
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:32]
}

// writeJSString writes s as JSON.stringify would: only the quote, the backslash and
// control characters below U+0020 are escaped.
func writeJSString(b *strings.Builder, s string) {
	const hexDigits = "0123456789abcdef"
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteByte(hexDigits[r>>4])
				b.WriteByte(hexDigits[r&0xf])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
