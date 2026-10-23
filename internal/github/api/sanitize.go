package api

import (
	"strconv"
	"unicode/utf8"
)

// caret is a control character written the way a terminal shows one: ^ and the
// letter it is made from. C1 characters (0x80-0x9f) show as the C0 characters
// they mirror. Anything else is returned untouched, delete (0x7f) included:
// it is a control character Go names as one and gh does not rewrite.
func caret(r rune) (string, bool) {
	switch {
	case r == '\t' || r == '\n' || r == '\v' || r == '\r':
		return "", false
	case r >= 0x00 && r <= 0x1f:
		return "^" + string('@'+r), true
	case r >= 0x80 && r <= 0x9f:
		return "^" + string('@'+r-0x80), true
	}
	return "", false
}

// caretJSON puts the control characters inside a JSON answer into caret
// notation before it is decoded, which is what gh does to every answer it
// hands the cli backend. A control character arrives either escaped (\u001b)
// or, for C1, as raw UTF-8; JSON allows no raw C0. Two-character escapes are
// copied whole, so the u in \\u001b is never read as the start of an escape.
func caretJSON(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		if b[i] == '\\' && i+1 < len(b) {
			if b[i+1] == 'u' && i+6 <= len(b) {
				if n, err := strconv.ParseUint(string(b[i+2:i+6]), 16, 16); err == nil {
					if c, ok := caret(rune(n)); ok {
						out = appendJSONText(out, c)
						i += 6
						continue
					}
				}
			}
			out = append(out, b[i], b[i+1])
			i += 2
			continue
		}
		r, size := utf8.DecodeRune(b[i:])
		if c, ok := caret(r); ok && r >= 0x80 {
			out = appendJSONText(out, c)
		} else {
			out = append(out, b[i:i+size]...)
		}
		i += size
	}
	return out
}

// appendJSONText writes a caret form into a JSON string. ^\ is the one whose
// letter JSON needs escaped.
func appendJSONText(out []byte, c string) []byte {
	for _, ch := range []byte(c) {
		if ch == '\\' {
			out = append(out, '\\')
		}
		out = append(out, ch)
	}
	return out
}

// caretText puts the control characters in plain text -- a diff -- into
// caret notation. A diff need not be UTF-8, and a lone byte in 0x80-0x9f is a
// C1 control to a terminal that reads it as one, so it goes too.
func caretText(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 && b[i] >= 0x80 && b[i] <= 0x9f {
			r = rune(b[i])
		}
		if c, ok := caret(r); ok {
			out = append(out, c...)
		} else {
			out = append(out, b[i:i+size]...)
		}
		i += size
	}
	return out
}
