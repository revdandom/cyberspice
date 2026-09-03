package main

import (
	"cyberspec/audio"
	"cyberspec/dsp"
	"cyberspec/viz"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// options holds the effective configuration: viz defaults, overlaid by the
// config file, overlaid by CLI flags.
type options struct {
	barStyle  string
	scheme    viz.ColorScheme
	ampMode   string // "linear" | "stevens" | "db"
	bands     int    // 0 = auto-size to terminal
	gain      float64
	tilt      float64 // spectral tilt, dB/octave
	chrome    bool    // show header/footer bars on startup
	peakFall  bool    // peak marker falls after its hold (vs. fade-only)
	showPeaks bool    // draw the peak markers at all
	layout    string  // "vertical" | "butterfly"
	splash    bool    // show the HACKERMAN intro
}

// defaultOptions returns the built-in defaults (before config file / flags).
func defaultOptions() options {
	return options{
		barStyle:  viz.BAR_STYLE,
		scheme:    schemeFromName(viz.DEFAULT_COLOR_SCHEME),
		ampMode:   viz.AMPLITUDE_MODE_DEFAULT,
		bands:     0,
		gain:      viz.DEFAULT_GAIN,
		tilt:      viz.SPECTRAL_TILT_DB_PER_OCT,
		chrome:    viz.SHOW_CHROME_DEFAULT,
		peakFall:  viz.PEAK_FALL_DEFAULT,
		showPeaks: viz.SHOW_PEAKS_DEFAULT,
		layout:    viz.LAYOUT_DEFAULT,
		splash:    viz.SPLASH_ENABLED,
	}
}

// normalizeLayout canonicalises a layout name; unknown warns → default.
func normalizeLayout(s string) string {
	switch strings.ToLower(s) {
	case "vertical", "v", "bars":
		return "vertical"
	case "butterfly", "b", "horizontal", "h", "mirror":
		return "butterfly"
	default:
		fmt.Fprintf(os.Stderr, "unknown layout %q, using %s\n", s, viz.LAYOUT_DEFAULT)
		return viz.LAYOUT_DEFAULT
	}
}

// channel is one per-band pipeline stage: temporal smoothing + peak tracking.
// The vertical layout uses one (mono); butterfly uses two (left, right).
type channel struct {
	smoother *viz.Smoother
	peaks    *viz.PeakTracker
	bands    []float64 // latest smoothed + monstercat-spread magnitudes
}

func newChannel(n int, fall bool) *channel {
	c := &channel{
		smoother: viz.NewSmoother(n),
		peaks:    viz.NewPeakTracker(n),
		bands:    make([]float64, n),
	}
	c.peaks.SetFall(fall)
	return c
}

func (c *channel) update(bands []float64, deltaMs int64) {
	sb := c.smoother.Smooth(bands)
	sb = viz.SpreadNeighbors(sb, viz.MONSTERCAT_FACTOR)
	c.bands = sb
	c.peaks.Update(sb, deltaMs)
}

// model represents the application state for Bubbletea
type model struct {
	// Audio processing
	capturer *audio.Capturer
	fft      *dsp.FFTProcessor

	// Visualization
	renderer *viz.Renderer
	chans    []*channel  // 1 (vertical) or 2 (butterfly: left, right)
	splash   *viz.Splash // intro; nil once finished / dismissed

	// State
	currentScheme viz.ColorScheme
	gain          float64
	defaultGain   float64 // gain restored by the "0" key
	tiltDB        float64 // spectral tilt, dB/octave (adjusted with [ and ])
	peakFall      bool    // peak falling animation (toggled with 'f')
	layout        string  // "vertical" | "butterfly"
	splashEnabled bool    // launch option: show the intro (persisted, not live)
	lastUpdate    time.Time

	// Band count
	numBands  int
	autoBands bool // resize band count with the terminal

	// Transient status line (e.g. "config saved")
	status       string
	statusExpiry time.Time

	// Terminal dimensions
	width  int
	height int

	// Error state
	err error
}

// audioTickMsg is sent every frame to trigger audio processing
type audioTickMsg struct{}

// computeBandsFor derives the band count from the terminal size, clamped to
// [MIN_BANDS, MAX_BANDS]. Vertical packs bands across the width (BAND_COLUMNS
// each); butterfly stacks them up the height (BAND_ROWS each).
func computeBandsFor(layout string, w, h int) int {
	var n int
	if layout == "butterfly" {
		if h <= 0 {
			h = 24
		}
		n = h / viz.BAND_ROWS
	} else {
		if w <= 0 {
			w = 80
		}
		n = (w + 1) / viz.BAND_COLUMNS
	}
	if n < viz.MIN_BANDS {
		n = viz.MIN_BANDS
	}
	if n > viz.MAX_BANDS {
		n = viz.MAX_BANDS
	}
	return n
}

// resize rebuilds the per-band pipeline for a new band count. Smoother and
// peak state are reset (a brief visual blip); the FFT's AGC ceiling is kept.
func (m *model) resize(n int) {
	if n == m.numBands {
		return
	}
	m.numBands = n
	m.fft.SetNumBands(n)
	for i := range m.chans {
		m.chans[i] = newChannel(n, m.peakFall)
	}
}

// rebuildChannels swaps the channel set for the current layout (1 vertical,
// 2 butterfly) at n bands. Used when the layout is toggled.
func (m *model) rebuildChannels(n int) {
	want := 1
	if m.layout == "butterfly" {
		want = 2
	}
	m.chans = make([]*channel, want)
	for i := range m.chans {
		m.chans[i] = newChannel(n, m.peakFall)
	}
	m.numBands = n
	m.fft.SetNumBands(n)
}

// Initialize sets up the application
func initialModel(opts options) model {
	// Create audio capturer
	capturer, err := audio.NewCapturer()
	if err != nil {
		return model{err: fmt.Errorf("audio initialization failed: %w", err)}
	}

	autoBands := opts.bands <= 0
	nbands := opts.bands
	if autoBands {
		nbands = computeBandsFor(opts.layout, 80, 24)
	}

	// Create FFT processor
	fft := dsp.NewFFTProcessor(viz.SAMPLE_RATE)
	fft.SetNumBands(nbands)
	fft.SetTilt(opts.tilt)

	// Create renderer (will be updated with actual terminal size)
	renderer := viz.NewRenderer(80, 24, opts.scheme)
	renderer.SetBarStyle(opts.barStyle)
	renderer.SetTiltDisplay(opts.tilt)
	renderer.SetAmplitudeMode(opts.ampMode)
	renderer.SetChrome(opts.chrome)
	renderer.SetShowPeaks(opts.showPeaks)
	renderer.SetLayout(opts.layout)

	m := model{
		capturer:      capturer,
		fft:           fft,
		renderer:      renderer,
		currentScheme: opts.scheme,
		gain:          opts.gain,
		defaultGain:   opts.gain,
		tiltDB:        opts.tilt,
		peakFall:      opts.peakFall,
		layout:        opts.layout,
		splashEnabled: opts.splash,
		lastUpdate:    time.Now(),
		numBands:      nbands,
		autoBands:     autoBands,
		width:         80,
		height:        24,
	}
	m.rebuildChannels(nbands)
	if opts.splash {
		m.splash = viz.NewSplash(80, 24, opts.layout, opts.scheme)
	}
	return m
}

// Init initializes the Bubbletea program
func (m model) Init() tea.Cmd {
	// Start audio processing tick
	return tea.Batch(
		tickCmd(),
		tea.EnterAltScreen,
	)
}

// tickCmd generates tick messages at target FPS
func tickCmd() tea.Cmd {
	return tea.Tick(time.Duration(1000/viz.TARGET_FPS)*time.Millisecond, func(t time.Time) tea.Msg {
		return audioTickMsg{}
	})
}

// Update handles messages and updates the model
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Update terminal dimensions
		m.width = msg.Width
		m.height = msg.Height
		m.renderer.SetTerminalSize(m.width, m.height)
		if m.splash != nil {
			m.splash.Resize(msg.Width, msg.Height, m.layout)
		}
		if m.autoBands {
			m.resize(computeBandsFor(m.layout, msg.Width, msg.Height))
		}
		return m, nil

	case audioTickMsg:
		// Process audio and update visualization
		if err := m.processAudio(); err != nil {
			m.err = err
			return m, tea.Quit
		}
		// Advance the intro splash (audio keeps calibrating underneath it).
		if m.splash != nil {
			m.splash.Update()
			if m.splash.Done() {
				m.splash = nil
			}
		}

		// Schedule next tick
		return m, tickCmd()

	case tea.KeyMsg:
		// Any key dismisses the splash (except quit, handled below).
		if m.splash != nil {
			switch msg.String() {
			case "q", "esc", "ctrl+c":
				return m, tea.Quit
			}
			m.splash = nil
			return m, nil
		}
		return m.handleKey(msg)

	default:
		return m, nil
	}
}

