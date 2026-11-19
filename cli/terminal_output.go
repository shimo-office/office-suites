package main

import (
	"strconv"
	"strings"
	"unicode"
)

// terminalText renders one metadata field without allowing it to add terminal
// lines, execute ANSI controls, or reorder surrounding trusted labels. JSON and
// downloaded files retain their original values; escape only at display time.
func terminalText(s string) string { return terminalEscape(s, false) }

// terminalContent preserves document line breaks and tabs while making terminal
// control and bidi formatting characters visible instead of executing them.
func terminalContent(s string) string { return terminalEscape(s, true) }

func terminalEscape(s string, multiline bool) string {
	var out strings.Builder
	for _, r := range s {
		if multiline && (r == '\n' || r == '\t') {
			out.WriteRune(r)
			continue
		}
		bidi := r == '\u061c' || r == '\u200e' || r == '\u200f' || (r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069')
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || bidi {
			quoted := strconv.QuoteRune(r)
			out.WriteString(quoted[1 : len(quoted)-1])
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}
