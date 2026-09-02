package viz

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Renderer handles visualization rendering
type Renderer struct {
	termWidth  int
	termHeight int
	scheme     ColorScheme
	barStyle   string  // "led" | "solid" | "braille" | "fibonacci"
	tiltDB     float64 // spectral tilt, for the header readout only
	ampMode    string  // "linear" | "stevens" | "db"
	chrome     bool    // show the header/footer bars
}

// SetTiltDisplay records the current spectral tilt so the header can show it.
// The actual tilt lives in the FFT processor.
func (r *Renderer) SetTiltDisplay(dbPerOctave float64) { r.tiltDB = dbPerOctave }

// ampModeOrder is the cycle order for the "a" key.
var ampModeOrder = []string{"linear", "stevens", "db"}

// SetAmplitudeMode selects the amplitude scale. Unknown values fall back to
// the configured default.
func (r *Renderer) SetAmplitudeMode(mode string) {
	switch mode {
	case "linear", "stevens", "db":
		r.ampMode = mode
	default:
		r.ampMode = AMPLITUDE_MODE_DEFAULT
	}
}

// CycleAmplitude advances to the next amplitude scale (wraps around).
func (r *Renderer) CycleAmplitude() {
	for i, m := range ampModeOrder {
		if m == r.ampMode {
			r.ampMode = ampModeOrder[(i+1)%len(ampModeOrder)]
			return
		}
	}
	r.ampMode = ampModeOrder[0]
}

// AmplitudeMode returns the active amplitude scale name.
func (r *Renderer) AmplitudeMode() string { return r.ampMode }

// ampValue maps a 0-1 band/peak value to its display height fraction.
//   linear  - passthrough
//   stevens - value^AMPLITUDE_EXPONENT (loudness power law)
//   db      - linear in dB over [AMPLITUDE_DB_FLOOR, 0] dB
func (r *Renderer) ampValue(v float64) float64 {
	if v <= 0 {
		return 0
	}
	switch r.ampMode {
	case "linear":
		return v
	case "db":
		d := 20 * math.Log10(v) // <= 0 since v <= 1
		if d <= AMPLITUDE_DB_FLOOR {
			return 0
		}
		n := (d - AMPLITUDE_DB_FLOOR) / -AMPLITUDE_DB_FLOOR
		if n > 1 {
			n = 1
		}
		return n
	default: // "stevens"
		return math.Pow(v, AMPLITUDE_EXPONENT)
	}
}

// SetChrome shows or hides the header/footer bars.
func (r *Renderer) SetChrome(on bool) { r.chrome = on }

// ToggleChrome flips the header/footer bars on/off. Bound to any key that has
// no other action, so a curious keypress reveals the controls.
func (r *Renderer) ToggleChrome() { r.chrome = !r.chrome }

// Chrome reports whether the header/footer bars are shown.
func (r *Renderer) Chrome() bool { return r.chrome }

// NewRenderer creates a new renderer
func NewRenderer(termWidth, termHeight int, scheme ColorScheme) *Renderer {
	return &Renderer{
		termWidth:  termWidth,
		termHeight: termHeight,
		scheme:     scheme,
		barStyle:   BAR_STYLE,
		ampMode:    AMPLITUDE_MODE_DEFAULT,
		chrome:     SHOW_CHROME_DEFAULT,
	}
}

// barStyleOrder is the cycle order for the "s" key.
var barStyleOrder = []string{"led", "solid", "braille", "fibonacci"}

// A bar cell (BAR_WIDTH full blocks) and an equally wide blank, so every
// style and the transpose step agree on column width.
var (
	barCell  = strings.Repeat("█", BAR_WIDTH)
	barBlank = strings.Repeat(" ", BAR_WIDTH)
)

// SetBarStyle changes the bar rendering style. Unknown values fall back to
// the configured default.
func (r *Renderer) SetBarStyle(style string) {
	switch style {
	case "led", "solid", "braille", "fibonacci":
		r.barStyle = style
	default:
		r.barStyle = BAR_STYLE
	}
}

// CycleBarStyle advances to the next bar style (wraps around).
func (r *Renderer) CycleBarStyle() {
	for i, s := range barStyleOrder {
		if s == r.barStyle {
			r.barStyle = barStyleOrder[(i+1)%len(barStyleOrder)]
			return
		}
	}
	r.barStyle = barStyleOrder[0]
}

// BarStyle returns the active bar style name.
func (r *Renderer) BarStyle() string { return r.barStyle }

// SetTerminalSize updates terminal dimensions
func (r *Renderer) SetTerminalSize(width, height int) {
	r.termWidth = width
	r.termHeight = height
}