// handleKey processes keyboard input
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		// Quit
		return m, tea.Quit

	case "c", "1", "2":
		// Cycle color scheme
		if msg.String() == "1" {
			m.currentScheme = viz.SchemeClassic
		} else if msg.String() == "2" {
			m.currentScheme = viz.SchemeSynthwave
		} else {
			// Cycle through schemes
			if m.currentScheme == viz.SchemeClassic {
				m.currentScheme = viz.SchemeSynthwave
			} else {
				m.currentScheme = viz.SchemeClassic
			}
		}
		m.renderer.SetColorScheme(m.currentScheme)

	case "s":
		// Cycle bar style
		m.renderer.CycleBarStyle()

	case "l":
		// Cycle layout (vertical <-> butterfly); rebuild for the new band
		// count and channel count.
		m.renderer.CycleLayout()
		m.layout = m.renderer.Layout()
		n := m.numBands
		if m.autoBands {
			n = computeBandsFor(m.layout, m.width, m.height)
		}
		m.rebuildChannels(n)

	case "a":
		// Cycle amplitude scale: linear -> stevens -> db
		m.renderer.CycleAmplitude()

	case "p":
		// Toggle the peak markers on/off
		m.renderer.ToggleShowPeaks()

	case "f":
		// Toggle the peak-marker falling animation (off = fade only)
		m.peakFall = !m.peakFall
		for _, c := range m.chans {
			c.peaks.SetFall(m.peakFall)
		}

	case "w":
		// Write current settings to ~/.config/cyberspec/config
		if path, err := writeConfig(m.currentOptions()); err != nil {
			m.status = "config save failed: " + err.Error()
		} else {
			m.status = "saved " + path
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)

	case "[":
		// Less high-frequency lift
		m.tiltDB -= 0.5
		if m.tiltDB < 0 {
			m.tiltDB = 0
		}
		m.fft.SetTilt(m.tiltDB)
		m.renderer.SetTiltDisplay(m.tiltDB)

	case "]":
		// More high-frequency lift
		m.tiltDB += 0.5
		if m.tiltDB > viz.SPECTRAL_TILT_MAX_SLOPE {
			m.tiltDB = viz.SPECTRAL_TILT_MAX_SLOPE
		}
		m.fft.SetTilt(m.tiltDB)
		m.renderer.SetTiltDisplay(m.tiltDB)

	case "+", "=":
		// Increase gain
		m.gain += viz.GAIN_STEP
		if m.gain > viz.MAX_GAIN {
			m.gain = viz.MAX_GAIN
		}

	case "-", "_":
		// Decrease gain
		m.gain -= viz.GAIN_STEP
		if m.gain < viz.MIN_GAIN {
			m.gain = viz.MIN_GAIN
		}

	case "0":
		// Reset gain to the launch value
		m.gain = m.defaultGain

	default:
		// Any key without an action toggles the header/footer bars, so a
		// curious keypress reveals (or hides) the controls.
		m.renderer.ToggleChrome()
	}

	return m, nil
}

