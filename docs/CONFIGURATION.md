# CyberSpec - Configuration Guide

This document lists all configurable constants in CyberSpec and explains how to modify them to customize behavior.

All constants are defined in `viz/config.go` with extensive comments. This guide provides a quick reference and usage examples.

---

## Quick Reference

| Constant | Default | Range | Effect |
|----------|---------|-------|--------|
| `ENABLE_A_WEIGHTING` | `true` | bool | Enable/disable frequency weighting |
| `SMOOTHING_ALPHA` | `0.4` | 0.0-1.0 | Temporal smoothing (lower = smoother) |
| `NUM_BANDS` | `16` | 8-64 | Number of frequency bars |
| `TARGET_FPS` | `30` | 10-60 | Frame rate target |
| `FLICKER_MIN_HZ` | `3.0` | 1.0-10.0 | Peak flicker min frequency |
| `FLICKER_MAX_HZ` | `5.0` | 1.0-10.0 | Peak flicker max frequency |
| `PEAK_HOLD_MS` | `500` | 100-2000 | How long peak holds before decay |
| `PEAK_DECAY_RATE` | `15.0` | 5.0-50.0 | Peak fall speed (units/sec) |
| `DEFAULT_GAIN` | `1.0` | 0.1-5.0 | Initial gain multiplier |
| `DECAY_STYLE` | `fibonacci_gaps` | string | Decay algorithm to use |

---

## Audio Capture

### Sample Rate

```go
const SAMPLE_RATE = 48000  // Hz
```

**What it does:** Audio sampling rate from PipeWire/PulseAudio

**When to change:**
- System uses different rate (check with `pactl info`)
- Usually 44100 or 48000

**Impact:** Higher = better frequency resolution, more CPU

---

### Buffer Size

```go
const BUFFER_SIZE = 1600  // samples
```

**What it does:** Number of samples captured per frame

**Calculation:** `SAMPLE_RATE / TARGET_FPS`
- 48000 / 30 = 1600 samples per frame
- Each frame is ~33ms of audio

**When to change:**
- Adjusting FPS (must recalculate)
- Trade latency vs smoothness

---

## DSP Processing

### FFT Size

```go
const FFT_SIZE = 2048  // Must be power of 2
```

**What it does:** Number of samples in FFT window

**Valid values:** 512, 1024, 2048, 4096, 8192

**Trade-offs:**

| Size | Frequency Resolution | Time Resolution | CPU Usage |
|------|---------------------|-----------------|-----------|
| 512  | Low (wide bins)     | High (fast)     | Low       |
| 1024 | Medium              | Medium          | Medium    |
| 2048 | Good (default)      | Good            | Medium    |
| 4096 | High (narrow bins)  | Low (slow)      | High      |
| 8192 | Very high           | Very low        | Very high |

**When to change:**
- More frequency detail: increase
- Faster response: decrease
- Performance issues: decrease

---

### A-Weighting

```go
const ENABLE_A_WEIGHTING = true
```

**What it does:** Apply A-weighting curve to balance bass/mid/treble visually

**Values:**
- `true`: Balanced visualization (recommended)
- `false`: Raw FFT (bass-heavy)

**When to disable:**
- Want to see raw frequency energy
- Debugging DSP issues
- Specific musical analysis

---

### Frequency Range

```go
const MIN_FREQ = 20.0    // Hz (lowest human hearing)
const MAX_FREQ = 20000.0 // Hz (highest human hearing)
```

**What it does:** Frequency range covered by all bands

**When to change:**
- Focus on specific range (e.g., 60-8000 for voice)
- Extend for ultrasonic analysis

**Examples:**

```go
// Focus on musical fundamentals
const MIN_FREQ = 60.0
const MAX_FREQ = 8000.0

// Include sub-bass
const MIN_FREQ = 10.0
const MAX_FREQ = 20000.0
```

---

### Number of Bands

```go
const NUM_BANDS = 16
```

**What it does:** How many frequency bars to display

**Common values:**
- `8`: Classic spectrum analyzer look
- `10`: Retro stereo equipment style
- `16`: Default, good detail
- `32`: High detail
- `64`: Maximum detail (may be cluttered)

**Impact:**
- More bands = more detail, narrower bars, wider display
- Fewer bands = simpler look, wider bars

**Note:** May need to adjust `BAR_SPACING` if changing

