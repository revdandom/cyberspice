package title

import (
	"encoding/json"
	"errors"
	"os"
)

// herdrSpace shows the title as the name of the herdr space (workspace)
// cyberspice runs in, in the sidebar. Close puts the original name back.
// herdr can't reset a name to automatic (renaming to "" leaves it blank), so
// an automatic name comes back as the same text, fixed.
type herdrSpace struct {
	id       string
	original string
	last     string
	dead     bool // the space is gone; stop touching it
}

func newHerdrSpace() (Target, error) {
	// Ask for the pane's workspace instead of trusting HERDR_WORKSPACE_ID,
	// which is inherited at launch and goes stale if the pane is moved.
	out, err := run("herdr", "pane", "get", os.Getenv("HERDR_PANE_ID"))
	if err != nil {
		return nil, err
	}
	var pane struct {
		Result struct {
			Pane struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &pane); err != nil {
		return nil, err
	}
	id := pane.Result.Pane.WorkspaceID
	if id == "" {
		return nil, errors.New("herdr pane get returned no workspace_id")
	}

	out, err = run("herdr", "workspace", "get", id)
	if err != nil {
		return nil, err
	}
	var ws struct {
		Result struct {
			Workspace struct {
				Label string `json:"label"`
			} `json:"workspace"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &ws); err != nil {
		return nil, err
	}
	original := ws.Result.Workspace.Label
	return &herdrSpace{id: id, original: original, last: original}, nil
}

func (h *herdrSpace) Set(text string) error {
	if text == "" {
		text = h.original
	}
	if h.dead || text == h.last {
		return nil
	}
	if _, err := run("herdr", "workspace", "rename", h.id, text); err != nil {
		h.dead = true
		return err
	}
	h.last = text
	return nil
}

func (h *herdrSpace) Close() error {
	err := h.Set("")
	h.dead = true
	return err
}

// herdrPane names this pane, which shows on its split border and overrides
// the agent label. herdr's CLI can't read a manual pane name back, so Close
// clears the name rather than restoring one set by hand.
type herdrPane struct {
	pane string
}

func newHerdrPane() (Target, error) {
	return &herdrPane{pane: os.Getenv("HERDR_PANE_ID")}, nil
}

func (h *herdrPane) Set(text string) error {
	args := []string{"pane", "rename", h.pane, text}
	if text == "" {
		args = []string{"pane", "rename", h.pane, "--clear"}
	}
	_, err := run("herdr", args...)
	return err
}

func (h *herdrPane) Close() error { return h.Set("") }
