package viz

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/lipgloss"
)

// ColorScheme represents a color scheme for the spectrum analyzer
type ColorScheme int

const (
	SchemeClassic   ColorScheme = 0
	SchemeSynthwave ColorScheme = 1
)

// GetColorForHeight returns the appropriate color for a given height percentage
//
// COLOR THEORY FOR SPECTRUM ANALYZERS:
//
// Traditional (Classic scheme):
//   - Green (low): Safe, calm, low levels
//   - Yellow (mid): Caution, moderate levels
//   - Red (high): Alert, high levels, approaching clipping
//   - Matches VU meters, mixing consoles, audio equipment
//
// Synthwave (Cyberpunk scheme):
//   - Cyan (low): Cool, digital, electric
//   - Blue (mid): Transition
//   - Magenta (high): Hot, intense, neon
//   - Matches 80s aesthetics, cyberpunk themes, neon signs
//
// Both schemes ramp the same way (see ledRamp): base color for the bottom
// third, base→mid across the middle third, mid→top across the top third.
//
// Parameters:
//   scheme         - Which color scheme to use
//   heightPercent  - Height as percentage (0.0 to 1.0)
//
// Returns:
//   lipgloss.Color - Color to use for this height
func GetColorForHeight(scheme ColorScheme, heightPercent float64) lipgloss.Color {
	// Clamp to valid range
	if heightPercent < 0.0 {
		heightPercent = 0.0
	}
	if heightPercent > 1.0 {
		heightPercent = 1.0
	}

	switch scheme {
	case SchemeClassic:
		return getClassicColor(heightPercent)
	case SchemeSynthwave:
		return getSynthwaveColor(heightPercent)
	default:
		return getClassicColor(heightPercent)
	}
}

// ledRamp models an RGB LED being driven harder as the bar rises. It holds
// `base` for the bottom third, interpolates base→mid across the middle
// third, and mid→top across the top third. Both schemes share these thirds,
// so the height at which colors transition is the same for every scheme.
func ledRamp(t float64, base, mid, top string) lipgloss.Color {
	switch {
	case t < 1.0/3.0:
		return lipgloss.Color(base)
	case t < 2.0/3.0:
		return interpolateColor(base, mid, (t-1.0/3.0)*3.0)
	default:
		return interpolateColor(mid, top, (t-2.0/3.0)*3.0)
	}
}

// getClassicColor: green, then ADD red to reach yellow at 2/3, then DROP
// green to reach pure red at the top. Peak color is the top of the ramp.
func getClassicColor(heightPercent float64) lipgloss.Color {
	return ledRamp(heightPercent, "#00FF00", "#FFFF00", "#FF0000")
}

// getSynthwaveColor: cyan, then REMOVE green to reach blue at 2/3, then ADD
// red to reach magenta at the top. Same transition heights as Classic.
func getSynthwaveColor(heightPercent float64) lipgloss.Color {
	return ledRamp(heightPercent, "#00FFFF", "#0000FF", "#FF00FF")
}

// GetPeakColor returns the color for peak indicators: the top of the bar
// ramp (a fully-driven LED), i.e. red for Classic, magenta for Synthwave.
func GetPeakColor(scheme ColorScheme) lipgloss.Color {
	switch scheme {
	case SchemeClassic:
		return lipgloss.Color(CLASSIC_PEAK_COLOR) // Red
	case SchemeSynthwave:
		return lipgloss.Color(SYNTHWAVE_PEAK_COLOR) // Magenta
	default:
		return lipgloss.Color(CLASSIC_PEAK_COLOR)
	}
}

