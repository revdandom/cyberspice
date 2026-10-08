package viz

// CyberSpice Configuration
// All configurable constants for customizing behavior
// Every constant here is documented inline; see docs/how-it-works.md for how

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

// Zero-padding factor: the FFT_SIZE-sample window is padded with zeros to
// FFT_SIZE*FFT_PAD points before the FFT. This adds no real resolution and no
// latency, but samples the spectrum every ~2.9 Hz instead of ~11.7 Hz, so the
// narrow low bands each get their own value instead of several adjacent bars
// reading the same bin and moving in lockstep. 1 = no padding.
const FFT_PAD = 4

// A-weighting models ear sensitivity but cuts the low end by 20-40 dB, which
// makes bass vanish from the visual. Off by default; SPECTRAL_TILT_DB_PER_OCT
// below is the gentle middle ground.
//
//	true  = perceptual (mid-forward, bass suppressed)
//	false = raw FFT magnitude (full bass)
const ENABLE_A_WEIGHTING = false

// Spectral tilt — the in-between between raw FFT (bass-heavy, no highs) and
// A-weighting (bass gone). A boost-only high shelf: each octave above
// MIN_FREQ is lifted by this many dB (bass stays at unity), all the way up.
// The slope is uniform, so more tilt always means more high end.
//
//	0    = flat (raw)
//	1.0  = subtle
//	3.0  = balanced for music (default)
//	4.5+ = bright / treble-forward (bass shrinks as the AGC follows the highs)
//
// Adjust live with [ and ]; override the launch value with `-tilt`.
const SPECTRAL_TILT_DB_PER_OCT = 3.0

// Hard limit on the live/CLI tilt slope.
const SPECTRAL_TILT_MAX_SLOPE = 6.0

// =============================================================================
// AMPLITUDE SCALE
// =============================================================================

// Bar height vs. sound level (the loudness "curve"). The pipeline is linear
// in amplitude, but the ear is not, so raw amplitude makes slightly-louder
// sounds shoot up. Modes (cycle with 'a', or -curve):
//
//	"linear"  - value maps straight through (raw amplitude).
//	"stevens" - value^AMPLITUDE_EXPONENT. Stevens' power law: perceived
//	            loudness ≈ (sound pressure)^0.6, so bar height ≈ loudness.
//	"db"      - a fixed dB window: bottom of the bar = AMPLITUDE_DB_FLOOR dB,
//	            top = 0 dB, linear in between. Equal dB steps → equal bar
//	            steps; opens up quiet detail the most (analyzer-style).
const AMPLITUDE_MODE_DEFAULT = "stevens"

// Exponent for "stevens" mode.
const AMPLITUDE_EXPONENT = 0.6

// Bottom-of-bar level for "db" mode, in dB below full scale.
const AMPLITUDE_DB_FLOOR = -60.0

// Automatic gain control — cava / cli-visualizer style. One `sensitivity`
// scalar multiplies every band. It calibrates fast on launch, then adapts
// gently so the display BREATHES with the music (loud passages fill the
// screen, quiet passages stay low) instead of being renormalised to full
// scale every frame. No gain keys needed.
//
//	TARGET      - loud peaks should reach this fraction of full height
//	DOWN        - pull-down strength when a peak clips (0..1; 1 = snap)
//	LOW_RATIO   - "low" means the scaled peak is below TARGET*LOW_RATIO ...
//	LOW_FRAMES  - ... for this many consecutive frames, then lift by UP
//	UP          - gentle per-step lift when the picture stays low
//	INIT_UP     - fast per-frame lift during the initial calibration ramp
//	QUIET_FLOOR - scaled peak below this = treat as silence, hold steady
//	NOISE_GATE  - bands below this fraction of full render as zero
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

// Frequency range to analyze (Hz): the full audible band, 20 Hz - 20 kHz.
// Little music or speaker output reaches below 30 Hz, so the leftmost bands
// can sit low or empty on most tracks.
const MIN_FREQ = 20.0
const MAX_FREQ = 20000.0

