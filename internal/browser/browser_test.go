package browser

import (
	"slices"
	"testing"
)

func TestCommandPerEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		goos       string
		wsl        bool
		browserEnv string
		want       []string
		wantOK     bool
	}{
		{
			name:   "windows hands the URL to the shell handler",
			goos:   "windows",
			want:   []string{"rundll32", "url.dll,FileProtocolHandler"},
			wantOK: true,
		},
		{
			name:   "macOS opens it",
			goos:   "darwin",
			want:   []string{"open"},
			wantOK: true,
		},
		{
			name:   "linux goes through xdg-open",
			goos:   "linux",
			want:   []string{"xdg-open"},
			wantOK: true,
		},
		{
			// gh's --web looks for xdg-open, x-www-browser, www-browser and
			// wslview, and a WSL install has none of them.
			name:   "WSL crosses over to Windows instead of xdg-open",
			goos:   "linux",
			wsl:    true,
			want:   []string{"powershell.exe", "-NoProfile", "-Command", "Start-Process"},
			wantOK: true,
		},
		{
			name:       "a named browser wins over the platform default",
			goos:       "linux",
			wsl:        true,
			browserEnv: "firefox",
			want:       []string{"firefox"},
			wantOK:     true,
		},
		{
			// "Everywhere" is the point: on Windows and macOS there is always
			// a platform default to fall back on, so only these cases catch a
			// named browser being tried second.
			name:       "a named browser wins on Windows too",
			goos:       "windows",
			browserEnv: "firefox",
			want:       []string{"firefox"},
			wantOK:     true,
		},
		{
			name:       "a named browser wins on macOS too",
			goos:       "darwin",
			browserEnv: "firefox",
			want:       []string{"firefox"},
			wantOK:     true,
		},
		{
			name:   "an unknown platform has nothing to try",
			goos:   "plan9",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := command(environment{
				goos:    tt.goos,
				wsl:     tt.wsl,
				browser: tt.browserEnv,
			})
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("command = %v, want %v", got, tt.want)
			}
		})
	}
}

// gh reads GH_BROWSER before BROWSER (`gh help environment`). A user who set
// both did so to tell gh apart from the rest of the machine, and octoscope
// must not reverse that.
func TestGHBrowserBeatsBrowser(t *testing.T) {
	t.Setenv("GH_BROWSER", "gh-choice")
	t.Setenv("BROWSER", "system-choice")

	if got := browserEnv(); got != "gh-choice" {
		t.Errorf("browserEnv = %q, want the GH_BROWSER value", got)
	}
}

func TestBrowserIsUsedWhenGHBrowserIsUnset(t *testing.T) {
	t.Setenv("GH_BROWSER", "")
	t.Setenv("BROWSER", "system-choice")

	if got := browserEnv(); got != "system-choice" {
		t.Errorf("browserEnv = %q, want the BROWSER value", got)
	}
}

// WSL sets WSL_DISTRO_NAME in every distro's shell, which is the signal left
// for a kernel that does not name itself in /proc/sys/kernel/osrelease.
func TestWSLDistroNameIsEnoughToDetectWSL(t *testing.T) {
	t.Setenv("WSL_DISTRO_NAME", "Ubuntu")

	if !isWSL() {
		t.Error("isWSL = false with WSL_DISTRO_NAME set")
	}
}

// TestNoCommandGoesThroughAShell pins what makes the arguments safe: the URL
// is appended as one argument to an executable, so nothing in it can be read
// as a command (.claude/rules/go-style.md).
func TestNoCommandGoesThroughAShell(t *testing.T) {
	t.Parallel()

	for _, goos := range []string{"windows", "darwin", "linux"} {
		for _, wsl := range []bool{false, true} {
			argv, ok := command(environment{goos: goos, wsl: wsl})
			if !ok {
				continue
			}
			if slices.Contains(argv, "-c") || argv[0] == "sh" || argv[0] == "bash" {
				t.Errorf("%s (wsl=%v) runs %v, which puts the URL through a shell", goos, wsl, argv)
			}
		}
	}
}