// currentOptions snapshots the live settings for writing to the config file.
func (m model) currentOptions() options {
	bands := 0
	if !m.autoBands {
		bands = m.numBands
	}
	return options{
		barStyle:  m.renderer.BarStyle(),
		scheme:    m.currentScheme,
		ampMode:   m.renderer.AmplitudeMode(),
		bands:     bands,
		gain:      m.gain,
		tilt:      m.tiltDB,
		chrome:    m.renderer.Chrome(),
		peakFall:  m.peakFall,
		showPeaks: m.renderer.ShowPeaks(),
		layout:    m.layout,
		splash:    m.splashEnabled,
	}
}

// processAudio reads audio and updates visualization state. Each channel
// applies its own temporal + monstercat smoothing and peak tracking;
// butterfly runs L and R through one shared AGC so they scale identically.
func (m *model) processAudio() error {
	now := time.Now()
	deltaMs := now.Sub(m.lastUpdate).Milliseconds()
	m.lastUpdate = now

	if m.layout == "butterfly" && len(m.chans) == 2 {
		left, right := m.capturer.ReadStereo()
		rawL := m.fft.ProcessRaw(left)
		rawR := m.fft.ProcessRaw(right)
		m.fft.NormalizeShared(rawL, rawR)
		m.chans[0].update(rawL, deltaMs)
		m.chans[1].update(rawR, deltaMs)
		return nil
	}

	mono, err := m.capturer.ReadSamples()
	if err != nil {
		return fmt.Errorf("audio read failed: %w", err)
	}
	m.chans[0].update(m.fft.Process(mono), deltaMs)
	return nil
}