---

## Visualization

### Target FPS

```go
const TARGET_FPS = 30
```

**What it does:** Frames per second for rendering

**Recommended values:**
- `15`: Low CPU, still smooth
- `20`: Balanced
- `30`: Smooth (default)
- `60`: Very smooth, high CPU

**Impact:** Higher FPS = smoother but more CPU usage

**Note:** Must recalculate `BUFFER_SIZE` if changing

---

### Smoothing

```go
const SMOOTHING_ALPHA = 0.4  // Medium smoothing
```

**What it does:** Controls temporal smoothing of bar motion

**Range:** 0.0 - 1.0

**Presets:**

```go
const SMOOTHING_ALPHA = 1.0   // None - instant response, jittery
const SMOOTHING_ALPHA = 0.7   // Light - slight smoothing
const SMOOTHING_ALPHA = 0.4   // Medium - balanced (default)
const SMOOTHING_ALPHA = 0.2   // Heavy - very smooth, noticeable lag
const SMOOTHING_ALPHA = 0.1   // Maximum - extremely smooth, slow
```

**How to choose:**
- Fast music (EDM, metal): 0.6-0.7
- Most music: 0.4
- Ambient/slow: 0.2-0.3
- Demo/presentation: 0.1

---

### Bar Spacing

```go
const BAR_SPACING = 3  // Characters per band
```

**What it does:** Horizontal spacing between bars

**Values:**
- `2`: Tight (`██ ██ ██`)
- `3`: Medium (`██  ██  ██`) - default
- `4`: Wide (`██   ██   ██`)
- `5`: Very wide

**Impact:** Wider = easier to see individual bands, wider terminal required

---

### Bar Characters

```go
var BAR_CHARS = []string{"█", "▇", "▆", "▅", "▄", "▃", "▂", "▁"}
```

**What it does:** Characters used to draw bars, from full to empty

**When to change:**
- Different terminal font
- Accessibility needs
- Aesthetic preference

**Alternatives:**

```go
// ASCII-only (maximum compatibility)
var BAR_CHARS = []string{"|", "|", "|", "|", ":", ":", ".", "."}

// Block-only (simpler)
var BAR_CHARS = []string{"█", "█", "▄", "▄", "▄", "▁", "▁", " "}

// Dots (minimalist)
var BAR_CHARS = []string{"⣿", "⣷", "⣶", "⣤", "⣀", "⡀", "⠀", " "}
```

---

### Peak Bar Character

```go
const PEAK_CHAR = "━"  // Heavy horizontal line
```

**What it does:** Character for peak indicator above bars

**Alternatives:**

```go
const PEAK_CHAR = "▔"  // Upper block
const PEAK_CHAR = "▁"  // Lower block  
const PEAK_CHAR = "─"  // Light horizontal line
const PEAK_CHAR = "═"  // Double horizontal line
const PEAK_CHAR = "━"  // Heavy horizontal line (default)
const PEAK_CHAR = "▬"  // Black rectangle
```

---

## Decay System

### Decay Style

```go
const DECAY_STYLE = "fibonacci_gaps"
```

**What it does:** Which decay algorithm to use

**Current values:**
- `"fibonacci_gaps"`: Fragmenting decay (implemented)

**Future options (not implemented):**
- `"fixed_segments"`: Fixed segments, growing gaps
- `"reducing_segments"`: Fibonacci number of segments
- `"solid"`: Traditional solid decay

---

### Fibonacci Decay Thresholds

```go
var DECAY_THRESHOLDS = map[int]int{
    90: 0,   // Solid
    70: 1,   // 1 line gap
    50: 2,   // 2 line gap
    30: 3,   // 3 line gap
    15: 5,   // 5 line gap
    5:  8,   // 8 line gap
    0:  13,  // 13 line gap
}
```

**What it does:** Maps energy percentage to gap size between segments

**How to customize:**

```go
// More aggressive fragmentation (fragments earlier)
var DECAY_THRESHOLDS = map[int]int{
    95: 0,
    80: 1,
    60: 2,
    40: 3,
    20: 5,
    10: 8,
}

// Gentler fragmentation (stays solid longer)
var DECAY_THRESHOLDS = map[int]int{
    95: 0,
    85: 1,
    75: 2,
    50: 3,
    25: 5,
    10: 8,
}

// Different sequence (powers of 2)
var DECAY_THRESHOLDS = map[int]int{
    90: 0,
    70: 1,
    50: 2,
    30: 4,
    15: 8,
    5:  16,
}
```

