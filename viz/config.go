package viz

// CyberSpec Configuration
// All configurable constants for customizing behavior
// See docs/CONFIGURATION.md for detailed explanations

// =============================================================================
// AUDIO CAPTURE
// =============================================================================

// Sample rate for audio capture (Hz)
// Should match your system's audio configuration
// Check with: pactl info | grep "Default Sample Specification"
const SAMPLE_RATE = 48000

// Buffer size in samples per frame
// Calculation: SAMPLE_RATE / TARGET_FPS
// 48000 / 30 = 1600 samples (~33ms per frame)
const BUFFER_SIZE = 1600

// =============================================================================
// DSP PROCESSING
// =============================================================================

// FFT size (must be power of 2)
// Larger = better frequency resolution but slower
// Options: 512, 1024, 2048, 4096, 8192
const FFT_SIZE = 2048

// Enable A-weighting curve to balance bass/mid/treble visualization
// true  = Balanced (bass reduced, mids/highs boosted) - RECOMMENDED
// false = Raw FFT (bass-heavy)
// See docs/ALGORITHMS.md for detailed explanation
const ENABLE_A_WEIGHTING = true

// Enable debug output (shows band magnitudes, heights, etc.)
// TEMPORARY: For debugging rendering issues
const ENABLE_DEBUG_OUTPUT = true

// Minimum and maximum frequencies to analyze (Hz)
// Human hearing range: 20 Hz - 20,000 Hz
const MIN_FREQ = 20.0
const MAX_FREQ = 20000.0

// Number of frequency bands to display
// Common values: 8 (classic), 10, 16 (default), 32, 64
const NUM_BANDS = 16

// =============================================================================
// TEMPORAL SMOOTHING
// =============================================================================

// Smoothing factor for Exponential Moving Average (EMA)
// Range: 0.0 - 1.0
// Lower = smoother but more lag
// Higher = more responsive but jittery
//
// Presets (uncomment to use):
const SMOOTHING_ALPHA = 0.4 // Medium - balanced (RECOMMENDED)
// const SMOOTHING_ALPHA = 1.0   // None - instant response, jittery
// const SMOOTHING_ALPHA = 0.7   // Light - slight smoothing
// const SMOOTHING_ALPHA = 0.2   // Heavy - very smooth, noticeable lag
// const SMOOTHING_ALPHA = 0.1   // Maximum - extremely smooth, slow

// =============================================================================
// VISUALIZATION
// =============================================================================

// Target frames per second
// Higher = smoother but more CPU
// Recommended: 20-30 for balance, 60 for maximum smoothness
const TARGET_FPS = 30

// Bar spacing in characters
// 2 = Tight, 3 = Medium (default), 4 = Wide
const BAR_SPACING = 3

// Characters for drawing bars (from full to empty)
// Default uses Unicode block characters
var BAR_CHARS = []string{"█", "▇", "▆", "▅", "▄", "▃", "▂", "▁"}

// Alternative character sets (uncomment to use):
// ASCII-only (maximum compatibility)
// var BAR_CHARS = []string{"|", "|", "|", "|", ":", ":", ".", "."}
// Block-only (simpler)
// var BAR_CHARS = []string{"█", "█", "▄", "▄", "▄", "▁", "▁", " "}

// Peak bar character
const PEAK_CHAR = "━" // Heavy horizontal line

// Alternative peak characters (uncomment to use):
// const PEAK_CHAR = "▔"  // Upper block
// const PEAK_CHAR = "▁"  // Lower block
// const PEAK_CHAR = "─"  // Light horizontal line
// const PEAK_CHAR = "═"  // Double horizontal line
// const PEAK_CHAR = "▬"  // Black rectangle

// =============================================================================
// DECAY SYSTEM
// =============================================================================

// Decay algorithm to use
// Current implementation: "fibonacci_gaps"
// See docs/ALGORITHMS.md for alternatives
const DECAY_STYLE = "fibonacci_gaps"

// Future options (not yet implemented):
// const DECAY_STYLE = "fixed_segments"     // Fixed segments, growing gaps
// const DECAY_STYLE = "reducing_segments"  // Fibonacci number of segments
// const DECAY_STYLE = "solid"              // Traditional solid decay

// Fibonacci Decay: Energy percentage → Gap size (lines between segments)
// Lower energy = larger gaps = more fragmented appearance
// Inspired by user-provided screenshot (docs/decay-inspiration.png)
var DECAY_THRESHOLDS = map[int]int{
	90: 0,  // 90-100%: Solid bar (no gaps)
	70: 1,  // 70-90%:  1 line gap
	50: 2,  // 50-70%:  2 line gap
	30: 3,  // 30-50%:  3 line gap
	15: 5,  // 15-30%:  5 line gap
	5:  8,  // 5-15%:   8 line gap
	0:  13, // 0-5%:    13 line gap (barely visible fragments)
}