// SetColorScheme changes the active color scheme
func (r *Renderer) SetColorScheme(scheme ColorScheme) {
	r.scheme = scheme
}

// Render creates the visual representation of the spectrum
//
// RENDERING PIPELINE:
//   1. Calculate available height (term height - header/footer)
//   2. For each frequency band:
//      a. Calculate bar height based on magnitude
//      b. Apply Fibonacci decay pattern
//      c. Render bar with appropriate colors
//      d. Render peak indicator with flicker
//   3. Combine all bands horizontally
//   4. Add header/footer
//   5. Return complete frame as string
//
// Parameters:
//   magnitudes  - Current band magnitudes (0.0-1.0)
//   peaks       - Peak tracker with flicker data
//   gain        - User-adjustable gain multiplier
//   schemeName  - Current scheme name for display
//
// Returns:
//   string - Complete frame ready for terminal output
func (r *Renderer) Render(magnitudes []float64, peaks *PeakTracker, gain float64, schemeName string) string {
	// Chrome hidden: spectrum fills the whole screen (minus one line of slack
	// so the frame never scrolls the alt-screen).
	if !r.chrome {
		usableHeight := r.termHeight - 1
		if usableHeight < 10 {
			return "Terminal too small - need at least 13 lines"
		}
		return r.buildSpectrum(magnitudes, peaks, usableHeight, gain)
	}

	// Calculate usable height (leave room for header and footer)
	headerHeight := 2
	footerHeight := 1
	usableHeight := r.termHeight - headerHeight - footerHeight

	if usableHeight < 10 {
		return "Terminal too small - need at least 13 lines"
	}

	// Build header
	header := r.buildHeader(gain, schemeName)

	// Build spectrum visualization
	spectrum := r.buildSpectrum(magnitudes, peaks, usableHeight, gain)

	// Build footer (optional - could show frequency labels)
	footer := r.buildFooter()

	// Combine all parts
	return header + "\n" + spectrum + "\n" + footer
}

// buildHeader creates the header display
func (r *Renderer) buildHeader(gain float64, schemeName string) string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00FFFF")).
		Render("CYBERSPEC")

	info := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Render(fmt.Sprintf("Gain: %.1fx  |  Scheme: %s  |  Style: %s  |  Tilt: %.1fdB/oct  |  Amp: %s", gain, schemeName, r.barStyle, r.tiltDB, r.AmplitudeMode()))

	// Add debug info if enabled
	if ENABLE_DEBUG_OUTPUT {
		debugInfo := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF8800")).
			Render(fmt.Sprintf("  [DEBUG MODE]"))
		return title + "  " + info + debugInfo
	}

	return title + "  " + info
}

// buildFooter creates the footer display
func (r *Renderer) buildFooter() string {
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Render("c: color  s: style  a: amp  [ ]: tilt  +/-: gain  w: save  q: quit")

	return help
}

// buildSpectrum creates the main spectrum visualization
//
// ALGORITHM:
//   1. For each frequency band, render vertical bar with decay
//   2. Transpose bars (vertical bands → horizontal lines)
//   3. Combine lines into final output
//
// This approach makes it easy to render each band independently
// then combine them horizontally.
func (r *Renderer) buildSpectrum(magnitudes []float64, peaks *PeakTracker, height int, gain float64) string {
	// Render each band as vertical column
	numBands := len(magnitudes)
	columns := make([][]string, numBands)

	// Track debug info
	var debugInfo strings.Builder
	if ENABLE_DEBUG_OUTPUT {
		debugInfo.WriteString("Magnitudes: [")
	}

	for i := 0; i < numBands; i++ {
		// Apply gain
		magnitude := magnitudes[i] * gain
		if magnitude > 1.0 {
			magnitude = 1.0
		}

		// Get peak info
		peakHeight := peaks.GetPeakHeight(i) * gain
		if peakHeight > 1.0 {
			peakHeight = 1.0
		}
		peakOpacity := peaks.GetPeakOpacity(i)

		// Render this band's column
		columns[i] = r.renderBand(magnitude, peakHeight, peakOpacity, height)

		// Collect debug info
		if ENABLE_DEBUG_OUTPUT {
			barHeight := int(magnitude * float64(height))
			debugInfo.WriteString(fmt.Sprintf("%d ", barHeight))
		}
	}

	if ENABLE_DEBUG_OUTPUT {
		debugInfo.WriteString("]")
	}

	// Transpose columns to rows
	lines := r.transposeColumns(columns, height)

	// Add debug info at bottom if enabled
	if ENABLE_DEBUG_OUTPUT {
		result := strings.Join(lines, "\n")
		debugStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
		return result + "\n" + debugStyle.Render(debugInfo.String())
	}

	// Join lines
	return strings.Join(lines, "\n")
}

