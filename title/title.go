// Package title publishes a short status string (the now-playing track) to
// the places a terminal user sees titles: the xterm window title, tmux
// window names and pane titles, and herdr spaces and panes.
//
// Each destination is a Target. Add one by implementing Target and
// registering it in New and Available.
package title

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Target is one place a title can be shown.
type Target interface {
	// Set shows text; "" means nothing is playing, and each target picks
	// its own idle state (restore the original, show a placeholder, ...).
	Set(text string) error
	// Close restores whatever the target showed before cyberspice used it.
	Close() error
}

// ErrUnavailable means the target doesn't apply here (e.g. tmux outside
// tmux).
var ErrUnavailable = errors.New("title target not available in this environment")

// Names lists the target names New accepts.
var Names = []string{"xterm", "tmux-window", "tmux-pane", "herdr-space", "herdr-pane"}

// Env carries what targets need from the host program.
type Env struct {
	// SetXTermTitle writes an OSC 2 title through the TUI's renderer, so it
	// never interleaves with a frame. Required for "xterm".
	SetXTermTitle func(string)
}

// InHerdr reports whether this process runs in a herdr-managed pane, where
// the herdr CLI talks to the session the terminal belongs to.
func InHerdr() bool { return os.Getenv("HERDR_ENV") == "1" && os.Getenv("HERDR_PANE_ID") != "" }

// InTmux reports whether this process runs inside a tmux pane.
func InTmux() bool { return os.Getenv("TMUX") != "" && os.Getenv("TMUX_PANE") != "" }

// Available reports whether the named target can work here, without
// touching anything.
func Available(name string) bool {
	switch name {
	case "xterm":
		return true
	case "tmux-window", "tmux-pane":
		return InTmux()
	case "herdr-space", "herdr-pane":
		return InHerdr()
	}
	return false
}

// New builds the named target. It returns ErrUnavailable when the target
// doesn't apply here, or an error for an unknown name.
func New(name string, env Env) (Target, error) {
	if !Available(name) {
		for _, n := range Names {
			if n == name {
				return nil, ErrUnavailable
			}
		}
		return nil, fmt.Errorf("unknown title target %q (want one of %s)", name, strings.Join(Names, ", "))
	}
	switch name {
	case "xterm":
		return newXTerm(env)
	case "tmux-window":
		return newTmuxWindow()
	case "tmux-pane":
		return newTmuxPane()
	case "herdr-space":
		return newHerdrSpace()
	default: // "herdr-pane"
		return newHerdrPane()
	}
}

// Group fans each call out to several targets.
type Group []Target

// Set updates every target, returning the first error.
func (g Group) Set(text string) error {
	var first error
	for _, t := range g {
		if err := t.Set(text); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Close restores every target, returning the first error.
func (g Group) Close() error {
	var first error
	for _, t := range g {
		if err := t.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// run executes a helper CLI (tmux, herdr) with a timeout, so a wedged server
// can't stall title updates or cyberspice's exit.
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out)), err
}
