package main

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/muesli/termenv"
)

// resolveBackground turns the -bg / `background` setting into the colour
// fades end at (viz.SetBackground). "auto" asks the terminal for its
// background with an OSC 11 query; a terminal that doesn't answer (or a
// multiplexer termenv won't query) leaves the old black target. Anything
// else must be a "#rrggbb" colour. Runs before Bubble Tea takes the terminal,
// since the reply arrives on stdin.
func resolveBackground(setting string) string {
	if !strings.EqualFold(setting, "auto") && setting != "" {
		return setting
	}
	if c, ok := termenv.NewOutput(os.Stdout).BackgroundColor().(termenv.RGBColor); ok {
		return strings.ToLower(string(c))
	}
	return ""
}

// bgColorMsg carries a background colour ("#rrggbb") the terminal reported
// after a theme change; Update hands it to viz.SetBackground.
type bgColorMsg string

// bgReplyPrefix starts the terminal's OSC 11 reply,
// e.g. "\x1b]11;rgb:2222/2b2b/3030\x1b\\".
const bgReplyPrefix = "\x1b]11;"

// bgQuery asks the terminal for its background. ST-terminated: herdr ignores
// the BEL form.
const bgQuery = "\x1b]11;?\x1b\\"

// bgInput is stdin with OSC 11 replies cut out before Bubble Tea sees them.
// Bubble Tea v1 doesn't parse OSC, so a reply would otherwise arrive as
// keypresses — and "1" and "]" are hotkeys. Each reply goes to onColor
// instead. Embedding *os.File keeps Fd, so Bubble Tea still puts the TTY in
// raw mode, and cancelreader still waits on it with epoll before each Read.
type bgInput struct {
	*os.File
	onColor func(hex string)
	pending []byte // a reply split across reads
	queued  []byte // filtered input that didn't fit the caller's buffer
}

func (b *bgInput) Read(p []byte) (int, error) {
	if len(b.queued) == 0 {
		buf := make([]byte, len(p))
		n, err := b.File.Read(buf)
		data := append(b.pending, buf[:n]...)
		b.pending = nil
		b.queued = b.strip(data)
		if len(b.queued) == 0 {
			return 0, err // all of it was a reply (or the start of one)
		}
	}
	n := copy(p, b.queued)
	b.queued = b.queued[n:]
	return n, nil
}

// strip removes complete OSC 11 replies from data, reporting each one, and
// keeps a trailing incomplete reply in b.pending for the next Read.
func (b *bgInput) strip(data []byte) []byte {
	var out []byte
	for {
		i := bytes.Index(data, []byte(bgReplyPrefix))
		if i < 0 {
			return append(out, data...)
		}
		out = append(out, data[:i]...)
		body := data[i+len(bgReplyPrefix):]
		end, termLen := bytes.IndexByte(body, '\a'), 1
		if st := bytes.Index(body, []byte("\x1b\\")); st >= 0 && (end < 0 || st < end) {
			end, termLen = st, 2
		}
		if end < 0 {
			if len(body) > 64 { // not a reply after all; let it through
				return append(out, data[i:]...)
			}
			b.pending = append([]byte(nil), data[i:]...)
			return out
		}
		if hex, ok := parseOSCColor(string(body[:end])); ok && b.onColor != nil {
			b.onColor(hex)
		}
		data = body[end+termLen:]
	}
}

// parseOSCColor converts an XParseColor "rgb:R/G/B" spec (1-4 hex digits per
// channel) to "#rrggbb".
func parseOSCColor(s string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(s, "rgb:"), "/")
	if len(parts) != 3 || !strings.HasPrefix(s, "rgb:") {
		return "", false
	}
	out := "#"
	for _, p := range parts {
		if len(p) < 1 || len(p) > 4 {
			return "", false
		}
		v, err := strconv.ParseUint(p, 16, 16)
		if err != nil {
			return "", false
		}
		max := uint64(1)<<(4*len(p)) - 1
		out += fmt.Sprintf("%02x", (v*255+max/2)/max)
	}
	return out, true
}

// watchBackground re-queries the terminal's background on SIGUSR1
// (ghostty-theme-toggle sends it). The terminal (or herdr) picks up a theme
// reload a moment after the signal, so it asks a few times as it settles;
// replies come back through bgInput.
func watchBackground() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGUSR1)
	go func() {
		for range sig {
			for _, d := range []time.Duration{200 * time.Millisecond, 800 * time.Millisecond, 2 * time.Second} {
				time.Sleep(d)
				os.Stdout.WriteString(bgQuery)
			}
		}
	}()
}
