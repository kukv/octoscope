package theme

import (
	"fmt"
	"testing"
)

// TestHighlightSurvivesTheCacheFilling is the one thing the size limit can
// break. The cache is dropped whole when it fills, so a line asked for
// across that boundary must still come back the same -- the answer comes
// from chroma either way, and only the speed changes.
func TestHighlightSurvivesTheCacheFilling(t *testing.T) {
	const code = "func Walk(ctx context.Context) error {"
	want := Highlight("walk.go", code)

	// Enough distinct lines to push the cache past its limit once.
	for i := range highlightCacheMax + 1 {
		Highlight("walk.go", fmt.Sprintf("x%d := %d", i, i))
	}

	if got := Highlight("walk.go", code); got != want {
		t.Fatalf("the answer changed once the cache had been dropped:\n%q\n%q", got, want)
	}
	// The answer surviving is not enough on its own -- it would survive just
	// as well with no limit at all. The limit is only proven by the map
	// never growing past it.
	if n := len(highlightLines); n > highlightCacheMax {
		t.Fatalf("the cache holds %d entries, want at most %d", n, highlightCacheMax)
	}
}
