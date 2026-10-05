package main

import (
	"cyberspice/nowplaying"
	"cyberspice/title"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// titleModes is the cycle order of the "t" key: where the now-playing track
// is shown. Modes that don't apply here (no tmux or herdr) are skipped.
//
//	off          nowhere
//	app          a line at the top of cyberspice
//	multiplexer  herdr: this space's name       tmux: the window name
//	xterm        the terminal window title (OSC 2)
//	pane         herdr: the pane border          tmux: the pane title
var titleModes = []string{"off", "app", "multiplexer", "xterm", "pane"}

// normalizeTitleMode canonicalises a title mode name; unknown warns → off.
func normalizeTitleMode(s string) string {
	switch strings.ToLower(s) {
	case "off", "none", "":
		return "off"
	case "app", "top", "header":
		return "app"
	case "multiplexer", "mux", "space", "window", "tmux", "herdr":
		return "multiplexer"
	case "xterm", "terminal", "term":
		return "xterm"
	case "pane":
		return "pane"
	default:
		fmt.Fprintf(os.Stderr, "unknown title mode %q, using off\n", s)
		return "off"
	}
}

// titleTargets maps a mode to the title targets that implement it here.
// "app" is drawn by View, so it needs none.
func titleTargets(mode string) []string {
	switch mode {
	case "xterm":
		return []string{"xterm"}
	case "multiplexer":
		if title.InHerdr() {
			return []string{"herdr-space"}
		}
		if title.InTmux() {
			return []string{"tmux-window"}
		}
	case "pane":
		if title.InHerdr() {
			return []string{"herdr-pane"}
		}
		if title.InTmux() {
			return []string{"tmux-pane"}
		}
	}
	return nil
}

// titleModeAvailable reports whether mode can be shown in this terminal.
func titleModeAvailable(mode string) bool {
	return mode == "off" || mode == "app" || len(titleTargets(mode)) > 0
}

// titleModeDesc names where a mode shows the track, for the status line.
func titleModeDesc(mode string) string {
	switch mode {
	case "app":
		return "top line"
	case "xterm":
		return "window title"
	case "multiplexer", "pane":
		if names := titleTargets(mode); len(names) > 0 {
			return strings.ReplaceAll(names[0], "-", " ")
		}
	}
	return mode
}

// trackMsg carries the now-playing label ("" = nothing playing) to the model.
type trackMsg string

// titleStatusMsg reports a title target failure on the status line.
type titleStatusMsg string

// titler follows MPRIS and keeps the active mode's targets up to date. It
// runs on its own goroutine: tmux and herdr updates exec a CLI, which must
// never stall a frame.
type titler struct {
	players string
	mode    atomic.Value // string
	poke    chan struct{}
	quit    chan struct{}
	done    chan struct{}
	watcher *nowplaying.Watcher
}

func newTitler(players string) *titler {
	t := &titler{
		players: players,
		poke:    make(chan struct{}, 1),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	t.mode.Store("off")
	return t
}

// Watch starts playerctl. Call it before Run; without it the titler is inert.
func (t *titler) Watch() error {
	w, err := nowplaying.Watch(t.players)
	if err != nil {
		return err
	}
	t.watcher = w
	return nil
}

// SetMode switches where the track is shown. Safe from Update: it never
// blocks, and the goroutine picks up the latest mode.
func (t *titler) SetMode(mode string) {
	t.mode.Store(mode)
	select {
	case t.poke <- struct{}{}:
	default:
	}
}

// Run starts the update goroutine, feeding trackMsg into p.
func (t *titler) Run(p *tea.Program) {
	if t.watcher == nil {
		close(t.done)
		return
	}
	go t.loop(p)
}

// Stop restores every target and waits for the goroutine. Call it after the
// program exits; p.Send is a no-op by then.
func (t *titler) Stop() {
	select {
	case <-t.done:
		return
	default:
	}
	close(t.quit)
	<-t.done
}

func (t *titler) loop(p *tea.Program) {
	defer close(t.done)
	env := title.Env{SetXTermTitle: func(s string) { p.Send(tea.SetWindowTitle(s)()) }}

	var (
		group title.Group
		mode  string
		label string
	)
	apply := func() {
		want := t.mode.Load().(string)
		if want == mode {
			return
		}
		_ = group.Close()
		group, mode = nil, want
		for _, name := range titleTargets(want) {
			tg, err := title.New(name, env)
			if err != nil {
				p.Send(titleStatusMsg(fmt.Sprintf("title %s: %v", name, err)))
				continue
			}
			group = append(group, tg)
		}
		_ = group.Set(label)
	}
	apply()

	tracks := t.watcher.Tracks()
	for {
		select {
		case tr, ok := <-tracks:
			if !ok { // playerctl exited; keep the last label
				tracks = nil
				continue
			}
			if l := tr.Label(); l != label {
				label = l
				p.Send(trackMsg(label))
				_ = group.Set(label)
			}
		case <-t.poke:
			apply()
		case <-t.quit:
			t.watcher.Stop()
			if tracks != nil {
				go func() {
					for range tracks {
					}
				}()
			}
			_ = group.Close()
			return
		}
	}
}

// activeTitleMode is the requested mode if it works here, else "off".
func (m model) activeTitleMode() string {
	if m.titleOK && titleModeAvailable(m.titleMode) {
		return m.titleMode
	}
	return "off"
}

// vizHeight is the height left for the visualizer: the "app" mode takes the
// top line.
func (m model) vizHeight() int {
	if m.activeTitleMode() == "app" {
		return m.height - 1
	}
	return m.height
}

// titleLine renders the "app" mode's now-playing line, cut to the width.
func (m model) titleLine() string {
	text, style := m.nowPlaying, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF"))
	if text == "" {
		text, style = "♪", lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	}
	return style.MaxWidth(m.width).Render(text)
}

// cycleTitleMode moves to the next mode that works here and resizes the
// visualizer, since "app" reserves a line.
func (m *model) cycleTitleMode() {
	m.statusExpiry = time.Now().Add(2 * time.Second)
	if !m.titleOK {
		m.status = "title: playerctl not running"
		return
	}
	cur := m.activeTitleMode()
	i := 0
	for j, mode := range titleModes {
		if mode == cur {
			i = j
		}
	}
	for {
		i = (i + 1) % len(titleModes)
		if titleModeAvailable(titleModes[i]) {
			break
		}
	}
	m.titleMode = titleModes[i]
	m.titler.SetMode(m.titleMode)
	m.status = "title: " + m.titleMode
	if desc := titleModeDesc(m.titleMode); desc != m.titleMode {
		m.status += " (" + desc + ")"
	}

	m.renderer.SetTerminalSize(m.width, m.vizHeight())
	if m.autoBands {
		m.resize(computeBandsFor(m.layout, m.width, m.vizHeight()))
	}
}
