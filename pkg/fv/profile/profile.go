// Package profile detects the host terminal's capabilities once at startup
// and exposes a singleton Profile that the rest of fv-go consults.
//
// Ported from FVProfile.pas. Detection rules track what the Pascal version
// did, but the VT-mode probe — Windows-only there — is delegated to the
// term backend (which calls SetVTProbe before InitFromEnv runs). On Unix
// the probe defaults to true if stdout is a tty.
package profile

import (
	"os"
	"strings"
	"sync"
)

// ColorSystem is the coarse color-capability class. Drives RGB-to-palette
// downsampling decisions.
type ColorSystem int

const (
	NoColors  ColorSystem = iota // NO_COLOR / not a tty
	Legacy                       // 16-color SGR
	EightBit                     // 256-color SGR
	TrueColor                    // 24-bit RGB SGR
)

// Profile is the runtime view of terminal capabilities.
type Profile struct {
	AnsiSupported    bool
	Interactive      bool
	LegacyConsole    bool
	Unicode          bool
	HyperlinkSupport bool
	SixelSupport     bool
	IsCI             bool
	ColorSystem      ColorSystem
}

var (
	mu          sync.RWMutex
	current     Profile
	initialized bool
	vtProbeOk   bool
	vtProbeSet  bool
)

// SetVTProbe lets the term backend report whether VT escapes are usable.
// On Windows this means ENABLE_VIRTUAL_TERMINAL_PROCESSING was accepted;
// on Unix it's "stdout is a real tty". Call before Init / Get to override
// the default heuristic.
func SetVTProbe(ok bool) {
	mu.Lock()
	vtProbeOk = ok
	vtProbeSet = true
	initialized = false // force re-detection
	mu.Unlock()
}

// Init runs the detection. Safe to call multiple times.
func Init() {
	mu.Lock()
	defer mu.Unlock()
	current = detect(vtProbeSet, vtProbeOk)
	initialized = true
}

// Get returns the current Profile, running Init lazily on first call.
func Get() Profile {
	mu.RLock()
	if initialized {
		p := current
		mu.RUnlock()
		return p
	}
	mu.RUnlock()
	Init()
	mu.RLock()
	defer mu.RUnlock()
	return current
}

func envDefined(name string) bool { return os.Getenv(name) != "" }

func envEquals(name, value string) bool {
	return strings.EqualFold(os.Getenv(name), value)
}

func envContains(name, needle string) bool {
	v := strings.ToLower(os.Getenv(name))
	return v != "" && strings.Contains(v, strings.ToLower(needle))
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func detectIsCI() bool {
	switch {
	case envEquals("GITHUB_ACTIONS", "true"),
		envDefined("APPVEYOR"),
		envEquals("TRAVIS", "true"),
		envEquals("GITLAB_CI", "true"),
		envDefined("JENKINS_URL"),
		envDefined("TEAMCITY_VERSION"),
		envDefined("BITBUCKET_BUILD_NUMBER"),
		envDefined("CI"):
		return true
	}
	return false
}

func detectColorSystem(vtOk bool) ColorSystem {
	if envDefined("NO_COLOR") {
		return NoColors
	}
	if envContains("COLORTERM", "truecolor") || envContains("COLORTERM", "24bit") {
		return TrueColor
	}
	if envContains("TERM", "256color") {
		return EightBit
	}
	if vtOk {
		return TrueColor
	}
	return Legacy
}

func detectAnsi(vtOk bool) bool {
	if envEquals("CLICOLOR_FORCE", "1") {
		return true
	}
	if !stdoutIsTTY() {
		return false
	}
	return vtOk
}

func detectHyperlinks(ansiOk bool) bool {
	if !ansiOk {
		return false
	}
	if envDefined("WT_SESSION") {
		return true
	}
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "iterm.app", "wezterm", "vscode", "hyper", "tabby", "apple_terminal":
		return true
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "xterm-direct") ||
		strings.Contains(term, "xterm-kitty") ||
		strings.Contains(term, "truecolor") {
		return true
	}
	return envDefined("KITTY_WINDOW_ID")
}

func detectSixel(ansiOk bool) bool {
	if !ansiOk {
		return false
	}
	if envDefined("WT_SESSION") {
		return true
	}
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "wezterm", "mintty":
		return true
	}
	term := strings.ToLower(os.Getenv("TERM"))
	return strings.Contains(term, "xterm-kitty") || strings.Contains(term, "mlterm")
}

func detect(probeSet, vtOk bool) Profile {
	if !probeSet {
		// Default: if no backend has answered yet, assume VT works iff
		// stdout is a tty. Backend can override later via SetVTProbe.
		vtOk = stdoutIsTTY()
	}
	isCI := detectIsCI()
	ansi := detectAnsi(vtOk)
	return Profile{
		AnsiSupported:    ansi,
		Interactive:      !isCI && stdoutIsTTY(),
		LegacyConsole:    stdoutIsTTY() && !vtOk,
		Unicode:          true, // Go strings are always UTF-8; the terminal handles display
		HyperlinkSupport: detectHyperlinks(ansi),
		SixelSupport:     detectSixel(ansi),
		IsCI:             isCI,
		ColorSystem:      detectColorSystem(vtOk),
	}
}
