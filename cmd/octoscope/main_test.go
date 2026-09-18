package main

import "testing"

// TestResolveVersion covers the three ways a build can come to know its own
// name, and the two that leave it with nothing.
func TestResolveVersion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		injected string
		module   string
		want     string
	}{
		{"goreleaser injected it", "v0.8.0", "", "v0.8.0"},
		{"go install left it in the binary", "", "v0.8.0", "v0.8.0"},
		{"injected wins over the module's own", "v0.8.0", "v0.7.0", "v0.8.0"},
		{"built from source without a version", "", "(devel)", "dev"},
		{"nothing to go on", "", "", "dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveVersion(tc.injected, tc.module); got != tc.want {
				t.Errorf("resolveVersion(%q, %q) = %q, want %q", tc.injected, tc.module, got, tc.want)
			}
		})
	}
}
