// Package browser opens a URL in whatever the machine uses to read the web.
package browser

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// NoneError says nothing on this machine can open a URL. It carries the URL
// so the UI can print it and let the user open it by hand, which is the whole
// of what can be done about an environment with no browser
// (.claude/rules/errors.md).
type NoneError struct{ URL string }

func (e *NoneError) Error() string { return "no browser to open " + e.URL }

// Open shows url in the user's browser.
func Open(url string) error {
	argv, ok := command(runtime.GOOS, isWSL(), browserEnv())
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

// browserEnv reads the variables gh honours, in gh's own order of precedence
// (`gh help environment`). Following gh here means a user who has already
// told gh which browser to use does not have to tell octoscope again.
func browserEnv() string {
	if b := os.Getenv("GH_BROWSER"); b != "" {
		return b
	}
	return os.Getenv("BROWSER")
}

// isWSL reports whether this is a Linux running inside Windows. WSL's kernel
// names itself in /proc/sys/kernel/osrelease ("microsoft-standard-WSL2");
// WSL_DISTRO_NAME is the second signal, for a kernel built without the name.
func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(release)), "microsoft")
}

// command names the program to run and the arguments before the URL. ok is
// false when this machine has nothing to try.
func command(goos string, wsl bool, browserEnv string) ([]string, bool) {
	// A browser the user named wins everywhere. The value is the executable;
	// it is never handed to a shell (.claude/rules/go-style.md).
	if browserEnv != "" {
		return []string{browserEnv}, true
	}
	switch {
	case goos == "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler"}, true
	case goos == "darwin":
		return []string{"open"}, true
	case wsl:
		// Reaching Windows from WSL. explorer.exe would do it too, but it
		// returns exit code 1 even when it worked, so a caller cannot tell a
		// failure from a success. wslview is not an option: wslu is no longer
		// packaged for Ubuntu 26.04.
		//
		// Start-Process re-parses what follows it, so a URL with & in it
		// would be cut short. GitHub item URLs carry no query string.
		return []string{"powershell.exe", "-NoProfile", "-Command", "Start-Process"}, true
	case goos == "linux":
		return []string{"xdg-open"}, true
	}
	return nil, false
}
