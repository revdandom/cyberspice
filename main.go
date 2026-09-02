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

// options holds command-line configuration.
type options struct {
	barStyle string
	scheme   viz.ColorScheme
	bands      int // 0 = auto-size to terminal
	gain       float64
	tilt       float64 // spectral tilt, dB/octave
	perceptual bool    // amplitude scale: true = Stevens, false = linear
}

// model represents the application state for Bubbletea
type model struct {
	// Audio processing
	capturer *audio.Capturer
	fft      *dsp.FFTProcessor
	smoother *viz.Smoother

	// Visualization
	renderer *viz.Renderer
	peaks    *viz.PeakTracker

	// State
	currentScheme viz.ColorScheme
	gain          float64
	defaultGain   float64 // gain restored by the "0" key
	tiltDB        float64 // spectral tilt, dB/octave (adjusted with [ and ])
	lastUpdate    time.Time
	currentBands  []float64 // Current smoothed band magnitudes

	// Band count
	numBands  int
	autoBands bool // resize band count with the terminal

	// Terminal dimensions
	width  int
	height int

	// Error state
	err error
}

// audioTickMsg is sent every frame to trigger audio processing
type audioTickMsg struct{}

// computeBands derives the band count from the terminal width, clamped to
// [MIN_BANDS, MAX_BANDS]. Each band occupies BAND_COLUMNS columns and the
// last one has no trailing gap, hence the +1.
func computeBands(width int) int {
	if width <= 0 {
		width = 80
	}
	n := (width + 1) / viz.BAND_COLUMNS
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
	m.smoother = viz.NewSmoother(n)
	m.peaks = viz.NewPeakTracker(n)
	m.currentBands = make([]float64, n)
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
		nbands = computeBands(80)
	}

	// Create FFT processor
	fft := dsp.NewFFTProcessor(viz.SAMPLE_RATE)
	fft.SetNumBands(nbands)
	fft.SetTilt(opts.tilt)

	// Create renderer (will be updated with actual terminal size)
	renderer := viz.NewRenderer(80, 24, opts.scheme)
	renderer.SetBarStyle(opts.barStyle)
	renderer.SetTiltDisplay(opts.tilt)
	renderer.SetPerceptualAmp(opts.perceptual)

	return model{
		capturer:      capturer,
		fft:           fft,
		smoother:      viz.NewSmoother(nbands),
		renderer:      renderer,
		peaks:         viz.NewPeakTracker(nbands),
		currentScheme: opts.scheme,
		gain:          opts.gain,
		defaultGain:   opts.gain,
		tiltDB:        opts.tilt,
		lastUpdate:    time.Now(),
		currentBands:  make([]float64, nbands),
		numBands:      nbands,
		autoBands:     autoBands,
		width:         80,
		height:        24,
	}
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
		if m.autoBands {
			m.resize(computeBands(msg.Width))
		}
		return m, nil

	case audioTickMsg:
		// Process audio and update visualization
		if err := m.processAudio(); err != nil {
			m.err = err
			return m, tea.Quit
		}

		// Schedule next tick
		return m, tickCmd()

	case tea.KeyMsg:
		// Handle keyboard input
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

	case "a":
		// Toggle amplitude scale (Stevens' loudness curve <-> linear)
		m.renderer.ToggleAmplitude()

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

// processAudio reads audio and updates visualization state
func (m *model) processAudio() error {
	// Calculate delta time since last update
	now := time.Now()
	deltaMs := now.Sub(m.lastUpdate).Milliseconds()
	m.lastUpdate = now

	// Read audio samples
	samples, err := m.capturer.ReadSamples()
	if err != nil {
		return fmt.Errorf("audio read failed: %w", err)
	}

	// Process through FFT to get frequency bands
	bands := m.fft.Process(samples)

	// Temporal smoothing (attack/release), then monstercat spatial spread so
	// the spectrum reads as a smooth envelope instead of isolated spikes.
	smoothedBands := m.smoother.Smooth(bands)
	smoothedBands = viz.SpreadNeighbors(smoothedBands, viz.MONSTERCAT_FACTOR)

	// Store current bands for rendering
	m.currentBands = smoothedBands

	// Update peak tracking
	m.peaks.Update(smoothedBands, deltaMs)

	return nil
}

