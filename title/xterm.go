package title

// xterm sets the terminal window title with OSC 2. Inside tmux this also sets
// the pane title (tmux's allow-set-title is on by default); inside herdr it
// becomes the pane's terminal_title, shown in sidebar agent rows and the
// outer window title, not on the pane border.
//
// The title the terminal had before cyberspice can't be read back, so the
// host saves and restores it around the whole run with PushXTermTitle /
// PopXTermTitle; Close only resets it to a neutral name.
type xterm struct {
	set func(string)
}

// Write PushXTermTitle before the TUI starts and PopXTermTitle after it
// exits. They use the xterm title stack (CSI 22/23 t); terminals without it
// ignore them.
const (
	PushXTermTitle = "\x1b[22;0t"
	PopXTermTitle  = "\x1b[23;0t"
)

const xtermIdleTitle = "cyberspice"

func newXTerm(env Env) (Target, error) {
	if env.SetXTermTitle == nil {
		return nil, ErrUnavailable
	}
	return &xterm{set: env.SetXTermTitle}, nil
}

func (x *xterm) Set(text string) error {
	if text == "" {
		text = xtermIdleTitle
	}
	x.set(text)
	return nil
}

func (x *xterm) Close() error { return x.Set("") }
