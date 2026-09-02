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

// Samples pulled from the capture stream per read (SAMPLE_RATE / TARGET_FPS).
// The capturer keeps a rolling window of the most recent FFT_SIZE samples and
// hands that whole window to the FFT, so this only controls read granularity.
const BUFFER_SIZE = 1600

// =============================================================================
// DSP PROCESSING
// =============================================================================

// FFT size (must be power of 2). This is also the rolling analysis window
// the capturer fills. 4096 @ 48kHz ≈ 85ms window, ~11.7 Hz bins — enough to
// resolve the low end. Larger = finer bass but sluggish transients.
const FFT_SIZE = 4096

// A-weighting models ear sensitivity but cuts the low end by 20-40 dB, which
// makes bass vanish from the visual. Off by default; SPECTRAL_TILT_DB_PER_OCT
// below is the gentle middle ground.
//   true  = perceptual (mid-forward, bass suppressed)
//   false = raw FFT magnitude (full bass)
const ENABLE_A_WEIGHTING = false

// Spectral tilt — the in-between between raw FFT (bass-heavy, no highs) and
// A-weighting (bass gone). A boost-only high shelf: each octave above
// MIN_FREQ is lifted by this many dB (bass stays at unity), all the way up.
// The slope is uniform, so more tilt always means more high end.
//   0    = flat (raw)
//   1.0  = subtle
//   3.0  = balanced for music (default)
//   4.5+ = bright / treble-forward (bass shrinks as the AGC follows the highs)
// Adjust live with [ and ]; override the launch value with `-tilt`.
const SPECTRAL_TILT_DB_PER_OCT = 3.0

// Hard limit on the live/CLI tilt slope.
const SPECTRAL_TILT_MAX_SLOPE = 6.0

// Automatic gain control — cava / cli-visualizer style. One `sensitivity`
// scalar multiplies every band. It calibrates fast on launch, then adapts
// gently so the display BREATHES with the music (loud passages fill the
// screen, quiet passages stay low) instead of being renormalised to full
// scale every frame. No gain keys needed.
//   TARGET      - loud peaks should reach this fraction of full height
//   DOWN        - pull-down strength when a peak clips (0..1; 1 = snap)
//   LOW_RATIO   - "low" means the scaled peak is below TARGET*LOW_RATIO ...
//   LOW_FRAMES  - ... for this many consecutive frames, then lift by UP
//   UP          - gentle per-step lift when the picture stays low
//   INIT_UP     - fast per-frame lift during the initial calibration ramp
//   QUIET_FLOOR - scaled peak below this = treat as silence, hold steady
//   NOISE_GATE  - bands below this fraction of full render as zero
const AGC_TARGET = 0.90
const AGC_DOWN = 0.5
const AGC_LOW_RATIO = 0.70
const AGC_LOW_FRAMES = 15
const AGC_UP = 1.02
const AGC_INIT_UP = 1.10
const AGC_QUIET_FLOOR = 0.02
const AGC_NOISE_GATE = 0.03

// Enable debug output (shows band magnitudes, heights, etc.)
// TEMPORARY: For debugging rendering issues
const ENABLE_DEBUG_OUTPUT = false

// Frequency range to analyze (Hz). Start at 30 Hz — there is essentially no
// musical content or speaker output below that, and 20-30 Hz bands just show
// up as dead slots on the left.
const MIN_FREQ = 30.0
const MAX_FREQ = 20000.0

// Number of frequency bands.
// When AUTO_BANDS is true this is only the fallback used before the terminal
// size is known (and in non-TTY contexts); the live count is derived from
// the terminal width each resize. The `-bands N` CLI flag forces a fixed N.
const NUM_BANDS = 32

// Auto-size the band count to the terminal width (like cli-visualizer).
const AUTO_BANDS = true

// Terminal columns one band occupies: 2 for the bar + 1 gap. Must match the
// spacing written by renderer.transposeColumns.
const BAND_COLUMNS = 3

// Clamp for the auto-sized band count.
const MIN_BANDS = 8
const MAX_BANDS = 96

// =============================================================================
// SPATIAL SMOOTHING
// =============================================================================

// Monstercat-style neighbour spread (dpayne/cli-visualizer). Each band bleeds
// into its neighbours as value / factor^distance, taking the max. This fills
// dips and widens peaks so the spectrum reads as a smooth envelope instead of
// isolated spikes.
//   ~1.3  = heavy spread (very smooth, blobby)
//   1.5   = balanced (default)
//   ~2.5  = light spread
//   <=1.0 = disabled
const MONSTERCAT_FACTOR = 1.5

// =============================================================================
// TEMPORAL SMOOTHING
// =============================================================================

// The bar smoother is asymmetric (fast attack, slow release), like
// dpayne/cli-visualizer. Bars jump up toward louder audio quickly, then
// ooze back down by exponential decay.

// Attack coefficient — how fast bars RISE toward a louder value.
// EMA weight applied only while rising: height = a*current + (1-a)*previous
// Range 0.0-1.0. Higher = snappier rise. 1.0 = instant.
const SMOOTHING_ALPHA = 0.7

// Release weight — how fast bars FALL. Each frame a falling bar decays to
// this fraction of its previous height (clamped to never drop below the
// live audio level). Lower = faster fall.
// 0.93 @ 30 FPS → ~1.05s from full height to 10%. A bit snappier than
// cli-visualizer so the peak indicator clearly separates above the bar.
// Range ~0.90 (fast) .. 0.985 (very slow / floaty).
const BAR_FALLOFF_WEIGHT = 0.93

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
// BAR STYLE
// =============================================================================

// How each vertical bar is drawn:
//   "solid"     - One continuous block column, single color per bar. (default)
//   "led"       - Stacked LED segments: short lit blocks with unlit gaps,
//                 colored by vertical position (green low → red high).
//                 Hardware-spectrum-analyzer look.
//   "braille"   - Braille dot-fill for 4× sub-row height resolution and a
//                 fine dotted texture. Needs a font with U+28xx glyphs.
//   "fibonacci" - Fragment bars into Fibonacci-spaced segments as energy
//                 drops (see DECAY_THRESHOLDS / SEGMENT_HEIGHTS). Currently
//                 fragments almost everything because normalizeBands()
//                 peak-normalizes each frame — needs rework before it looks
//                 right. See docs/decay-inspiration.png.
const BAR_STYLE = "solid"

// LED style: rows per lit segment and per unlit gap (segment + gap repeats
// up the bar). 2/1 gives chunky segments with thin dark seams; 1/1 is a
// sparse dot-matrix column; 3/1 is nearly solid with hairline seams.
const LED_SEGMENT_ROWS = 2
const LED_GAP_ROWS = 1

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
const PEAK_HOLD_MS = 150

// Peak fall — per-frame exponential decay, same model as BAR_FALLOFF_WEIGHT.
// Each frame a falling peak keeps this fraction of its height.
//
// MUST be > BAR_FALLOFF_WEIGHT: because both decays are per-frame and
// proportional to height, a larger weight here means the peak sheds a
// smaller fraction every frame at every height, so a falling peak can
// never descend onto a falling bar. The peak only rejoins the bar when
// fresh audio pushes the bar back up to it.
// Range ~(BAR_FALLOFF_WEIGHT+0.01) .. 0.995.
const PEAK_FALLOFF_WEIGHT = 0.97

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
const DEFAULT_COLOR_SCHEME = "synthwave"

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