---

### Segment Heights

```go
var SEGMENT_HEIGHTS = map[int]int{
    90: 999, // Solid (full height)
    70: 3,   // 3-line segments
    50: 2,   // 2-line segments
    30: 2,   // 2-line segments
    15: 1,   // 1-line segments
    5:  1,   // 1-line dashes
}
```

**What it does:** Height of each segment at different energy levels

**How to customize:**

```go
// Taller segments
var SEGMENT_HEIGHTS = map[int]int{
    90: 999,
    70: 5,   // Taller
    50: 4,
    30: 3,
    15: 2,
    5:  1,
}

// All single-line segments (dashes)
var SEGMENT_HEIGHTS = map[int]int{
    90: 999,
    70: 1,   // Always 1 line
    50: 1,
    30: 1,
    15: 1,
    5:  1,
}
```

---

## Peak Behavior

### Peak Hold Time

```go
const PEAK_HOLD_MS = 500  // milliseconds
```

**What it does:** How long peak bar stays at maximum before falling

**Range:** 100-2000 ms

**Examples:**

```go
const PEAK_HOLD_MS = 250   // Quick drop (fast music)
const PEAK_HOLD_MS = 500   // Default
const PEAK_HOLD_MS = 1000  // Longer hold (easier to see)
const PEAK_HOLD_MS = 2000  // Very long (slow music)
```

---

### Peak Decay Rate

```go
const PEAK_DECAY_RATE = 15.0  // units per second
```

**What it does:** How fast peak bar falls after hold time

**Range:** 5.0-50.0

**Examples:**

```go
const PEAK_DECAY_RATE = 5.0   // Slow fall
const PEAK_DECAY_RATE = 15.0  // Default
const PEAK_DECAY_RATE = 30.0  // Fast fall
const PEAK_DECAY_RATE = 50.0  // Very fast fall
```

**Note:** Higher values = peak disappears quickly

---

### Peak Flicker Frequency

```go
const FLICKER_MIN_HZ = 3.0
const FLICKER_MAX_HZ = 5.0
```

**What it does:** Random flicker rate range for peak bar

**Range:** 1.0-10.0 Hz

**Examples:**

```go
// Slow, dramatic flicker
const FLICKER_MIN_HZ = 1.0
const FLICKER_MAX_HZ = 3.0

// Fast, jittery flicker
const FLICKER_MIN_HZ = 8.0
const FLICKER_MAX_HZ = 12.0

// No variation (fixed frequency)
const FLICKER_MIN_HZ = 5.0
const FLICKER_MAX_HZ = 5.0
```

---

### Peak Flicker Opacity

```go
const FLICKER_MIN_OPACITY = 0.3  // Never fully invisible
const FLICKER_MAX_OPACITY = 1.0  // Full brightness
```

**What it does:** Opacity range for peak flicker jumps

**Range:** 0.0-1.0

**Examples:**

```go
// More dramatic (can disappear completely)
const FLICKER_MIN_OPACITY = 0.0
const FLICKER_MAX_OPACITY = 1.0

// Subtle (always visible)
const FLICKER_MIN_OPACITY = 0.7
const FLICKER_MAX_OPACITY = 1.0

// Very subtle
const FLICKER_MIN_OPACITY = 0.8
const FLICKER_MAX_OPACITY = 1.0
```

---

### Peak Brightness Alternation

```go
const FLICKER_DIM_FACTOR = 0.5  // 50% dimmer on alternate frames
```

**What it does:** How much dimmer alternate frames are

**Range:** 0.0-1.0

**Examples:**

```go
const FLICKER_DIM_FACTOR = 1.0   // No alternation (always bright)
const FLICKER_DIM_FACTOR = 0.7   // Subtle alternation
const FLICKER_DIM_FACTOR = 0.5   // Default
const FLICKER_DIM_FACTOR = 0.2   // Dramatic alternation
const FLICKER_DIM_FACTOR = 0.0   // Fully off on alternate frames
```

---

## Gain Control

### Default Gain

```go
const DEFAULT_GAIN = 1.0
```

**What it does:** Initial gain multiplier on startup

**Range:** 0.1-5.0

**Examples:**