// View renders the current state
func (m model) View() string {
	// Check for errors
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.\n", m.err)
	}

	// Intro splash takes over the screen until it finishes.
	if m.splash != nil {
		return m.splash.Render()
	}

	// Get scheme name for display
	schemeName := "Classic"
	if m.currentScheme == viz.SchemeSynthwave {
		schemeName = "Synthwave"
	}

	// Render spectrum
	var out string
	if m.layout == "butterfly" && len(m.chans) == 2 {
		out = m.renderer.RenderButterfly(m.chans[0].bands, m.chans[1].bands,
			m.chans[0].peaks, m.chans[1].peaks, m.gain, schemeName)
	} else {
		out = m.renderer.Render(m.chans[0].bands, m.chans[0].peaks, m.gain, schemeName)
	}

	// Transient status line (e.g. after "w"), on top for a few seconds.
	if m.status != "" && time.Now().Before(m.statusExpiry) {
		return m.status + "\n" + out
	}
	return out
}

// schemeFromName maps a scheme name to its ColorScheme (unknown -> classic).
func schemeFromName(name string) viz.ColorScheme {
	switch strings.ToLower(name) {
	case "synthwave", "synth", "cyberpunk":
		return viz.SchemeSynthwave
	default:
		return viz.SchemeClassic
	}
}

// schemeName is the inverse of schemeFromName, for writing the config file.
func schemeName(s viz.ColorScheme) string {
	if s == viz.SchemeSynthwave {
		return "synthwave"
	}
	return "classic"
}

// normalizeAmp canonicalises an amplitude-mode name (with aliases); an
// unknown value warns and falls back to the default.
func normalizeAmp(s string) string {
	switch strings.ToLower(s) {
	case "linear", "raw", "amplitude":
		return "linear"
	case "stevens", "perceptual", "loudness", "gamma":
		return "stevens"
	case "db", "decibel", "log":
		return "db"
	default:
		fmt.Fprintf(os.Stderr, "unknown amp %q, using %s\n", s, viz.AMPLITUDE_MODE_DEFAULT)
		return viz.AMPLITUDE_MODE_DEFAULT
	}
}

// parseFlags applies command-line flags on top of `base` (defaults + config
// file). Only flags the user actually passed change anything.
func parseFlags(base options) options {
	style := flag.String("style", base.barStyle, "bar style: led, solid, braille, gradient")
	color := flag.String("color", schemeName(base.scheme), "color scheme: classic, synthwave")
	amp := flag.String("amp", base.ampMode, "amplitude scale: linear, stevens, db")
	bands := flag.Int("bands", base.bands, "number of frequency bands (0 = auto-size to terminal width)")
	gain := flag.Float64("gain", base.gain, "initial gain multiplier")
	tilt := flag.Float64("tilt", base.tilt, "spectral tilt: dB/octave high-frequency lift (0 = flat)")
	chrome := flag.Bool("chrome", base.chrome, "show the header/footer bars on startup")
	fall := flag.Bool("fall", base.peakFall, "peak marker falls after its hold (false = fade only)")
	peaks := flag.Bool("peaks", base.showPeaks, "draw the peak markers")
	layout := flag.String("layout", base.layout, "layout: vertical or butterfly (horizontal, stereo split)")
	splash := flag.Bool("splash", base.splash, "show the HACKERMAN intro")
	flag.Parse()

	opts := base
	opts.barStyle = strings.ToLower(*style)
	opts.scheme = schemeFromName(*color)
	opts.ampMode = normalizeAmp(*amp)
	opts.bands = *bands
	opts.gain = *gain
	opts.tilt = *tilt
	opts.chrome = *chrome
	opts.peakFall = *fall
	opts.showPeaks = *peaks
	opts.layout = normalizeLayout(*layout)
	opts.splash = *splash

	switch strings.ToLower(*color) {
	case "synthwave", "synth", "cyberpunk", "classic":
	default:
		fmt.Fprintf(os.Stderr, "unknown -color %q, using %s\n", *color, schemeName(opts.scheme))
	}

	switch opts.barStyle {
	case "led", "solid", "braille", "gradient":
	default:
		fmt.Fprintf(os.Stderr, "unknown -style %q, using %s\n", *style, viz.BAR_STYLE)
		opts.barStyle = viz.BAR_STYLE
	}

	if opts.tilt < 0 {
		opts.tilt = 0
	}
	if opts.tilt > viz.SPECTRAL_TILT_MAX_SLOPE {
		opts.tilt = viz.SPECTRAL_TILT_MAX_SLOPE
	}
	if opts.gain < viz.MIN_GAIN {
		opts.gain = viz.MIN_GAIN
	}
	if opts.gain > viz.MAX_GAIN {
		opts.gain = viz.MAX_GAIN
	}
	if opts.bands < 0 {
		opts.bands = 0
	}

	return opts
}