// View renders the current state
func (m model) View() string {
	// Check for errors
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.\n", m.err)
	}

	// Get scheme name for display
	schemeName := "Classic"
	if m.currentScheme == viz.SchemeSynthwave {
		schemeName = "Synthwave"
	}

	// Render spectrum with current band magnitudes
	return m.renderer.Render(m.currentBands, m.peaks, m.gain, schemeName)
}

// parseFlags reads command-line options.
//
//	-style  led | solid | braille | fibonacci   (bar rendering)
//	-color  classic | synthwave                  (color scheme)
//	-bands  N                                    (0 = auto-size to terminal)
//	-gain   X                                    (initial gain multiplier)
// schemeFromName maps a scheme name to its ColorScheme (unknown -> classic).
func schemeFromName(name string) viz.ColorScheme {
	if strings.ToLower(name) == "synthwave" {
		return viz.SchemeSynthwave
	}
	return viz.SchemeClassic
}

// ampDefaultName is the -amp default that matches AMPLITUDE_PERCEPTUAL_DEFAULT.
func ampDefaultName() string {
	if viz.AMPLITUDE_PERCEPTUAL_DEFAULT {
		return "stevens"
	}
	return "linear"
}

func parseFlags() options {
	style := flag.String("style", viz.BAR_STYLE, "bar style: led, solid, braille, fibonacci")
	color := flag.String("color", viz.DEFAULT_COLOR_SCHEME, "color scheme: classic, synthwave")
	bands := flag.Int("bands", 0, "number of frequency bands (0 = auto-size to terminal width)")
	gain := flag.Float64("gain", viz.DEFAULT_GAIN, "initial gain multiplier")
	tilt := flag.Float64("tilt", viz.SPECTRAL_TILT_DB_PER_OCT, "spectral tilt: dB/octave high-frequency lift (0 = flat)")
	amp := flag.String("amp", ampDefaultName(), "amplitude scale: stevens (perceptual loudness) or linear")
	flag.Parse()

	opts := options{
		barStyle:   strings.ToLower(*style),
		scheme:     schemeFromName(viz.DEFAULT_COLOR_SCHEME),
		bands:      *bands,
		gain:       *gain,
		tilt:       *tilt,
		perceptual: viz.AMPLITUDE_PERCEPTUAL_DEFAULT,
	}

	switch strings.ToLower(*amp) {
	case "stevens", "perceptual", "loudness", "log":
		opts.perceptual = true
	case "linear", "raw", "amplitude":
		opts.perceptual = false
	default:
		fmt.Fprintf(os.Stderr, "unknown -amp %q, using %s\n", *amp, ampDefaultName())
	}

	if opts.tilt < 0 {
		opts.tilt = 0
	}
	if opts.tilt > viz.SPECTRAL_TILT_MAX_SLOPE {
		opts.tilt = viz.SPECTRAL_TILT_MAX_SLOPE
	}

	switch strings.ToLower(*color) {
	case "synthwave", "synth", "cyberpunk":
		opts.scheme = viz.SchemeSynthwave
	case "classic":
		opts.scheme = viz.SchemeClassic
	default:
		fmt.Fprintf(os.Stderr, "unknown -color %q, using %s\n", *color, viz.DEFAULT_COLOR_SCHEME)
	}

	switch opts.barStyle {
	case "led", "solid", "braille", "fibonacci":
	default:
		fmt.Fprintf(os.Stderr, "unknown -style %q, using %s\n", *style, viz.BAR_STYLE)
		opts.barStyle = viz.BAR_STYLE
	}

	if opts.gain < viz.MIN_GAIN {
		opts.gain = viz.MIN_GAIN
	}
	if opts.gain > viz.MAX_GAIN {
		opts.gain = viz.MAX_GAIN
	}

	return opts
}

// main entry point
func main() {
	// Initialize model
	m := initialModel(parseFlags())

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
