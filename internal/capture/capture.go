// Package capture decides whether a prompt is a jot.
//
// The rule is deliberately narrow: the prompt must start with ">>", then a
// single space, then a non-whitespace character. Anything else — including
// ">>x", " >> x", "\>> x", "a >> b" and "> quote" — is an ordinary prompt and
// must pass through untouched.
package capture

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Prefix is the marker that turns a prompt into a jot.
const Prefix = ">> "

// Match reports whether prompt is a jot and, if so, returns the jotted text
// with the prefix removed and trailing whitespace trimmed.
func Match(prompt string) (text string, ok bool) {
	if !strings.HasPrefix(prompt, Prefix) {
		return "", false
	}
	rest := prompt[len(Prefix):]
	r, _ := utf8.DecodeRuneInString(rest)
	if rest == "" || unicode.IsSpace(r) {
		return "", false
	}
	return strings.TrimRightFunc(rest, unicode.IsSpace), true
}