// Number of frequency bands.
// When AUTO_BANDS is true this is only the fallback used before the terminal
// size is known (and in non-TTY contexts); the live count is derived from
// the terminal width each resize. The `-bands N` CLI flag forces a fixed N.
const NUM_BANDS = 32

// Auto-size the band count to the terminal width (like cli-visualizer).
const AUTO_BANDS = true

// Cells per bar (all styles). 2 keeps bars narrow so more channels fit.
const BAR_WIDTH = 2

// Layout: "vertical" (bars rise, frequency left→right) or "butterfly"
// (frequency bottom→top, one row per band; left channel grows left from the
// centre, right channel grows right). Toggle live with 'l'.
const LAYOUT_DEFAULT = "vertical"

// butterfly layout: terminal rows per frequency band (mirror of BAR_WIDTH).
const BAND_ROWS = 2

// butterfly layout: blank columns down the middle between the L and R
// halves. 0 = the halves meet with no seam; raise it for a visible gutter.
const BUTTERFLY_CENTER_GAP = 0

// Terminal columns one band occupies: the bar plus a 1-cell gap. Must match
// the spacing written by renderer.transposeColumns.
const BAND_COLUMNS = BAR_WIDTH + 1

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
//
//	~1.3  = heavy spread (very smooth, blobby)
//	1.5   = balanced (default)
//	~2.5  = light spread
//	<=1.0 = disabled
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

// Show the header (title/gain/scheme/...) and footer (key help) bars on
// startup. When false the spectrum fills the whole screen; pressing any key
// that has no other action toggles them back on.
const SHOW_CHROME_DEFAULT = true

// HACKERBOT intro splash: a braille halftone of the still (the tin-robot
// figure, the chrome wordmark, the office) holds briefly, then dissolves
// into dots that pour into a pool and fade out. -splash / a `splash` config
// key override it; any key dismisses it early.
const SPLASH_ENABLED = true
const SPLASH_HOLD_MS = 1600     // the scene holds this long before it crumbles
const SPLASH_MOVE_MIN_MS = 1300 // minimum pour time before an early pool-out
const SPLASH_DECAY_MS = 4000    // hard cap on the pour phase
const SPLASH_FADE_MS = 850      // pool fade-out once (mostly) pooled
const SPLASH_GRAVITY = 0.12     // dot-cells/frame² the particles accelerate
const SPLASH_POOL_FRACTION = 0.95

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
//
//	"solid"     - One continuous block column, single color per bar. (default)
//	"led"       - Stacked LED segments: short lit blocks with unlit gaps,
//	              colored by vertical position (green low → red high).
//	              Hardware-spectrum-analyzer look.
//	"braille"   - Braille dot-fill for 4× sub-row height resolution and a
//	              fine dotted texture. Needs a font with U+28xx glyphs.
//	"gradient"  - Solid column with a vertical brightness gradient of the
//	              scheme's peak colour: bright at the base, fading toward
//	              GRADIENT_TIP_FLOOR at the tip (a "beam" that thins out).
//	"constellation" - Not bars: a field of drifting, twinkling points
//	              connected by faint lines, reacting to both channels
//	              (butterfly mirrors it left/right). See the CONSTELLATION
//	              STYLE section below and viz/constellation.go.
//	"rain"      - Not bars: a digital-rain spectrograph. Small braille dots
//	              fall from the top, each coloured by its frequency's
//	              loudness when it spawned and keeping that colour as it
//	              falls. See the RAIN STYLE section below and viz/rain.go.
//	              Dot colour (fade-in, silence cutoff): DOT COLOUR section.
//	"waterfall" - Not bars: a falling spectrograph. Like rain, but a full,
//	              evenly spaced row of dots is emitted at a steady rate and
//	              every row falls at the same speed — a regular dot grid
//	              scrolling down. See WATERFALL STYLE and viz/waterfall.go.
const BAR_STYLE = "solid"