// renderBand renders a single frequency band as a vertical column
//
// FIBONACCI DECAY IMPLEMENTATION:
//
// See docs/ALGORITHMS.md and docs/decay-inspiration.png for details.
//
// As energy decreases, bars fragment into segments with gaps that grow
// following the Fibonacci sequence (0, 1, 2, 3, 5, 8, 13...).
//
// ALGORITHM:
//   1. Calculate bar height from magnitude
//   2. Determine energy level (for decay pattern selection)
//   3. If high energy (>90%): Render solid bar
//   4. If lower energy: Render with Fibonacci gaps
//      a. Look up gap size from DECAY_THRESHOLDS
//      b. Look up segment height from SEGMENT_HEIGHTS
//      c. Render alternating segments and gaps
//   5. Add peak indicator if present
//
// Parameters:
//   magnitude    - Band magnitude (0.0-1.0, gain-adjusted)
//   peakHeight   - Peak height (0.0-1.0, gain-adjusted)
//   peakOpacity  - Peak opacity from flicker (0.0-1.0)
//   height       - Available height in terminal lines
//
// Returns:
//   []string - Array of strings, one per line (bottom to top)
func (r *Renderer) renderBand(magnitude, peakHeight, peakOpacity float64, height int) []string {
	// Map amplitude to display height (linear / Stevens / dB, see ampValue).
	// Everything below works in this display domain so color, gaps and the
	// peak marker all agree with the bar height.
	dispMag := r.ampValue(magnitude)
	dispPeak := r.ampValue(peakHeight)

	// Calculate bar height in terminal lines
	barHeight := int(dispMag * float64(height))
	if barHeight < 0 {
		barHeight = 0
	}
	if barHeight > height {
		barHeight = height
	}

	// Calculate peak position
	peakPos := int(dispPeak * float64(height))
	if peakPos < 0 {
		peakPos = 0
	}
	if peakPos > height {
		peakPos = height
	}

	// Create column (bottom to top), all blank at bar width.
	column := make([]string, height)
	for i := range column {
		column[i] = barBlank
	}

	switch r.barStyle {
	case "led":
		r.renderLEDBar(column, dispMag, height)
	case "braille":
		r.renderBrailleBar(column, dispMag, height)
	case "fibonacci":
		energyPercent := int(dispMag * 100)
		if energyPercent < 90 {
			r.renderFibonacciDecay(column, barHeight, dispMag, energyPercent)
		} else {
			r.renderSolidBar(column, barHeight, dispMag)
		}
	default: // "solid"
		r.renderSolidBar(column, barHeight, dispMag)
	}

	// Add peak indicator (thin PEAK_CHAR line for every style)
	if peakPos > 0 && peakOpacity > 0.01 {
		r.renderPeak(column, peakPos, peakOpacity)
	}

	return column
}

// renderFibonacciDecay renders a bar with Fibonacci gap pattern
//
// DECAY PATTERN (see docs/decay-inspiration.png):
//
//   High energy (90-100%): Solid bar, gap = 0
//   Medium-high (70-90%):  Small gaps, gap = 1 line
//   Medium (50-70%):       Medium gaps, gap = 2 lines
//   Medium-low (30-50%):   Larger gaps, gap = 3 lines
//   Low (15-30%):          Big gaps, gap = 5 lines (Fibonacci!)
//   Very low (5-15%):      Huge gaps, gap = 8 lines (Fibonacci!)
//   Minimal (0-5%):        Barely visible, gap = 13 lines (Fibonacci!)
//
// ALGORITHM:
//   1. Look up gap size for this energy level
//   2. Look up segment height for this energy level
//   3. Fill column from bottom, alternating:
//      - Draw 'segmentHeight' lines of bars
//      - Skip 'gapSize' lines (empty)
//      - Repeat until we reach barHeight
//
// Parameters:
//   column        - Column to fill (modified in place)
//   barHeight     - Total height to fill
//   magnitude     - Magnitude for color selection
//   energyPercent - Energy percentage for threshold lookup
func (r *Renderer) renderFibonacciDecay(column []string, barHeight int, magnitude float64, energyPercent int) {
	// Look up gap size and segment height for this energy level
	gapSize := r.getGapForEnergy(energyPercent)
	segmentHeight := r.getSegmentHeightForEnergy(energyPercent)

	// Select character based on energy level
	char := r.getCharForEnergy(magnitude)

	// Get color for this magnitude
	color := GetColorForHeight(r.scheme, magnitude)
	style := lipgloss.NewStyle().Foreground(color)

	// Fill column with segmented pattern
	position := 0
	for position < barHeight {
		// Draw segment
		for i := 0; i < segmentHeight && position < barHeight; i++ {
			column[position] = style.Render(char)
			position++
		}

		// Add gap
		for i := 0; i < gapSize && position < barHeight; i++ {
			column[position] = barBlank
			position++
		}
	}
}