// Fibonacci Decay: Energy percentage → Segment height (lines per segment)
// Lower energy = shorter segments = smaller fragments
var SEGMENT_HEIGHTS = map[int]int{
	90: 999, // 90-100%: Solid (full height)
	70: 3,   // 70-90%:  3-line segments
	50: 2,   // 50-70%:  2-line segments
	30: 2,   // 30-50%:  2-line segments
	15: 1,   // 15-30%:  1-line segments (dashes)
	5:  1,   // 5-15%:   1-line dashes
}

// =============================================================================
// PEAK BEHAVIOR
// =============================================================================

// How long peak bar holds at maximum before falling (milliseconds)
// Range: 100-2000 ms
// Recommended: 500 ms for most music
const PEAK_HOLD_MS = 500

// Peak fall speed after hold time (units per second)
// Higher = faster fall
// Range: 5.0-50.0
const PEAK_DECAY_RATE = 15.0

// Peak flicker frequency range (Hz)
// Random flicker rate between these values
// Inspiration: "Failing light bulb" effect
const FLICKER_MIN_HZ = 3.0
const FLICKER_MAX_HZ = 5.0

// Peak flicker opacity range
// Min: never fully invisible (0.3 = always 30% visible)
// Max: full brightness
// Range: 0.0-1.0
const FLICKER_MIN_OPACITY = 0.3
const FLICKER_MAX_OPACITY = 1.0

// Brightness alternation factor
// Every other frame is dimmed by this factor
// 0.5 = 50% dimmer on alternate frames
// 1.0 = no alternation
// Range: 0.0-1.0
const FLICKER_DIM_FACTOR = 0.5

// Opacity decay curve exponent
// Controls how fast opacity fades as peak decays
// Higher = faster fade near end
const OPACITY_DECAY_EXPONENT = 1.5

// =============================================================================
// GAIN CONTROL
// =============================================================================

// Initial gain multiplier on startup
// 1.0 = neutral, <1.0 = quieter, >1.0 = louder
const DEFAULT_GAIN = 1.0

// Minimum and maximum gain limits
const MIN_GAIN = 0.1
const MAX_GAIN = 5.0

// Gain adjustment step per keypress (+/- keys)
const GAIN_STEP = 0.1

// =============================================================================
// COLOR SCHEMES
// =============================================================================

// Default color scheme on startup
// Options: "classic", "synthwave"
const DEFAULT_COLOR_SCHEME = "classic"

// Classic scheme: Green → Yellow → Red
// Traditional spectrum analyzer look
var CLASSIC_COLORS = []string{
	"#00FF00", "#88FF00", // Green (0-33%)
	"#FFFF00", "#FF8800", // Yellow/Orange (33-66%)
	"#FF4400", "#FF0000", // Red (66-100%)
}

// Classic peak color
const CLASSIC_PEAK_COLOR = "#00FFFF" // Bright cyan

// Synthwave scheme: Cyan → Magenta
// Cyberpunk aesthetic
var SYNTHWAVE_COLORS = []string{
	"#00AAAA", "#00DDFF", // Deep cyan (0-25%)
	"#00DDFF", "#8800FF", // Cyan-Purple (25-50%)
	"#8800FF", "#DD00DD", // Purple-Magenta (50-75%)
	"#DD00DD", "#FF00FF", // Hot Magenta (75-100%)
}

// Synthwave peak color
const SYNTHWAVE_PEAK_COLOR = "#FF00FF" // Bright magenta

// =============================================================================
// PERFORMANCE
// =============================================================================

// Show debug info overlay (FPS, CPU, etc.)
const SHOW_DEBUG_INFO = false

// Maximum frame rendering time before skipping (milliseconds)
// Prevents "spiral of death" in slow terminals
const MAX_FRAME_TIME_MS = 40

// =============================================================================
// KEYBOARD CONTROLS
// =============================================================================

// Documented here for reference
// Actual implementation in main.go

// c or 1/2  - Cycle color schemes (1=Classic, 2=Synthwave)
// + or =    - Increase gain (+GAIN_STEP)
// - or _    - Decrease gain (-GAIN_STEP)
// 0         - Reset gain to DEFAULT_GAIN
// q or ESC  - Quit
// Ctrl+C    - Force quit
