package diff

import "testing"

// benchSink keeps the compiler from optimising View away.
var benchSink string

// BenchmarkView is one frame of the diff pane at the width a screen most
// commonly opens at. Bubble Tea calls View once per message, so this is
// what a single keypress costs.
func BenchmarkView(b *testing.B) {
	m := goldenModel(160)
	for b.Loop() {
		benchSink = m.View()
	}
}