```go
const DEFAULT_GAIN = 0.5   // Start quieter
const DEFAULT_GAIN = 1.0   // Default (neutral)
const DEFAULT_GAIN = 1.5   // Start louder
const DEFAULT_GAIN = 2.0   // Much louder
```

---

### Gain Limits

```go
const MIN_GAIN = 0.1
const MAX_GAIN = 5.0
```

**What it does:** Limits for gain adjustment via keyboard

**When to change:**
- Need more gain: increase MAX_GAIN
- Prevent distortion: decrease MAX_GAIN

---

### Gain Step

```go
const GAIN_STEP = 0.1  // Amount to change per keypress
```

**What it does:** How much gain changes with +/- keys

**Examples:**

```go
const GAIN_STEP = 0.05  // Fine control
const GAIN_STEP = 0.1   // Default
const GAIN_STEP = 0.25  // Coarse control
```

---

## Color Schemes

### Default Scheme

```go
const DEFAULT_COLOR_SCHEME = "classic"
```

**What it does:** Which color scheme to use on startup

**Values:**
- `"classic"`: Green → Yellow → Red
- `"synthwave"`: Cyan → Magenta

---

### Color Definitions

**Classic Scheme:**

```go
var CLASSIC_COLORS = []string{
    "#00FF00", "#88FF00",  // Green (0-33%)
    "#FFFF00", "#FF8800",  // Yellow (33-66%)
    "#FF4400", "#FF0000",  // Red (66-100%)
}
```

**Synthwave Scheme:**

```go
var SYNTHWAVE_COLORS = []string{
    "#00AAAA", "#00DDFF",  // Cyan (0-25%)
    "#00DDFF", "#8800FF",  // Cyan-Purple (25-50%)
    "#8800FF", "#DD00DD",  // Purple-Magenta (50-75%)
    "#DD00DD", "#FF00FF",  // Hot Magenta (75-100%)
}
```

**How to add new scheme:**

See `docs/ALGORITHMS.md` section on Color Interpolation for examples.

---

## Performance

### Enable Debug Info

```go
const SHOW_DEBUG_INFO = false
```

**What it does:** Show FPS, CPU usage, buffer stats

**Values:**
- `false`: Clean display (default)
- `true`: Show debug overlay

---

### Frame Skip Threshold

```go
const MAX_FRAME_TIME_MS = 40  // Skip frame if rendering takes >40ms
```

**What it does:** Skip rendering if previous frame took too long

**Purpose:** Prevent spiral of death in slow terminals

---

## Example Configurations

### High Performance (Low CPU)

```go
const TARGET_FPS = 20
const FFT_SIZE = 1024
const SMOOTHING_ALPHA = 0.3
const NUM_BANDS = 8
const ENABLE_A_WEIGHTING = false
```

### Maximum Quality (High CPU)

```go
const TARGET_FPS = 60
const FFT_SIZE = 4096
const SMOOTHING_ALPHA = 0.5
const NUM_BANDS = 32
const ENABLE_A_WEIGHTING = true
```

### Retro Style (8-band classic)

```go
const NUM_BANDS = 8
const BAR_SPACING = 4
const SMOOTHING_ALPHA = 0.2
const DECAY_STYLE = "solid"
const DEFAULT_COLOR_SCHEME = "classic"
```

### Aggressive Cyberpunk

```go
const FLICKER_MIN_HZ = 8.0
const FLICKER_MAX_HZ = 15.0
const FLICKER_MIN_OPACITY = 0.0
const FLICKER_DIM_FACTOR = 0.2
const SMOOTHING_ALPHA = 0.6  // More responsive
```

### Smooth & Calm

```go
const SMOOTHING_ALPHA = 0.2  // Very smooth
const PEAK_HOLD_MS = 1000    // Long hold
const PEAK_DECAY_RATE = 5.0  // Slow fall
const FLICKER_MIN_HZ = 1.0
const FLICKER_MAX_HZ = 2.0   // Gentle flicker
```

---

## How to Modify Configuration

1. **Edit `viz/config.go`**
2. **Change desired constants**
3. **Rebuild:** `go build`
4. **Test:** `./cyberspec`
5. **Iterate as needed**

### Tips

- **Change one thing at a time** to understand impact
- **Keep backups** of working configurations
- **Document your changes** in comments
- **Test with different music** styles

---

*Last Updated: September 1, 2026*
