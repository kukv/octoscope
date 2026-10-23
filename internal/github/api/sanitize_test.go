package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// gh hands the cli backend every answer with its control characters in caret
// notation. Anyone who can open a pull request on a public repository writes
// its title, body and diff, and a control character left raw there would be
// acted on by the user's terminal rather than shown -- C1 controls and
// DCS/APC/SOS/PM strings get past Bubble Tea. So the api backend does what gh
// does, on the JSON as it arrives.
func TestControlCharactersInJSONBecomeCaretNotation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"an escaped ESC", `"\u001b[2K"`, "^[[2K"},
		{"a DCS string", `"\u001bP$qm\u001b\\"`, "^[P$qm^[\\"},
		{"an escaped C1 CSI", `"\u009b6n"`, "^[6n"},
		{"a raw C1 OSC and ST", "\"\u009d2;x\u009c\"", "^]2;x^\\"},
		{"an escaped file separator, whose caret form needs a backslash", `"\u001c"`, "^\\"},
		{"an escaped backslash before u001b is text", `"\\u001b"`, `\u001b`},
		{"newline and tab are layout", `"a\nb\tc\u000a"`, "a\nb\tc\n"},
		{"carriage return is layout", `"a\r\n"`, "a\r\n"},
		{"Japanese and a surrogate pair stay", `"日本😀"`, "日本\U0001F600"},
		{"a quote stays escaped", `"a\"b"`, `a"b`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := caretJSON([]byte(tt.in))
			var got string
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("caretJSON(%s) = %s, not JSON: %v", tt.in, out, err)
			}
			if got != tt.want {
				t.Errorf("caretJSON(%s) decodes to %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// gh pr diff refuses a diff with an escape in it outright; the api backend
// shows it instead, but in caret notation so the diff cannot repaint the
// terminal. Tab and carriage return are how files are laid out and stay.
func TestControlCharactersInADiffBecomeCaretNotation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ESC", "+\x1b]52;c;x\x07\n", "+^[]52;c;x^G\n"},
		{"UTF-8 C1", "+\u009b6n\n", "+^[6n\n"},
		{"a lone C1 byte that is not UTF-8", "+\x9b6n\n", "+^[6n\n"},
		{"layout stays", "+\ta\r\n", "+\ta\r\n"},
		{"Japanese stays", "+日本\n", "+日本\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := string(caretText([]byte(tt.in))); got != tt.want {
				t.Errorf("caretText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Every GraphQL answer goes through post, so a title read there is the
// title the detail view draws.
func TestAGraphQLAnswerArrivesInCaretNotation(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, http.StatusOK, `{"data":{"repository":{"pullRequest":{"title":"\u009b6n\u001bP$qm\u001b\\"}}}}`)
	pr, err := c.GetPR(context.Background(), "kukv/octoscope", 1)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.Title != "^[6n^[P$qm^[\\" {
		t.Errorf("title = %q, want caret notation", pr.Title)
	}
}

func TestADiffArrivesInCaretNotation(t *testing.T) {
	t.Parallel()

	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "diff --git a/f b/f\n+\u009b6n\n")
	})
	diff, err := c.PRDiff(context.Background(), "kukv/octoscope", 1)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}
	if string(diff.Raw) != "diff --git a/f b/f\n+^[6n\n" {
		t.Errorf("raw = %q, want caret notation", diff.Raw)
	}
}

// The files API is what a pull request too large for a diff is read
// through, and its patches are the same text the diff would have carried.
func TestTheFilesFallbackArrivesInCaretNotation(t *testing.T) {
	t.Parallel()

	calls := 0
	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusNotAcceptable)
			_, _ = io.WriteString(w, `{"message":"too large"}`)
			return
		}
		_, _ = io.WriteString(w, `[{"filename":"f","status":"modified","patch":"@@ -0,0 +1 @@\n+\u009b6n"}]`)
	})
	diff, err := c.PRDiff(context.Background(), "kukv/octoscope", 1)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}
	if len(diff.Files) != 1 || diff.Files[0].Patch == nil || *diff.Files[0].Patch != "@@ -0,0 +1 @@\n+^[6n" {
		t.Errorf("files = %+v, want the patch in caret notation", diff.Files)
	}
}
