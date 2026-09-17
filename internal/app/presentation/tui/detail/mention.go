// Whether a block of GitHub markdown is addressed at the reader. The answer
// decides which comment is drawn as the reader's own, so it is asked of the
// source GitHub sent -- not of what glamour drew, where colour and wrapping
// can split a name across two lines.

package detail

import "strings"

// mentionsViewer reports whether src names login the way a person writes to
// a person.
//
// This is not a markdown parser. Three places are skipped because an "@name"
// in them is certainly not addressed at anyone -- a fenced code block, an
// inline code span, and a quoted line -- and every other "@name" is taken at
// face value, including one inside a link or a heading. Lighting one comment
// too many costs a glance; missing the comment that names the reader costs
// the whole point of the highlight.
func mentionsViewer(src, login string) bool {
	if login == "" {
		return false
	}
	inFence := false
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		// Either marker toggles either kind of fence. Telling them apart
		// would only matter for a ``` inside a ~~~ block, where the answer
		// is the same anyway: it is code, and nobody is being addressed.
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || strings.HasPrefix(trimmed, ">") {
			continue
		}
		if mentionsInLine(stripCodeSpans(trimmed), login) {
			return true
		}
	}
	return false
}

// stripCodeSpans drops what sits between backticks. An odd number of them
// means the last one closes nothing -- in markdown it is a literal backtick,
// and the words after it are prose -- so such a line is left whole.
func stripCodeSpans(line string) string {
	parts := strings.Split(line, "`")
	if len(parts)%2 == 0 { // an even count of parts is an odd count of backticks
		return line
	}
	var b strings.Builder
	for i := 0; i < len(parts); i += 2 {
		b.WriteString(parts[i])
	}
	return b.String()
}

// mentionsInLine finds "@login" where the name ends. GitHub logins are
// letters, digits and hyphens and are not case-sensitive, so the "@kukv" in
// "@kukv-bot" is the start of somebody else's name.
func mentionsInLine(line, login string) bool {
	lower := strings.ToLower(line)
	name := "@" + strings.ToLower(login)
	for i := 0; i+len(name) <= len(lower); {
		j := strings.Index(lower[i:], name)
		if j < 0 {
			return false
		}
		end := i + j + len(name)
		if end == len(lower) || !isLoginByte(lower[end]) {
			return true
		}
		i = end
	}
	return false
}

// isLoginByte reports whether b could be the next byte of a login. A byte of
// a multi-byte character is not one, which is what makes "@kukvさん" a
// mention of kukv.
func isLoginByte(b byte) bool {
	return b == '-' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z'
}
