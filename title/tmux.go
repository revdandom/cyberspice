package title

import (
	"os"
	"strings"
)

// tmuxWindow names this pane's window, which shows in the tmux status bar.
// rename-window turns automatic-rename off for the window, so Close restores
// the old name and switches automatic-rename back on if it was on.
type tmuxWindow struct {
	pane       string
	original   string
	autoRename bool
}

func newTmuxWindow() (Target, error) {
	pane := os.Getenv("TMUX_PANE")
	out, err := run("tmux", "display-message", "-p", "-t", pane, "#{automatic-rename}\t#{window_name}")
	if err != nil {
		return nil, err
	}
	auto, name, _ := strings.Cut(out, "\t")
	return &tmuxWindow{pane: pane, original: name, autoRename: auto == "1"}, nil
}

func (t *tmuxWindow) Set(text string) error {
	if text == "" {
		return t.Close()
	}
	_, err := run("tmux", "rename-window", "-t", t.pane, text)
	return err
}

func (t *tmuxWindow) Close() error {
	if _, err := run("tmux", "rename-window", "-t", t.pane, t.original); err != nil {
		return err
	}
	if t.autoRename {
		_, err := run("tmux", "set-window-option", "-t", t.pane, "automatic-rename", "on")
		return err
	}
	return nil
}

// tmuxPane sets this pane's title (#{pane_title}) with select-pane -T. It's
// visible only where pane-border-format or set-titles uses #{pane_title}.
type tmuxPane struct {
	pane     string
	original string
}

func newTmuxPane() (Target, error) {
	pane := os.Getenv("TMUX_PANE")
	original, err := run("tmux", "display-message", "-p", "-t", pane, "#{pane_title}")
	if err != nil {
		return nil, err
	}
	return &tmuxPane{pane: pane, original: original}, nil
}

func (t *tmuxPane) Set(text string) error {
	if text == "" {
		text = t.original
	}
	_, err := run("tmux", "select-pane", "-t", t.pane, "-T", text)
	return err
}

func (t *tmuxPane) Close() error { return t.Set("") }
