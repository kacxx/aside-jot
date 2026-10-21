// Package capture decides whether a prompt is a jot.
//
// The rule is deliberately narrow: the prompt must start with ">>", then one
// or more spaces or tabs (a non-breaking space counts), then a non-whitespace
// character. Leading <system-reminder> blocks, which Claude Desktop puts in
// front of the prompt, are skipped first. Anything else — including ">>x",
// " >> x", "\>> x", "a >> b" and "> quote" — is an ordinary prompt and must
// pass through untouched.
package capture

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Prefix is the marker that turns a prompt into a jot.
const Prefix = ">>"

// Match reports whether prompt is a jot and, if so, returns the jotted text
// with the prefix removed and trailing whitespace trimmed.
func Match(prompt string) (text string, ok bool) {
	prompt = StripSystemReminders(prompt)
	if !strings.HasPrefix(prompt, Prefix) {
		return "", false
	}
	rest := prompt[len(Prefix):]
	body := strings.TrimLeftFunc(rest, isGap)
	if len(body) == len(rest) {
		return "", false
	}
	r, _ := utf8.DecodeRuneInString(body)
	if body == "" || unicode.IsSpace(r) {
		return "", false
	}
	return strings.TrimRightFunc(body, unicode.IsSpace), true
}

// isGap reports whether r may separate the prefix from the text.
func isGap(r rune) bool {
	return r == ' ' || r == '\t' || r == '\u00a0'
}

// StripSystemReminders removes the <system-reminder> blocks that start text,
// and the line breaks after them. A block that is never closed leaves nothing.
func StripSystemReminders(text string) string {
	const open, end = "<system-reminder>", "</system-reminder>"
	for {
		t := strings.TrimLeft(text, " \t\r\n")
		if !strings.HasPrefix(t, open) {
			return text
		}
		_, rest, ok := strings.Cut(t, end)
		if !ok {
			return ""
		}
		text = strings.TrimLeft(rest, "\r\n")
	}
}
