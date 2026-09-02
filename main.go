package main

import (
	"cyberspec/audio"
	"cyberspec/dsp"
	"cyberspec/viz"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

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
	currentScheme  viz.ColorScheme
	gain           float64
	lastUpdate     time.Time
	currentBands   []float64  // Current smoothed band magnitudes

	// Terminal dimensions
	width  int
	height int

	// Error state
	err error
}

// audioTickMsg is sent every frame to trigger audio processing
type audioTickMsg struct{}

// Initialize sets up the application
func initialModel() model {
	// Create audio capturer
	capturer, err := audio.NewCapturer()
	if err != nil {
		return model{err: fmt.Errorf("audio initialization failed: %w", err)}
	}

	// Create FFT processor
	fft := dsp.NewFFTProcessor(viz.SAMPLE_RATE)

	// Create smoother
	smoother := viz.NewSmoother(viz.NUM_BANDS)

	// Create peak tracker
	peaks := viz.NewPeakTracker(viz.NUM_BANDS)

	// Create renderer (will be updated with actual terminal size)
	renderer := viz.NewRenderer(80, 24, viz.SchemeClassic)

	return model{
		capturer:      capturer,
		fft:           fft,
		smoother:      smoother,
		renderer:      renderer,
		peaks:         peaks,
		currentScheme: viz.SchemeClassic,
		gain:          viz.DEFAULT_GAIN,
		lastUpdate:    time.Now(),
		currentBands:  make([]float64, viz.NUM_BANDS),
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
		// Reset gain to default
		m.gain = viz.DEFAULT_GAIN
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

	// Apply smoothing
	smoothedBands := m.smoother.Smooth(bands)

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

// main entry point
func main() {
	// Initialize model
	m := initialModel()

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