// renderSolidBar renders a solid bar without fragmentation
func (r *Renderer) renderSolidBar(column []string, barHeight int, magnitude float64) {
	// Get color for this magnitude
	color := GetColorForHeight(r.scheme, magnitude)
	style := lipgloss.NewStyle().Foreground(color)

	// Fill from bottom to barHeight
	for i := 0; i < barHeight; i++ {
		column[i] = style.Render(barCell)
	}
}

// renderLEDBar draws the LED style: one amplitude level per cell row, each a
// LED_LINE_GLYPH (a lower-partial block) — a short lit segment sitting on a
// dark upper portion (the gap) in the same cell, so blocks are half a row
// tall at a 1:1 lit:gap ratio. Coloured by absolute position via ledRamp.
// The peak marker is drawn separately by renderPeak.
func (r *Renderer) renderLEDBar(column []string, dispMag float64, height int) {
	if height < 1 {
		return
	}

	lit := int(dispMag*float64(height) + 0.5)
	if lit > height {
		lit = height
	}

	cell := strings.Repeat(LED_LINE_GLYPH, BAR_WIDTH)
	for b := 0; b < lit; b++ {
		pos := (float64(b) + 0.5) / float64(height)
		style := lipgloss.NewStyle().Foreground(GetColorForHeight(r.scheme, pos))
		if LED_GAP_COLOR != "" {
			style = style.Background(lipgloss.Color(LED_GAP_COLOR))
		}
		column[b] = style.Render(cell)
	}
}

// renderBrailleBar draws the bar with vertical braille fill. Each terminal
// row is 4 braille dot-rows tall, giving the bar 4× the height resolution of
// the block styles plus a fine dotted texture. Full cells use the solid
// braille block; the topmost partial cell fills from its bottom. Colored by
// vertical position. Requires a font with U+28xx (Braille Patterns) glyphs.
func (r *Renderer) renderBrailleBar(column []string, magnitude float64, height int) {
	// partial[n] = n dot-rows filled from the bottom of a cell (n = 1..3).
	partial := [4]string{"", "⣀", "⣤", "⣶"}

	subRows := int(magnitude * float64(height) * 4.0)
	if subRows < 0 {
		subRows = 0
	}
	fullCells := subRows / 4
	rem := subRows % 4

	denom := float64(height - 1)
	if denom < 1 {
		denom = 1
	}

	for i := 0; i < fullCells && i < height; i++ {
		color := GetColorForHeight(r.scheme, float64(i)/denom)
		column[i] = lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("⣿", BAR_WIDTH))
	}
	if rem > 0 && fullCells < height {
		color := GetColorForHeight(r.scheme, float64(fullCells)/denom)
		column[fullCells] = lipgloss.NewStyle().Foreground(color).Render(strings.Repeat(partial[rem], BAR_WIDTH))
	}
}

// renderPeak renders the peak indicator with flicker
func (r *Renderer) renderPeak(column []string, peakPos int, opacity float64) {
	if peakPos >= len(column) {
		peakPos = len(column) - 1
	}

	// Get peak color
	baseColor := GetPeakColor(r.scheme)

	// Apply opacity to color (simplified - just adjust brightness)
	// In a full implementation, you'd parse the color and adjust RGB
	// For now, we'll use the base color with visual indication
	style := lipgloss.NewStyle().Foreground(baseColor)

	if opacity < 0.3 {
		// Very dim - use dimmed style
		style = style.Faint(true)
	}

	// Render peak marker (braille mode uses a dotted line to match the bar
	// texture; every other style uses the heavy horizontal PEAK_CHAR).
	glyph := strings.Repeat(PEAK_CHAR, BAR_WIDTH)
	if r.barStyle == "braille" {
		glyph = strings.Repeat("⠉", BAR_WIDTH)
	}
	column[peakPos] = style.Render(glyph)
}

