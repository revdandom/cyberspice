// Package nowplaying follows the current track over MPRIS.
//
// It runs `playerctl --follow`, which watches the session bus for
// org.mpris.MediaPlayer2.* players and prints one line per track or
// play/pause change, so there is no polling. Read-only: it never controls
// a player.
package nowplaying

import (
	"bufio"
	"os/exec"
	"strings"
)

// Track is one MPRIS metadata snapshot.
type Track struct {
	Status string // "Playing" | "Paused" | "Stopped" | "" (no player)
	Artist string
	Title  string
}

// Label renders the track for a title bar: "▶ Title — Artist" while playing,
// "⏸ …" while paused, "" when nothing is playing.
func (t Track) Label() string {
	var icon string
	switch t.Status {
	case "Playing":
		icon = "▶"
	case "Paused":
		icon = "⏸"
	default:
		return ""
	}
	if t.Title == "" {
		return ""
	}
	if t.Artist == "" {
		return icon + " " + t.Title
	}
	return icon + " " + t.Title + " — " + t.Artist
}

// Watcher streams Track changes until Stop is called.
type Watcher struct {
	cmd    *exec.Cmd
	tracks chan Track
}

// Watch starts following players. players is a playerctl --player list
// ("" = playerctl's default choice). playerctl matches the part of the bus
// name before the first dot, so an app registered as
// "io.github.lullabyX.sone" is selected with "io".
func Watch(players string) (*Watcher, error) {
	args := []string{"--follow", "metadata", "--format", "{{status}}\t{{artist}}\t{{title}}"}
	if players != "" {
		args = append([]string{"--player=" + players}, args...)
	}
	cmd := exec.Command("playerctl", args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	w := &Watcher{cmd: cmd, tracks: make(chan Track)}
	go func() {
		defer close(w.tracks)
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			f := strings.SplitN(sc.Text(), "\t", 3)
			for len(f) < 3 {
				f = append(f, "")
			}
			w.tracks <- Track{Status: f[0], Artist: f[1], Title: f[2]}
		}
		_ = cmd.Wait()
	}()
	return w, nil
}

// Tracks delivers each change and is closed when playerctl exits. Keep
// reading it until it closes, or the reader goroutine blocks forever.
func (w *Watcher) Tracks() <-chan Track { return w.tracks }

// Stop kills playerctl; Tracks closes once its output drains.
func (w *Watcher) Stop() { _ = w.cmd.Process.Kill() }
