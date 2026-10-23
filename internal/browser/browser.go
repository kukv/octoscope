// Package browser opens a URL in whatever the machine uses to read the web.
package browser

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"
)

// The whole of the environment this package reads. The first two name the
// browser to run, in that order of precedence; the third says this is WSL.
// browserEnv and isWSL carry the why.
const (
	envGHBrowser = "GH_BROWSER"
	envBrowser   = "BROWSER"
	envWSLDistro = "WSL_DISTRO_NAME"
)

// The values runtime.GOOS takes that this package knows how to open a URL on.
const (
	osWindows = "windows"
	osDarwin  = "darwin"
	osLinux   = "linux"
)

// NoneError says nothing on this machine can open a URL. It carries the URL
// so the UI can print it and let the user open it by hand, which is the whole
// of what can be done about an environment with no browser
// (.claude/rules/errors.md).
type NoneError struct{ URL string }

func (e *NoneError) Error() string {
	return fmt.Sprintf("no browser to open %s", e.URL)
}

func Open(rawURL string) error {
	if err := validate(rawURL); err != nil {
		return err
	}
	argv, ok := command(environment{
		goos:    runtime.GOOS,
		wsl:     isWSL(),
		browser: browserEnv(),
	}, rawURL)
	if !ok {
		return &NoneError{URL: rawURL}
	}
	// LookPath separates "this machine has no such program" from "the program
	// ran and failed". Only the first is the environment's doing, and only the
	// first is worth telling the user how to work around.
	if _, err := exec.LookPath(argv[0]); err != nil {
		return &NoneError{URL: rawURL}
	}
	// Start, not Run: a browser started from a terminal keeps running until
	// its window closes, and octoscope has a screen to draw in the meantime.
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap it. Whatever the launcher exits with says nothing about whether
	// the page opened, but without a Wait it stays a zombie until octoscope
	// itself exits, and one is left behind every time o is pressed.
	go func() { _ = cmd.Wait() }()
	return nil
}

// validate lets through only a web page. Not every URL comes from GitHub: the
// checks view opens whatever a check's author wrote as its details URL, and
// anything but http(s) would be a file, a handler on the user's machine, or a
// flag to the opener. %q keeps control characters in it off the screen.
func validate(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("refuse to open %q: not an http or https URL", rawURL)
	}
	return nil
}

// environment is what the machine says about itself, reduced to the three
// things that decide which program can open a URL.
type environment struct {
	goos    string
	wsl     bool
	browser string
}

// opener is one way of opening a URL. command returns the program to run and
// its arguments, the URL among them, or nil when the environment is not one
// this opener handles.
type opener interface {
	command(env environment, url string) []string
}

// openers are tried in order, and the order is part of the specification: a
// browser the user named wins everywhere, and WSL has to be recognised before
// plain Linux because a WSL install has no xdg-open. Reordering this changes
// behaviour, so both cases are pinned in browser_test.go.
var openers = []opener{
	userNamed{},
	windowsHandler{},
	macOpen{},
	wslBridge{},
	xdgOpen{},
}

// command names the program to run and its arguments. ok is false when this
// machine has nothing to try.
func command(env environment, url string) ([]string, bool) {
	for _, o := range openers {
		if argv := o.command(env, url); argv != nil {
			return argv, true
		}
	}
	return nil, false
}

// userNamed runs the browser the user named. The value is the executable; it
// is never handed to a shell (.claude/rules/go-style.md).
type userNamed struct{}

func (userNamed) command(env environment, url string) []string {
	if env.browser == "" {
		return nil
	}
	return []string{env.browser, url}
}

type windowsHandler struct{}

func (windowsHandler) command(env environment, url string) []string {
	if env.goos != osWindows {
		return nil
	}
	return []string{"rundll32", "url.dll,FileProtocolHandler", url}
}

type macOpen struct{}

func (macOpen) command(env environment, url string) []string {
	if env.goos != osDarwin {
		return nil
	}
	return []string{"open", url}
}

// wslBridge reaches Windows from WSL. explorer.exe would do it too, but it
// returns exit code 1 even when it worked, so a caller cannot tell a failure
// from a success. wslview is not an option: wslu is no longer packaged for
// Ubuntu 26.04.
//
// -Command would read the URL as script: ; would start a second statement and
// $(...) would run. -EncodedCommand takes the script as base64, which no
// quoting between WSL and Windows can alter, and the URL sits in it as a
// single-quoted literal, where nothing but a quote means anything.
type wslBridge struct{}

func (wslBridge) command(env environment, url string) []string {
	if !env.wsl {
		return nil
	}
	script := "Start-Process " + singleQuote(url)
	return []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(script)}
}

// singleQuote makes s a PowerShell single-quoted string. PowerShell takes
// the typographic single quotes (U+2018 to U+201B) for ' too, and in each
// case two in a row stand for one.
func singleQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'', '‘', '’', '‚', '‛':
			b.WriteRune(r)
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}

// encodePowerShell is what -EncodedCommand reads: base64 of UTF-16LE.
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}

type xdgOpen struct{}

func (xdgOpen) command(env environment, url string) []string {
	if env.goos != osLinux {
		return nil
	}
	return []string{"xdg-open", url}
}

// browserEnv reads the variables gh honours, in gh's own order of precedence
// (`gh help environment`). Following gh here means a user who has already
// told gh which browser to use does not have to tell octoscope again.
func browserEnv() string {
	if b := os.Getenv(envGHBrowser); b != "" {
		return b
	}
	return os.Getenv(envBrowser)
}

// isWSL reports whether this is a Linux running inside Windows. WSL's kernel
// names itself in /proc/sys/kernel/osrelease ("microsoft-standard-WSL2");
// WSL_DISTRO_NAME is the second signal, for a kernel built without the name.
func isWSL() bool {
	if os.Getenv(envWSLDistro) != "" {
		return true
	}
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(release)), "microsoft")
}
