package browser

import (
	"encoding/base64"
	"encoding/binary"
	"slices"
	"testing"
	"unicode/utf16"
)

const itemURL = "https://github.com/o/r/pull/1"

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
			want:   []string{"rundll32", "url.dll,FileProtocolHandler", itemURL},
			wantOK: true,
		},
		{
			name:   "macOS opens it",
			goos:   "darwin",
			want:   []string{"open", itemURL},
			wantOK: true,
		},
		{
			name:   "linux goes through xdg-open",
			goos:   "linux",
			want:   []string{"xdg-open", itemURL},
			wantOK: true,
		},
		{
			name:       "a named browser wins over the platform default",
			goos:       "linux",
			wsl:        true,
			browserEnv: "firefox",
			want:       []string{"firefox", itemURL},
			wantOK:     true,
		},
		{
			// "Everywhere" is the point: on Windows and macOS there is always
			// a platform default to fall back on, so only these cases catch a
			// named browser being tried second.
			name:       "a named browser wins on Windows too",
			goos:       "windows",
			browserEnv: "firefox",
			want:       []string{"firefox", itemURL},
			wantOK:     true,
		},
		{
			name:       "a named browser wins on macOS too",
			goos:       "darwin",
			browserEnv: "firefox",
			want:       []string{"firefox", itemURL},
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
			}, itemURL)
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

// TestNoCommandGoesThroughAShell pins what makes the arguments safe: nothing
// in the URL can be read as a command (.claude/rules/go-style.md). On WSL
// that means no -Command, which re-parses its arguments as a script.
func TestNoCommandGoesThroughAShell(t *testing.T) {
	t.Parallel()

	for _, goos := range []string{"windows", "darwin", "linux"} {
		for _, wsl := range []bool{false, true} {
			argv, ok := command(environment{goos: goos, wsl: wsl}, itemURL)
			if !ok {
				continue
			}
			if slices.Contains(argv, "-c") || slices.Contains(argv, "-Command") || argv[0] == "sh" || argv[0] == "bash" {
				t.Errorf("%s (wsl=%v) runs %v, which puts the URL through a shell", goos, wsl, argv)
			}
		}
	}
}

// The checks view opens whatever a check's author put in detailsUrl or
// targetUrl. Only a web page may reach an opener: anything else is a file, a
// handler on the user's machine, or a flag to the opener itself.
func TestOnlyHTTPURLsAreOpened(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url    string
		wantOK bool
	}{
		{itemURL, true},
		{"HTTP://example.com/x?a=1&b=2", true},
		{"file:///etc/passwd", false},
		{"javascript:alert(1)", false},
		{"powershell:x", false},
		{"-foo", false},
		{"https:///nohost", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()

			err := validate(tt.url)
			if ok := err == nil; ok != tt.wantOK {
				t.Errorf("validate(%q) = %v, want ok = %v", tt.url, err, tt.wantOK)
			}
		})
	}
}

// gh's --web looks for xdg-open, x-www-browser, www-browser and wslview, and
// a WSL install has none of them, so WSL crosses over to Windows through
// PowerShell. The URL must reach it as a string handed to Start-Process,
// never as script: a check's author chooses the URL.
func TestWSLHandsTheURLToStartProcessAsALiteral(t *testing.T) {
	t.Parallel()

	for _, url := range []string{
		itemURL,
		"https://example.invalid/x;Write-Output(1)",
		"https://example.invalid/$(6*7)",
		"https://example.invalid/a'b",
		"https://example.invalid/a’b",
		"https://example.invalid/?a=1&b=2",
	} {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			argv, ok := command(environment{goos: "linux", wsl: true}, url)
			if !ok || argv[0] != "powershell.exe" {
				t.Fatalf("command = %v, want powershell.exe", argv)
			}
			i := slices.Index(argv, "-EncodedCommand")
			if i < 0 || i+2 != len(argv) {
				t.Fatalf("argv = %v, want the script as the last argument after -EncodedCommand", argv)
			}
			script := decodePowerShell(t, argv[i+1])
			const head = "Start-Process "
			if len(script) < len(head) || script[:len(head)] != head {
				t.Fatalf("script = %q, want it to start with %q", script, head)
			}
			got, rest := singleQuoted(t, script[len(head):])
			if got != url || rest != "" {
				t.Errorf("script = %q: literal %q then %q, want literal %q and nothing after", script, got, rest, url)
			}
		})
	}
}

// decodePowerShell undoes -EncodedCommand: base64 of UTF-16LE.
func decodePowerShell(t *testing.T, s string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b)%2 != 0 {
		t.Fatalf("decode %q: %v", s, err)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

// singleQuoted reads a PowerShell single-quoted string from the start of s
// the way PowerShell does: any of the five single-quote characters opens and
// closes it, and two in a row stand for one. It returns the value and what
// follows the closing quote.
func singleQuoted(t *testing.T, s string) (val, rest string) {
	t.Helper()
	isQuote := func(r rune) bool {
		return r == '\'' || r == '‘' || r == '’' || r == '‚' || r == '‛'
	}
	rs := []rune(s)
	if len(rs) == 0 || !isQuote(rs[0]) {
		t.Fatalf("%q does not start with a single quote", s)
	}
	var v []rune
	for i := 1; i < len(rs); i++ {
		if !isQuote(rs[i]) {
			v = append(v, rs[i])
			continue
		}
		if i+1 < len(rs) && isQuote(rs[i+1]) {
			v = append(v, rs[i+1])
			i++
			continue
		}
		return string(v), string(rs[i+1:])
	}
	t.Fatalf("%q has no closing quote", s)
	return "", ""
}