// LED style: one amplitude level per cell row, drawn as a lower-partial
// block — a short lit segment on a dark upper half (the gap) in the same
// cell. Blocks are half a row tall, 1:1 lit:gap, one level per row.
//
// LED_LINE_GLYPH sets the lit fraction of each cell:
//
//	"▄" = 1/2 (default)   "▃" = 3/8   "▂" = 1/4   "▁" = 1/8 (thinner, more air)
const LED_LINE_GLYPH = "▄"

// Colour of the dark (gap) half of each LED cell. Black by default; set to
// "" to fall back to the terminal background, or to match a non-black one.
const LED_GAP_COLOR = "#000000"

// gradient style: brightness of the bar's tip relative to its base
// (base is always full). 0.05 = the tip fades almost to black; raise it to
// keep the whole bar readable.
const GRADIENT_TIP_FLOOR = 0.05

// =============================================================================
// CONSTELLATION STYLE
// =============================================================================

// A field of drifting points connected to nearby neighbours by faint lines —
// ported from oiwn/tarts (https://github.com/oiwn/tarts, MIT) and made
// audio-reactive. See viz/constellation.go for the simulation; colour comes
// from the active ColorScheme, not tarts' own fixed palette (see
// tintConstellation).

// Stars per cell of field area, clamped to [MIN,MAX]. Kept low: at the
// original 0.035 a typical terminal filled with 150-180 stars, and combined
// with CONSTELLATION_CONNECT_RADIUS that put nearly every star within
// range of several others — the connections tiled the whole screen into a
// dense, static-looking mesh instead of a few readable constellations.
const CONSTELLATION_STAR_DENSITY = 0.015
const CONSTELLATION_MIN_STARS = 18
const CONSTELLATION_MAX_STARS = 110

// Connection radius, as a fraction of the field's diagonal, at rest.
const CONSTELLATION_CONNECT_RADIUS = 0.11

// How much bass energy (0..1) widens that radius, multiplicatively — with
// bandEnergy now reporting peak (not average) band energy, a real kick
// pushes this close to its full swing, visibly pulling more lines in.
const CONSTELLATION_BASS_RADIUS_BOOST = 1.3

// A star won't connect to more than this many neighbours.
const CONSTELLATION_MAX_CONNECTIONS = 3

// Drift speed range, cells/second, at rest (silence). Raised from the
// original 0.3-1.2: at that range the field barely appeared to move
// between glances even with the energy boost below.
const CONSTELLATION_MIN_SPEED = 0.9
const CONSTELLATION_MAX_SPEED = 2.4

// How much overall energy (0..1) multiplies that speed, on top of 1x.
const CONSTELLATION_ENERGY_SPEED_BOOST = 2.5

// Twinkle angular speed (radians/sec-ish, scaled per star by its own random
// multiplier), at rest and added per unit of treble energy.
const CONSTELLATION_TWINKLE_BASE = 0.8
const CONSTELLATION_TWINKLE_TREBLE_BOOST = 3.0

// A frame-to-frame jump in overall energy bigger than this counts as a
// transient (a kick, a snare hit) and fires a brief whole-field flash.
// Raised alongside the switch to peak-based bandEnergy, which swings much
// more per frame than the old average did.
const CONSTELLATION_ONSET_THRESHOLD = 0.22

// Per-frame decay of the flash brightness once triggered.
const CONSTELLATION_FLASH_DECAY = 0.85

// Glyphs stars are drawn with, mirroring tarts' STAR_GLYPHS (plain dot up to
// a four-pointed sparkle). Connections are always drawn with '·'.
var CONSTELLATION_GLYPHS = []rune{'.', '*', '+', '✦'}

// Terminal cells are taller than wide (roughly 1:2). This is how much a
// y-distance is scaled up relative to an x-distance when computing "is this
// cell near that wave ring" or laying rings out on the vertical style's
// circle, so a ring/circle that's meant to be round on screen actually
// looks round in the field's raw (uneven) row/column units.
const CONSTELLATION_CELL_ASPECT = 2.0

