package main

import (
	"strings"
	"unicode/utf8"
)

// cp1252 byte for the undo of a Windows-1252 mis-decode.  Older shimo.net
// builds sometimes return UTF-8 bytes decoded as Windows-1252, which produces
// characters such as "ä¼ä¸šé«˜çº§ç‰ˆ" for 企业高级版.  The 0x80-0x9F range cannot be
// recovered with a plain Latin-1 cast, so map those code points explicitly.
var cp1252Reverse = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87,
	'ˆ': 0x88, '‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E,
	'‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97,
	'˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B, 'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// repairMojibake fixes double-encoded text without touching genuine ASCII or
// legitimate accented text ("café" stays as-is because its byte sequence is not
// valid UTF-8).
func repairMojibake(s string) string {
	if s == "" || !utf8.ValidString(s) {
		return s
	}
	b := make([]byte, 0, len(s))
	ascii := true
	for _, r := range s {
		if r <= 0xFF {
			if r > 0x7F {
				ascii = false
			}
			b = append(b, byte(r))
			continue
		}
		if v, ok := cp1252Reverse[r]; ok {
			ascii = false
			b = append(b, v)
			continue
		}
		return s
	}
	if ascii || !utf8.Valid(b) {
		return s
	}
	fixed := string(b)
	if fixed == s || !utf8.ValidString(fixed) {
		return s
	}
	hasWide := false
	for _, r := range fixed {
		if r > 0xFF {
			hasWide = true
			break
		}
	}
	if !hasWide {
		return s
	}
	return fixed
}

func deepRepairMojibake(v any) any {
	switch x := v.(type) {
	case string:
		return repairMojibake(x)
	case map[string]any:
		for k, item := range x {
			x[k] = deepRepairMojibake(item)
		}
		return x
	case []any:
		for i, item := range x {
			x[i] = deepRepairMojibake(item)
		}
		return x
	default:
		return v
	}
}

func nonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