// interpolateColor performs linear interpolation between two hex colors
//
// LINEAR INTERPOLATION (LERP):
//   result = start + (end - start) × factor
//
// Applied independently to R, G, B channels:
//   R_result = R_start + (R_end - R_start) × factor
//   G_result = G_start + (G_end - G_start) × factor
//   B_result = B_start + (B_end - B_start) × factor
//
// EXAMPLE:
//   interpolateColor("#FF0000", "#0000FF", 0.5)
//   Red (#FF0000) to Blue (#0000FF) at 50%
//   = (255,0,0) to (0,0,255) at 0.5
//   = (127.5, 0, 127.5)
//   = (#7F007F) - Purple
//
// Parameters:
//   color1 - Start color (hex string, e.g., "#FF0000")
//   color2 - End color (hex string, e.g., "#0000FF")
//   factor - Interpolation factor (0.0 = color1, 1.0 = color2)
//
// Returns:
//   lipgloss.Color - Interpolated color as hex string
func interpolateColor(color1, color2 string, factor float64) lipgloss.Color {
	// Clamp factor to valid range
	if factor < 0.0 {
		factor = 0.0
	}
	if factor > 1.0 {
		factor = 1.0
	}

	// Parse hex colors to RGB
	r1, g1, b1 := parseHexColor(color1)
	r2, g2, b2 := parseHexColor(color2)

	// Interpolate each channel
	r := uint8(float64(r1) + factor*float64(r2-r1))
	g := uint8(float64(g1) + factor*float64(g2-g1))
	b := uint8(float64(b1) + factor*float64(b2-b1))

	// Convert back to hex
	hexColor := fmt.Sprintf("#%02x%02x%02x", r, g, b)
	return lipgloss.Color(hexColor)
}

// parseHexColor parses a hex color string to RGB components
//
// SUPPORTED FORMATS:
//   - "#RRGGBB" (e.g., "#FF8800")
//   - "#RGB"    (e.g., "#F80") - expands to "#FF8800"
//
// Parameters:
//   hex - Hex color string
//
// Returns:
//   r, g, b - RGB components (0-255)
func parseHexColor(hex string) (r, g, b uint8) {
	// Remove '#' if present
	if len(hex) > 0 && hex[0] == '#' {
		hex = hex[1:]
	}

	// Handle short form (#RGB)
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}

	// Parse hex to integers
	if len(hex) == 6 {
		if val, err := strconv.ParseUint(hex[0:2], 16, 8); err == nil {
			r = uint8(val)
		}
		if val, err := strconv.ParseUint(hex[2:4], 16, 8); err == nil {
			g = uint8(val)
		}
		if val, err := strconv.ParseUint(hex[4:6], 16, 8); err == nil {
			b = uint8(val)
		}
	}

	return
}

// ADDING NEW COLOR SCHEMES:
//
// 1. Add new constant:
//    const SchemeVaporwave ColorScheme = 2
//
// 2. Define colors in viz/config.go:
//    var VAPORWAVE_COLORS = []string{
//        "#FF71CE", "#01CDFE",  // Pink to cyan (0-50%)
//        "#01CDFE", "#B967FF",  // Cyan to purple (50-100%)
//    }
//    const VAPORWAVE_PEAK_COLOR = "#FF10F0"
//
// 3. Implement color function:
//    func getVaporwaveColor(heightPercent float64) lipgloss.Color {
//        if heightPercent < 0.5 {
//            return interpolateColor("#FF71CE", "#01CDFE", heightPercent/0.5)
//        } else {
//            return interpolateColor("#01CDFE", "#B967FF", (heightPercent-0.5)/0.5)
//        }
//    }
//
// 4. Add case to GetColorForHeight:
//    case SchemeVaporwave:
//        return getVaporwaveColor(heightPercent)
//
// 5. Add case to GetPeakColor:
//    case SchemeVaporwave:
//        return lipgloss.Color(VAPORWAVE_PEAK_COLOR)
//
// OTHER COLOR SCHEME IDEAS:
//
// Matrix (all green):
//   func getMatrixColor(heightPercent float64) lipgloss.Color {
//       intensity := uint8(100 + heightPercent*155)
//       return lipgloss.Color(fmt.Sprintf("#00%02x00", intensity))
//   }
//
// Fire (yellow to red):
//   0-50%:  #FFFF00 → #FF8800 (yellow to orange)
//   50-100%: #FF8800 → #FF0000 (orange to red)
//
// Ocean (blue gradient):
//   0-50%:  #001a33 → #0066cc (dark blue to medium)
//   50-100%: #0066cc → #00ccff (medium to bright cyan)
//
// Grayscale (monochrome):
//   intensity := uint8(heightPercent * 255)
//   return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", intensity, intensity, intensity))
//
// Rainbow (full spectrum):
//   // Convert heightPercent to HSV hue (0-360)
//   // Convert HSV to RGB
//   // Use color theory libraries for HSV↔RGB conversion
//
// See docs/ALGORITHMS.md for more examples and color theory details