// How much of the old whole-field "ambient" hue still shows as a dim
// baseline between wave hits, so the field isn't pure black at low volume.
// 0 = fully black between waves; 1 = as strong as the waves themselves.
const CONSTELLATION_AMBIENT_MIX = 0.35

// A band's gain-adjusted level must reach this before it spawns a colour
// ring (see viz/constellation.go's waveEmitter). Below cava/monstercat's own
// noise floor a band never gets this high at all.
const CONSTELLATION_WAVE_SPAWN_THRESHOLD = 0.35

// Minimum seconds between two rings spawned by the SAME band, so a
// sustained loud note doesn't spawn a new ring every single frame.
const CONSTELLATION_WAVE_SPAWN_COOLDOWN_S = 0.25

// How fast a colour ring's radius grows, in the same aspect-corrected
// distance units as CONSTELLATION_CELL_ASPECT — cells/second.
const CONSTELLATION_WAVE_SPEED = 14.0

// A ring's thickness (how far from its exact radius a cell still counts as
// "on the ring"), in the same units as CONSTELLATION_WAVE_SPEED.
const CONSTELLATION_WAVE_WIDTH = 3.5

// How long a ring lives, in seconds, before it's culled — its brightness
// contribution fades linearly to 0 over this span. At CONSTELLATION_WAVE_SPEED
// a ring travels roughly SPEED*this many cells before dying (~42 at the
// defaults below) — enough to visibly cross a good part of a typical
// terminal, not just a few cells around where it spawned.
const CONSTELLATION_WAVE_FADE_S = 3.0

// Hard cap on simultaneously active rings per field-side; the oldest are
// dropped past this so a wall of noise can't grow the slice forever.
const CONSTELLATION_WAVE_MAX_ACTIVE = 24

// =============================================================================
// RAIN STYLE
// =============================================================================

// Digital-rain spectrograph (viz/rain.go). Units are braille dots: a
// terminal cell is 2 dots wide and 4 tall, and every dot column (vertical)
// or dot row (butterfly) is one lane.

// Drops spawned per lane per second at silence. Lower = sparser rain.
const RAIN_SPAWN_RATE = 0.35

// Extra spawn rate at full loudness, as a multiple of RAIN_SPAWN_RATE
// (1.0 = a loud band rains twice as densely as a quiet one; 0 = density
// ignores loudness and only the colour reacts).
const RAIN_LEVEL_SPAWN_BOOST = 1.0

// Per-drop fall speed, picked at random in this range, in dots/second.
// At 40 rows (160 dots) the defaults take ~3-6 s to cross the screen.
const RAIN_MIN_SPEED = 28.0
const RAIN_MAX_SPEED = 55.0

// A lane won't spawn a new drop until its newest one has moved this many
// dots from the spawn edge, so dots don't pile up at the head.
const RAIN_MIN_GAP = 3.0

// =============================================================================
// WATERFALL STYLE
// =============================================================================

// Falling spectrograph (viz/waterfall.go). Colours scroll one cell per
// step; between steps each cell's glyph animates through WATERFALL_GLYPHS.

// Frames per one-cell step. At TARGET_FPS 30, 4 = 7.5 rows/s, so a 40-row
// screen holds ~5 s of history. Keep it a multiple of the glyph count (4)
// so each glyph shows for the same number of frames.
const WATERFALL_FRAMES_PER_ROW = 4

// Glyph animation across one step, played in order. The default slides one
// dot (left column, so cells keep space on both axes) down the cell, one
// dot row per frame, for smooth motion. Other options:
//
//	"⠉⠒⠤⣀" = two dots wide, sliding (denser)
//	"⠃"    = static two-dot glyph; colours alone scroll (cell steps)
//	"⠿"    = static, thick (the previous default)
const WATERFALL_GLYPHS = "⠁⠂⠄⡀"

// Butterfly scrolls sideways: the dot steps across the cell's two dot
// columns, written for the right half (outward = rightward); the left half
// plays it mirrored.
const WATERFALL_GLYPHS_BUTTERFLY = "⠂⠐"

