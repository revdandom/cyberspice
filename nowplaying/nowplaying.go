// Package nowplaying follows the current track over MPRIS.
//
// It runs `playerctl --all-players --follow`, which watches the session bus
// for org.mpris.MediaPlayer2.* players and prints one line per track or
// play/pause change on any of them, so there is no polling. Read-only: it
// never controls a player.
//
// Following every player and choosing here matters: plain `--follow` with a
// --player list locks onto the first listed player that exists, so a stopped
// MPD would hide a YouTube video playing in the browser.
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
// ("" = every player). playerctl matches the part of the bus name before
// the first dot, so an app registered as "io.github.lullabyX.sone" is
// selected with "io". The list order is a priority: a playing player beats
// a paused one, then an earlier list entry beats a later one, then the most
// recently changed player wins.
func Watch(players string) (*Watcher, error) {
	args := []string{"--all-players", "--follow", "metadata", "--format", "{{playerInstance}}\t{{status}}\t{{artist}}\t{{title}}"}
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
		s := newSelector(players)
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			f := strings.SplitN(sc.Text(), "\t", 4)
			for len(f) < 4 {
				f = append(f, "")
			}
			s.update(f[0], Track{Status: f[1], Artist: f[2], Title: f[3]})
			s.prune(livePlayers())
			w.tracks <- s.current()
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

// livePlayers lists the player instances on the bus, or nil if playerctl
// fails (then nothing is pruned).
func livePlayers() map[string]bool {
	out, err := exec.Command("playerctl", "--list-all").Output()
	if err != nil {
		return nil
	}
	live := map[string]bool{}
	for _, name := range strings.Fields(string(out)) {
		live[name] = true
	}
	return live
}

type entry struct {
	track Track
	rank  int // index into the --player list
	seq   int // when it last changed
}

// selector remembers every player's latest track and picks the one to show.
type selector struct {
	order   []string // --player list; nil = every player, equal rank
	players map[string]*entry
	seq     int
}

func newSelector(players string) *selector {
	s := &selector{players: map[string]*entry{}}
	if players != "" {
		s.order = strings.Split(players, ",")
	}
	return s
}

// rank is the first --player entry matching instance, as playerctl matches:
// the name before the first dot, or "%any".
func (s *selector) rank(instance string) int {
	name, _, _ := strings.Cut(instance, ".")
	for i, p := range s.order {
		if p == "%any" || p == name || p == instance {
			return i
		}
	}
	return len(s.order)
}

func (s *selector) update(instance string, t Track) {
	if instance == "" {
		return
	}
	s.seq++
	s.players[instance] = &entry{track: t, rank: s.rank(instance), seq: s.seq}
}

// prune drops players that have left the bus. live == nil keeps them all.
func (s *selector) prune(live map[string]bool) {
	if live == nil {
		return
	}
	for name := range s.players {
		if !live[name] {
			delete(s.players, name)
		}
	}
}

// current is the playing player with the best rank, else the paused one,
// ties going to the most recent change; Track{} when neither exists.
func (s *selector) current() Track {
	for _, status := range []string{"Playing", "Paused"} {
		var best *entry
		for _, e := range s.players {
			if e.track.Status != status || e.track.Title == "" {
				continue
			}
			if best == nil || e.rank < best.rank || (e.rank == best.rank && e.seq > best.seq) {
				best = e
			}
		}
		if best != nil {
			return best.track
		}
	}
	return Track{}
}