// getGapForEnergy returns gap size for a given energy percentage
//
// Uses DECAY_THRESHOLDS map with fallback logic
func (r *Renderer) getGapForEnergy(energyPercent int) int {
	// Get sorted threshold keys
	thresholds := make([]int, 0, len(DECAY_THRESHOLDS))
	for k := range DECAY_THRESHOLDS {
		thresholds = append(thresholds, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(thresholds)))

	// Find matching threshold
	for _, threshold := range thresholds {
		if energyPercent >= threshold {
			return DECAY_THRESHOLDS[threshold]
		}
	}

	// Default to maximum gap
	return 13
}

// getSegmentHeightForEnergy returns segment height for energy percentage
func (r *Renderer) getSegmentHeightForEnergy(energyPercent int) int {
	// Get sorted threshold keys
	thresholds := make([]int, 0, len(SEGMENT_HEIGHTS))
	for k := range SEGMENT_HEIGHTS {
		thresholds = append(thresholds, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(thresholds)))

	// Find matching threshold
	for _, threshold := range thresholds {
		if energyPercent >= threshold {
			return SEGMENT_HEIGHTS[threshold]
		}
	}

	// Default to single line
	return 1
}

// getCharForEnergy selects character based on energy level
//
// CHARACTER SELECTION:
//   High energy (>70%):  Full blocks █
//   Medium (40-70%):     Full blocks (could use half blocks)
//   Low (20-40%):        Half blocks ▄
//   Very low (<20%):     Thin dashes ─
//
// This creates additional visual interest as bars decay
func (r *Renderer) getCharForEnergy(magnitude float64) string {
	switch {
	case magnitude > 0.4:
		return barCell // Full blocks
	case magnitude > 0.2:
		return strings.Repeat("▄", BAR_WIDTH) // Half blocks
	default:
		return strings.Repeat("─", BAR_WIDTH) // Thin dashes
	}
}

// transposeColumns converts vertical columns to horizontal lines
//
// SIMPLIFIED VERSION FOR DEBUGGING
//
// Column coordinate system:
//   column[0]        = bottom of bar
//   column[height-1] = top of bar
//
// Screen coordinate system:
//   lines[0]         = top of screen
//   lines[height-1]  = bottom of screen
//
// Parameters:
//   columns - Array of columns (each column[0] = bottom, column[height-1] = top)
//   height  - Height in lines
//
// Returns:
//   []string - Array of horizontal lines (lines[0] = top of screen)
func (r *Renderer) transposeColumns(columns [][]string, height int) []string {
	lines := make([]string, height)

	// For each screen line (from top to bottom)
	for screenY := 0; screenY < height; screenY++ {
		var line strings.Builder

		// Map screen coordinate to column coordinate
		// Screen line 0 (top) maps to column index (height-1)
		// Screen line (height-1) (bottom) maps to column index 0
		columnY := height - 1 - screenY

		// For each frequency band (left to right)
		for bandIdx := 0; bandIdx < len(columns); bandIdx++ {
			// Get the character at this position
			if columnY >= 0 && columnY < len(columns[bandIdx]) {
				line.WriteString(columns[bandIdx][columnY])
			} else {
				line.WriteString(barBlank)
			}

			// Add spacing between bands (except after last band)
			if bandIdx < len(columns)-1 {
				line.WriteString(" ")
			}
		}

		lines[screenY] = line.String()
	}

	return lines
}

// HOW TO EXPERIMENT WITH DECAY PATTERNS:
//
// See viz/config.go for all configurable constants.
//
// QUICK TWEAKS:
//
// 1. Make decay more aggressive (fragment earlier):
//    var DECAY_THRESHOLDS = map[int]int{
//        95: 0, 80: 1, 60: 2, 40: 3, 20: 5, 10: 8,
//    }
//
// 2. Make decay gentler (stay solid longer):
//    var DECAY_THRESHOLDS = map[int]int{
//        95: 0, 85: 1, 75: 2, 50: 3, 25: 5, 10: 8,
//    }
//
// 3. Use different sequence (powers of 2):
//    var DECAY_THRESHOLDS = map[int]int{
//        90: 0, 70: 1, 50: 2, 30: 4, 15: 8, 5: 16,
//    }
//
// 4. Taller segments:
//    var SEGMENT_HEIGHTS = map[int]int{
//        90: 999, 70: 5, 50: 4, 30: 3, 15: 2, 5: 1,
//    }
//
// ALTERNATIVE DECAY PATTERNS (see docs/ALGORITHMS.md):
//   - Fixed segments with growing gaps
//   - Reducing number of segments (Fibonacci count)
//   - Traditional solid decay
//
// To implement alternatives, modify renderFibonacciDecay() or add new
// rendering functions and switch based on DECAY_STYLE constant.