// Rows fade in from black over the first WATERFALL_FADE_IN cells after the
// spawn edge (top, or the butterfly's centre seam) and fade out over the last
// WATERFALL_FADE_OUT cells before the far edge, instead of popping in and
// out. The fade follows the sliding dot sub-step by sub-step, so it is
// smooth. 0 = no fade at that end.
const (
	WATERFALL_FADE_IN  = 1
	WATERFALL_FADE_OUT = 2
)

// =============================================================================
// DOT COLOUR (rain + waterfall)
// =============================================================================

// Below this curved 0..1 level a lane draws no dot, the way a bar with no
// height draws nothing — silent frequencies stay black. 0 = every lane
// always draws.
const DOT_MIN_LEVEL = 0.03

// The scheme ramp holds the base colour flat from 0 to 0.30 (rampBaseEnd in
// colors.go). The dot styles use that range instead: from DOT_MIN_LEVEL up
// to 0.30 a dot fades in from DOT_FADE_FLOOR brightness to the full base
// colour, then follows the normal ramp toward the hot colour. Below ~0.1
// the dimmest dots get hard to see on black; 1.0 turns the fade off.
const DOT_FADE_FLOOR = 0.18

// Brightness steps across the fade. Quantizing keeps neighbouring cells
// on identical colours (cheaper output, see renderCellRow) without visible
// banding at 24-bit colour.
const DOT_FADE_STEPS = 48

// =============================================================================
// PEAK BEHAVIOR
// =============================================================================

// How long peak bar holds at maximum before falling (milliseconds)
// Range: 100-2000 ms
// Recommended: 500 ms for most music
const PEAK_HOLD_MS = 150

// Whether the peak markers are drawn at all. Toggle live with 'p'.
const SHOW_PEAKS_DEFAULT = true

// Whether the peak marker falls after its hold (true) or stays at its
// captured height and only fades out (false). Toggle live with 'f'.
const PEAK_FALL_DEFAULT = true

// Peak fall — per-frame exponential decay, same model as BAR_FALLOFF_WEIGHT.
// Only used when the falling animation is on. Each frame a falling peak
// keeps this fraction of its height.
//
// MUST be > BAR_FALLOFF_WEIGHT: because both decays are per-frame and
// proportional to height, a larger weight here means the peak sheds a
// smaller fraction every frame at every height, so a falling peak can
// never descend onto a falling bar. The peak only rejoins the bar when
// fresh audio pushes the bar back up to it.
// Range ~(BAR_FALLOFF_WEIGHT+0.01) .. 0.995.
const PEAK_FALLOFF_WEIGHT = 0.97

// Peak marker fade: milliseconds from a band's last peak until its marker
// has faded fully to black. The Age timer resets on every new peak, so
// active bands stay bright and only quiet ones fade out.
const PEAK_FADE_MS = 1200

// Fade curve exponent, applied to the 0..1 fade progress before blending the
// marker toward black (see renderer.FadePeakColor). Values < 1 bend the
// curve so the *perceived* brightness drops steadily instead of hanging
// bright and then snapping dark. 0.45 ≈ the inverse of display gamma; the
// cheapest fade curve that tracks the eye (one math.Pow, no colour library).
const PEAK_FADE_GAMMA = 0.45

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

// Colors ramp like an RGB LED driven harder: a base plateau at the bottom,
// then a transition to the top color which is reached near (not at) full
// height and held. The peak bar is the top-of-ramp color.
// See viz/colors.go ledRamp() for the actual stops.

// Classic: green --add red--> yellow --drop green--> red
var CLASSIC_COLORS = []string{"#00FF00", "#FFFF00", "#FF0000"}

const CLASSIC_PEAK_COLOR = "#FF0000" // Red

// Synthwave: cyan --(drop green + add red)--> magenta (single smooth lerp)
var SYNTHWAVE_COLORS = []string{"#00FFFF", "#FF00FF"}

const SYNTHWAVE_PEAK_COLOR = "#FF00FF" // Magenta

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