// main entry point
func main() {
	// defaults -> config file -> CLI flags
	base := defaultOptions()
	loadConfigInto(&base)
	m := initialModel(parseFlags(base))

	// Check for initialization errors
	if m.err != nil {
		fmt.Fprintf(os.Stderr, "Initialization failed: %v\n", m.err)
		os.Exit(1)
	}

	// Ensure audio resources are cleaned up
	defer m.capturer.Close()

	// Create Bubbletea program
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support (optional)
	)

	// Run the program
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}

// ARCHITECTURE NOTES:
//
// BUBBLETEA MODEL-UPDATE-VIEW PATTERN:
//
//   1. Model: Application state (audio, FFT, peaks, etc.)
//   2. Update: Handle messages (keyboard, ticks, window resize)
//   3. View: Render current state to string
//
// This is similar to Elm/Redux architecture:
//   - Immutable state
//   - Pure functions
//   - Message-driven updates
//
// EXECUTION FLOW:
//
//   Init() → tickCmd() → audioTickMsg
//                ↓
//   Update(audioTickMsg) → processAudio()
//                ↓
//   Read audio → FFT → Smooth → Update peaks
//                ↓
//   View() → Renderer.Render()
//                ↓
//   Display → tickCmd() → loop
//
// PERFORMANCE CONSIDERATIONS:
//
// Target: 30 FPS = 33ms per frame
//
// Time budget breakdown:
//   - Audio read: <1ms
//   - FFT: ~1-2ms (2048 FFT)
//   - Smoothing: <0.1ms
//   - Peak update: <0.1ms
//   - Rendering: ~1-5ms (depends on terminal)
//   - Total: ~5-10ms per frame
//
// Remaining ~23ms available for:
//   - Terminal I/O
//   - Bubbletea overhead
//   - OS scheduling
//
// This gives comfortable headroom for 30 FPS.
//
// OPTIMIZATION OPPORTUNITIES:
//
// If performance becomes an issue:
//
// 1. Reduce FFT size (2048 → 1024)
// 2. Reduce target FPS (30 → 20)
// 3. Process audio in separate goroutine
// 4. Cache rendered strings
// 5. Reduce number of bands (16 → 8)
//
// DEBUGGING:
//
// Add debug output in processAudio():
//   fmt.Fprintf(os.Stderr, "Frame time: %dms, Bands: %v\n", deltaMs, bands)
//
// Enable debug info in viz/config.go:
//   const SHOW_DEBUG_INFO = true
//
// TROUBLESHOOTING:
//
// Problem: Frozen visualization
//   - Check audio capture is working
//   - Add logging in processAudio()
//   - Check for errors in m.err
//
// Problem: Low FPS
//   - Check CPU usage (should be <5%)
//   - Reduce FFT_SIZE or TARGET_FPS
//   - Profile with pprof
//
// Problem: No bars visible
//   - Increase gain with + key
//   - Check audio is actually playing
//   - Verify A-weighting isn't over-reducing
//   - Add debug logging to see band values
//
// FUTURE ENHANCEMENTS:
//
// 1. Store smoothed bands in model for proper rendering
//    (Current implementation renders peaks but not bars)
//
// 2. Add FPS counter in debug mode
//
// 3. Add configuration file support:
//    - Load/save color schemes
//    - Remember gain setting
//    - Custom key bindings
//
// 4. Add beat detection:
//    - Detect bass kicks
//    - Pulse effects on beat
//
// 5. Add recording mode:
//    - Save frames to images
//    - Generate video with ffmpeg
//
// See docs/IMPLEMENTATION_PLAN.md for complete list
