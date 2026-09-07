// Package browser opens a URL in whatever the machine uses to read the web.
package browser

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

// Open shows url in the user's browser.
func Open(url string) error {
	argv, ok := command(environment{
		goos:    runtime.GOOS,
		wsl:     isWSL(),
		browser: browserEnv(),
	})
	if !ok {
		return &NoneError{URL: url}
	}
	argv = append(argv, url)
	// LookPath separates "this machine has no such program" from "the program
	// ran and failed". Only the first is the environment's doing, and only the
	// first is worth telling the user how to work around.
	if _, err := exec.LookPath(argv[0]); err != nil {
		return &NoneError{URL: url}
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

// environment is what the machine says about itself, reduced to the three
// things that decide which program can open a URL.
type environment struct {
	goos    string
	wsl     bool
	browser string
}

// opener is one way of opening a URL. command returns the program to run and
// the arguments that go before the URL, or nil when the environment is not
// one this opener handles.
type opener interface {
	command(env environment) []string
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

// command names the program to run and the arguments before the URL. ok is
// false when this machine has nothing to try.
func command(env environment) ([]string, bool) {
	for _, o := range openers {
		if argv := o.command(env); argv != nil {
			return argv, true
		}
	}
	return nil, false
}

// userNamed runs the browser the user named. The value is the executable; it
// is never handed to a shell (.claude/rules/go-style.md).
type userNamed struct{}

func (userNamed) command(env environment) []string {
	if env.browser == "" {
		return nil
	}
	return []string{env.browser}
}

type windowsHandler struct{}

func (windowsHandler) command(env environment) []string {
	if env.goos != osWindows {
		return nil
	}
	return []string{"rundll32", "url.dll,FileProtocolHandler"}
}

type macOpen struct{}

func (macOpen) command(env environment) []string {
	if env.goos != osDarwin {
		return nil
	}
	return []string{"open"}
}

// wslBridge reaches Windows from WSL. explorer.exe would do it too, but it
// returns exit code 1 even when it worked, so a caller cannot tell a failure
// from a success. wslview is not an option: wslu is no longer packaged for
// Ubuntu 26.04.
//
// Start-Process re-parses what follows it, so a URL with & in it would be cut
// short. GitHub item URLs carry no query string.
type wslBridge struct{}

func (wslBridge) command(env environment) []string {
	if !env.wsl {
		return nil
	}
	return []string{"powershell.exe", "-NoProfile", "-Command", "Start-Process"}
}

type xdgOpen struct{}

func (xdgOpen) command(env environment) []string {
	if env.goos != osLinux {
		return nil
	}
	return []string{"xdg-open"}
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
