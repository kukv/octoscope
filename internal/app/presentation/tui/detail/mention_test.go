package detail

import "testing"

// TestMentionsViewer is the whole rule, case by case. The scan is not a
// markdown parser: it skips the three places an "@name" is certainly not
// addressed at a person, and takes everything else at face value.
func TestMentionsViewer(t *testing.T) {
	cases := map[string]struct {
		body string
		want bool
	}{
		"plain":                          {"@kukv ここ見てもらえますか", true},
		"mid-sentence":                   {"これは @kukv の担当です", true},
		"at the very end":                {"よろしく @kukv", true},
		"a different case":               {"@KuKv お願いします", true},
		"inside a link":                  {"[@kukv](https://github.com/kukv)", true},
		"straight onto a wide character": {"@kukvさん お願いします", true},
		"another name starting the same": {"@kukv-bot が直します", false},
		"a longer name":                  {"@kukvx が直します", false},
		"somebody else":                  {"@alice お願いします", false},
		"no mention at all":              {"LGTM です", false},
		"in a code span":                 {"`@kukv` と書くと通知が飛ぶ", false},
		"in a fenced block":              {"```\n@kukv\n```", false},
		"in a tilde fenced block":        {"~~~\n@kukv\n~~~", false},
		"in an unclosed fence":           {"```\n@kukv", false},
		"in a quoted line":               {"> @kukv と言われた", false},
		"in an indented quote":           {"  > @kukv と言われた", false},
		"after a fence closes":           {"```\ncode\n```\n@kukv 見てください", true},
		"an unmatched backtick":          {"値は 3` です @kukv", true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := mentionsViewer(c.body, "kukv"); got != c.want {
				t.Errorf("mentionsViewer(%q) = %v, want %v", c.body, got, c.want)
			}
		})
	}
}

// TestAnUnknownViewerMentionsNobody is the state the view is in until the
// lookup answers, and for good if it failed. Everything must read as "not
// addressed at me" rather than as "addressed at everybody".
func TestAnUnknownViewerMentionsNobody(t *testing.T) {
	if mentionsViewer("@kukv @alice @", "") {
		t.Error("an empty login matched")
	}
}
