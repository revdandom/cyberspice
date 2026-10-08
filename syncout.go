package main

import "os"

// Synchronized output (DEC private mode 2026): a terminal that supports it
// holds the screen until the end marker, then shows the whole frame at once,
// so a big repaint never appears half-drawn (tearing). Terminals without it
// ignore the sequences. bubbletea v1 doesn't emit them itself, so its output
// is wrapped.
const (
	syncBegin = "\x1b[?2026h"
	syncEnd   = "\x1b[?2026l"
)

// syncOutput is os.Stdout with every Write bracketed by the sync markers.
// bubbletea flushes each frame in a single Write, so one frame = one
// synchronized update. Embedding *os.File keeps Fd/Read/Close, so bubbletea
// still sees a TTY (raw mode, window size).
type syncOutput struct{ *os.File }

func (s syncOutput) Write(p []byte) (int, error) {
	buf := make([]byte, 0, len(syncBegin)+len(p)+len(syncEnd))
	buf = append(buf, syncBegin...)
	buf = append(buf, p...)
	buf = append(buf, syncEnd...)
	if _, err := s.File.Write(buf); err != nil {
		return 0, err
	}
	return len(p), nil
}
