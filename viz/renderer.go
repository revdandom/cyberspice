package viz

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Renderer handles visualization rendering with Fibonacci decay
type Renderer struct {
	termWidth  int
	termHeight int
	scheme     ColorScheme
}

// NewRenderer creates a new renderer
func NewRenderer(termWidth, termHeight int, scheme ColorScheme) *Renderer {
	return &Renderer{
		termWidth:  termWidth,
		termHeight: termHeight,
		scheme:     scheme,
	}
}

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

	gainStr := fmt.Sprintf("Gain: %.1fx", gain)
	schemeStr := fmt.Sprintf("Scheme: %s", schemeName)

	info := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Render(fmt.Sprintf("%s  |  %s", gainStr, schemeStr))

	return title + "  " + info
}

// buildFooter creates the footer display
func (r *Renderer) buildFooter() string {
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Render("c: color  +/-: gain  q: quit")

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
	columns := make([][]string, NUM_BANDS)

	for i := 0; i < NUM_BANDS; i++ {
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
	}

	// Transpose columns to rows
	lines := r.transposeColumns(columns, height)

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
	// Calculate bar height in terminal lines
	barHeight := int(magnitude * float64(height))
	if barHeight < 0 {
		barHeight = 0
	}
	if barHeight > height {
		barHeight = height
	}

	// Calculate peak position
	peakPos := int(peakHeight * float64(height))
	if peakPos < 0 {
		peakPos = 0
	}
	if peakPos > height {
		peakPos = height
	}

	// Create column (bottom to top)
	column := make([]string, height)

	// Determine if we should use decay pattern
	energyPercent := int(magnitude * 100)

	if DECAY_STYLE == "fibonacci_gaps" && energyPercent < 90 {
		// Use Fibonacci decay pattern
		r.renderFibonacciDecay(column, barHeight, magnitude, energyPercent)
	} else {
		// Render solid bar (no fragmentation)
		r.renderSolidBar(column, barHeight, magnitude)
	}

	// Add peak indicator
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
			column[position] = "  " // Empty space (2 chars for alignment)
			position++
		}
	}
}

// renderSolidBar renders a solid bar without fragmentation
func (r *Renderer) renderSolidBar(column []string, barHeight int, magnitude float64) {
	// Get color for this magnitude
	color := GetColorForHeight(r.scheme, magnitude)
	style := lipgloss.NewStyle().Foreground(color)

	// Use full block character
	char := "██"

	// Fill from bottom to barHeight
	for i := 0; i < barHeight; i++ {
		column[i] = style.Render(char)
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

	// Render peak character
	column[peakPos] = style.Render(PEAK_CHAR + PEAK_CHAR) // Double width
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
	case magnitude > 0.7:
		return "██" // Full blocks
	case magnitude > 0.4:
		return "██" // Full blocks (could differentiate)
	case magnitude > 0.2:
		return "▄▄" // Half blocks
	default:
		return "──" // Thin dashes
	}
}

// transposeColumns converts vertical columns to horizontal lines
//
// INPUT:  Array of columns (each column is bottom-to-top)
// OUTPUT: Array of lines (each line is left-to-right)
//
// EXAMPLE:
//   Columns (3 bands, 4 lines high):
//     [0]: ["█", "█", " ", " "]
//     [1]: ["█", " ", " ", " "]
//     [2]: ["█", "█", "█", " "]
//
//   Lines (top to bottom, left to right):
//     [3]: "      "  (top)
//     [2]: "  ██"
//     [1]: "██  ██"
//     [0]: "████████"  (bottom)
//
// Parameters:
//   columns - Array of columns (vertical)
//   height  - Height in lines
//
// Returns:
//   []string - Array of horizontal lines
func (r *Renderer) transposeColumns(columns [][]string, height int) []string {
	lines := make([]string, height)

	// For each line (top to bottom)
	for y := height - 1; y >= 0; y-- {
		var line strings.Builder

		// For each column (left to right)
		for x := 0; x < len(columns); x++ {
			if y < len(columns[x]) {
				line.WriteString(columns[x][y])
			} else {
				line.WriteString("  ") // Empty space
			}

			// Add spacing between bands
			if x < len(columns)-1 {
				line.WriteString(" ") // BAR_SPACING is already in char width
			}
		}

		lines[height-1-y] = line.String()
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
