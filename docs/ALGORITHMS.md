# CyberSpec - Algorithm Documentation

This document provides detailed technical explanations of all algorithms used in CyberSpec, including rationale, implementation details, and how to modify them.

---

## Table of Contents

1. [A-Weighting Curve](#a-weighting-curve)
2. [Decay Algorithms](#decay-algorithms)
3. [Peak Flicker Algorithm](#peak-flicker-algorithm)
4. [Temporal Smoothing](#temporal-smoothing)
5. [Frequency Band Calculation](#frequency-band-calculation)
6. [Color Interpolation](#color-interpolation)

---

## A-Weighting Curve

### Purpose

The A-weighting curve solves the "bass-heavy visualization" problem common in spectrum analyzers.

### The Problem

Raw FFT magnitude values are heavily biased toward low frequencies because:

1. **Physical energy distribution:** Bass frequencies carry more physical energy in typical music
2. **Musical mixing:** Music is often mixed with emphasized bass for impact
3. **FFT characteristics:** Lower frequency bins accumulate more energy
4. **Human perception mismatch:** Our ears don't perceive loudness linearly across frequencies

Without correction, a spectrum analyzer shows:
```
Bass:  ████████████████████  (Dominates)
Mids:  ████
Highs: █
```

This doesn't match what we *hear* - the mids and highs are audible but visually underrepresented.

### The Solution: A-Weighting

A-weighting is an industry-standard frequency weighting curve used in sound level meters. It models human ear sensitivity across frequencies.

**Effect:**
- Reduces bass (<1 kHz) by ~30-40 dB
- Slightly boosts presence range (2-5 kHz)  
- Reduces very high frequencies (>10 kHz) slightly
- Results in visually balanced representation that matches perceived loudness

### Mathematical Formula

```
A(f) = 20 × log₁₀(
  (12194² × f⁴) / 
  ((f² + 20.6²) × √((f² + 107.7²) × (f² + 737.9²)) × (f² + 12194²))
)
```

Where:
- `f` = frequency in Hz
- Result is in dB (decibels)
- Convert dB to linear multiplier: `10^(A(f)/20)`

### Implementation

**File:** `dsp/weighting.go`

```go
// Pre-calculate A-weighting multipliers for all FFT bins
func calculateAWeighting(sampleRate, fftSize int) []float64 {
    weights := make([]float64, fftSize/2)
    
    for i := 0; i < fftSize/2; i++ {
        freq := float64(i) * float64(sampleRate) / float64(fftSize)
        
        if freq < 1.0 {
            weights[i] = 0.0 // Avoid division by zero
            continue
        }
        
        // A-weighting formula
        f2 := freq * freq
        f4 := f2 * f2
        
        numerator := 12194.0 * 12194.0 * f4
        denominator := (f2 + 20.6*20.6) * 
                       math.Sqrt((f2 + 107.7*107.7) * (f2 + 737.9*737.9)) * 
                       (f2 + 12194.0*12194.0)
        
        aWeight := 20.0 * math.Log10(numerator / denominator)
        
        // Convert dB to linear multiplier
        weights[i] = math.Pow(10.0, aWeight/20.0)
    }
    
    return weights
}

// Apply during magnitude calculation
magnitude := math.Sqrt(real*real + imag*imag) * aWeighting[bin]
```

### Configuration

**Enable/Disable:** `viz/config.go`

```go
// Set to false to see raw FFT magnitudes without A-weighting
const ENABLE_A_WEIGHTING = true
```

### Alternative Curves

**Not implemented but documented for future use:**

#### C-Weighting
- More neutral than A-weighting
- Less bass reduction
- Used for louder sounds
- Formula similar to A-weighting but different constants

#### Equal-Loudness Contours (ISO 226)
- Most accurate to human perception
- Based on Fletcher-Munson curves
- More complex to implement
- Requires lookup tables or complex formulas

#### Custom Per-Band Scaling
- Simple multipliers per frequency band
- Easy to tweak by ear
- Example: `[bass:0.3, mids:1.0, highs:2.5]`
- Less scientifically accurate but more intuitive

### How to Modify

**Adjust the curve strength:**

```go
// Make A-weighting more aggressive
weights[i] = math.Pow(10.0, aWeight/15.0)  // Divide by 15 instead of 20

// Make A-weighting gentler
weights[i] = math.Pow(10.0, aWeight/25.0)  // Divide by 25
```

**Implement custom curve:**

```go
func customWeighting(freq float64) float64 {
    switch {
    case freq < 100:
        return 0.2  // Heavily reduce sub-bass
    case freq < 500:
        return 0.4  // Reduce bass
    case freq < 2000:
        return 1.0  // Keep mids neutral
    case freq < 8000:
        return 1.5  // Boost presence
    default:
        return 1.2  // Slightly boost highs
    }
}
```

---

## Decay Algorithms

### Overview

CyberSpec implements a unique "Fibonacci gap spacing" decay pattern inspired by cyberpunk aesthetics. As frequency bars lose energy, they fragment into horizontal segments with increasing vertical gaps.

### Implemented: Fibonacci Gap Spacing (Option C)

**Visual Reference:** See `docs/decay-inspiration.png`

**Concept:** Bars fragment into segments with gaps that grow following Fibonacci sequence.

#### Energy → Gap Mapping

```
Energy Level    Gap Size    Segment Height    Visual Effect
─────────────────────────────────────────────────────────────
100-90%         0 lines     Full height       Solid bar
 90-70%         1 line      2-3 lines         Slight breaks
 70-50%         2 lines     2 lines           Clear segments
 50-30%         3 lines     1-2 lines         Scattered pieces
 30-15%         5 lines     1 line            Sparse dashes
 15-5%          8 lines     1 line            Barely visible
  5-0%          13 lines    Fading            Nearly gone
```

#### Implementation

**File:** `viz/renderer.go`

```go
// Configuration map (in viz/config.go)
var DECAY_THRESHOLDS = map[int]int{
    90: 0,   // Solid
    70: 1,   // 1 line gap
    50: 2,   // 2 line gap
    30: 3,   // 3 line gap
    15: 5,   // 5 line gap
    5:  8,   // 8 line gap
    0:  13,  // 13 line gap
}

var SEGMENT_HEIGHTS = map[int]int{
    90: 999, // Solid (full height)
    70: 3,   // 3-line segments
    50: 2,   // 2-line segments
    30: 2,   // 2-line segments
    15: 1,   // 1-line segments
    5:  1,   // 1-line dashes
}

// Rendering function
func renderBarWithDecay(height int, energy float64) []string {
    energyPercent := int(energy * 100)
    
    // Determine gap size and segment height based on energy
    gapSize := getGapForEnergy(energyPercent)
    segmentHeight := getSegmentHeightForEnergy(energyPercent)
    
    if gapSize == 0 {
        // Solid bar - no fragmentation
        return renderSolidBar(height, energy)
    }
    
    // Fragmented bar
    lines := make([]string, height)
    position := 0
    
    for position < height {
        // Draw segment
        for i := 0; i < segmentHeight && position < height; i++ {
            lines[position] = getCharForEnergy(energy)
            position++
        }
        
        // Add gap
        for i := 0; i < gapSize && position < height; i++ {
            lines[position] = "  " // Empty space
            position++
        }
    }
    
    return lines
}

// Character selection by energy level
func getCharForEnergy(energy float64) string {
    switch {
    case energy > 0.7:
        return "██"  // Full block
    case energy > 0.4:
        return "▄▄"  // Half block
    case energy > 0.2:
        return "▁▁"  // Thin block
    default:
        return "─"   // Dash
    }
}
```

### Alternative Decay Patterns (Documented, Not Implemented)

These patterns were discussed during design but not implemented in the POC. They are documented here for future experimentation.

#### Option A: Fixed Segments, Growing Gaps

**Concept:** Fixed number of segments, gaps between them grow.

```
100%: ████  75%: ████  50%: ████  25%: ████
      ████       
      ████       ████       
      ████              
      ████       ████       ████       ████
```

**Implementation approach:**
```go
numSegments := 4  // Fixed
gapSize := int((1.0 - energy) * 10)  // Grows as energy decreases
segmentHeight := height / (numSegments + (numSegments-1)*gapSize)
```

#### Option B: Reducing Segments

**Concept:** Number of segments decreases as energy drops.

```
Fibonacci segments: 8, 5, 3, 2, 1

100%: 8 segments (solid)
 60%: 5 segments
 40%: 3 segments
 20%: 2 segments
 10%: 1 segment
```

**Implementation approach:**
```go
var fibSegments = []int{8, 5, 3, 2, 1}
index := int((1.0 - energy) * float64(len(fibSegments)))
numSegments := fibSegments[index]
```

#### Option C: Solid Decay (Traditional)

**Concept:** Simple solid bar that shrinks without fragmentation.

```go
func renderSolidBar(height int, energy float64) []string {
    lines := make([]string, height)
    for i := 0; i < height; i++ {
        lines[i] = "██"
    }
    return lines
}
```

### How to Switch Decay Patterns

**File:** `viz/config.go`

```go
const DECAY_STYLE = "fibonacci_gaps"  // Current implementation

// To implement alternatives, add:
// const DECAY_STYLE = "fixed_segments"
// const DECAY_STYLE = "reducing_segments"  
// const DECAY_STYLE = "solid"

// Then in renderer.go, switch based on DECAY_STYLE
```

### Customizing Fibonacci Decay

**Adjust gap progression:**

```go
// More aggressive fragmentation
var DECAY_THRESHOLDS = map[int]int{
    95: 0,   // Fragment earlier
    80: 1,
    60: 2,
    40: 3,
    20: 5,
    10: 8,
    0:  13,
}

// Gentler fragmentation
var DECAY_THRESHOLDS = map[int]int{
    95: 0,   // Stay solid longer
    85: 1,
    75: 2,
    50: 3,
    25: 5,
    10: 8,
    0:  13,
}
```

**Use different sequence:**

```go
// Powers of 2 instead of Fibonacci
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

## Peak Flicker Algorithm

### Concept

Peak bars flicker like a "failing light bulb" - random stuttering with overall decay in brightness.

### Inspiration

The effect mimics:
- Fluorescent light failing
- Neon sign flickering out
- Glitching hologram
- Dying LED

### Algorithm Components

#### 1. Opacity Layers

**Two levels of opacity:**

1. **Base Opacity:** Overall trend (1.0 → 0.3 as peak decays)
2. **Current Opacity:** Per-frame value (random jumps within range)

#### 2. Random Timing

**Flicker frequency:** 3-5 Hz (random)

```go
minInterval := 1000.0 / FLICKER_MAX_HZ  // 200ms
maxInterval := 1000.0 / FLICKER_MIN_HZ  // 333ms
interval := random(minInterval, maxInterval)
```

#### 3. Brightness Alternation

**Every frame alternates:**
- **Bright frame:** Color at current opacity
- **Dim frame:** Color at current opacity × 0.5

This creates stuttering effect even when opacity isn't changing.

### Implementation

**File:** `viz/peaks.go`

```go
type PeakFlicker struct {
    BaseOpacity      float64  // Overall opacity (1.0 → min as it decays)
    CurrentOpacity   float64  // Current frame opacity (for random jumps)
    LastFlickerTime  int64    // Milliseconds since last flicker event
    NextFlickerIn    int      // Random interval to next flicker (ms)
    BrightFrame      bool     // Alternates every frame
    TimeSincePeak    int64    // Time elapsed since peak was captured
}

func (p *PeakFlicker) Update(deltaMs int64) {
    // Update base opacity (decreases over time)
    decayProgress := float64(p.TimeSincePeak) / float64(PEAK_DECAY_TIME_MS)
    p.BaseOpacity = 1.0 - (decayProgress * (1.0 - FLICKER_MIN_OPACITY))
    
    // Clamp to valid range
    if p.BaseOpacity < FLICKER_MIN_OPACITY {
        p.BaseOpacity = FLICKER_MIN_OPACITY
    }
    
    // Check if it's time for a flicker event
    p.LastFlickerTime += deltaMs
    if p.LastFlickerTime >= int64(p.NextFlickerIn) {
        // Trigger flicker: jump to random opacity
        // Bias toward lower values as base opacity decreases
        opacityRange := p.BaseOpacity - FLICKER_MIN_OPACITY
        randomFactor := rand.Float64()
        p.CurrentOpacity = FLICKER_MIN_OPACITY + (randomFactor * opacityRange)
        
        // Random interval to next flicker
        p.NextFlickerIn = randomInterval(FLICKER_MIN_HZ, FLICKER_MAX_HZ)
        p.LastFlickerTime = 0
    }
    
    // Alternate brightness every frame
    p.BrightFrame = !p.BrightFrame
}

func (p *PeakFlicker) GetOpacity() float64 {
    if p.BrightFrame {
        return p.CurrentOpacity
    } else {
        return p.CurrentOpacity * FLICKER_DIM_FACTOR
    }
}

func randomInterval(minHz, maxHz float64) int {
    minMs := 1000.0 / maxHz
    maxMs := 1000.0 / minHz
    return int(minMs + rand.Float64()*(maxMs-minMs))
}
```

### Configuration

**File:** `viz/config.go`

```go
// Flicker frequency range (Hz)
const FLICKER_MIN_HZ = 3.0
const FLICKER_MAX_HZ = 5.0

// Opacity range
const FLICKER_MIN_OPACITY = 0.3  // Never fully invisible
const FLICKER_MAX_OPACITY = 1.0  // Full brightness

// Brightness alternation
const FLICKER_DIM_FACTOR = 0.5  // 50% dimmer on alternate frames

// Opacity decay curve
const OPACITY_DECAY_EXPONENT = 1.5  // Higher = faster fade near end

// Peak decay time
const PEAK_DECAY_TIME_MS = 3000  // 3 seconds to fully decay
```

### Visual Timeline Example

```
Time    Base    Current   Bright?   Visual
        Opacity Opacity   Frame     Result
─────────────────────────────────────────────
0ms     1.0     1.0       Yes       ━━━━ (Full bright)
33ms    1.0     1.0       No        ━━━━ (Dimmed)
66ms    1.0     1.0       Yes       ━━━━ (Full bright)
[flicker event at 200ms]
200ms   0.95    0.6       No            (Jump to 0.6, dimmed = barely visible)
233ms   0.95    0.6       Yes       ━━━━ (0.6 opacity)
266ms   0.95    0.6       No        ━━━━ (Dimmed)
[flicker event at 520ms]
520ms   0.85    0.9       Yes       ━━━━ (Jump back up)
553ms   0.85    0.9       No        ━━━━ (Dimmed)
[continues...]
```

### How to Modify

**Faster/slower flicker:**

```go
const FLICKER_MIN_HZ = 5.0  // Faster
const FLICKER_MAX_HZ = 10.0
```

**Less random (more regular):**

```go
// Use fixed interval instead of random
p.NextFlickerIn = 200  // Fixed 5 Hz
```

**Smoother fade (less jumpy):**

```go
// Instead of random jumps, use smooth decay
p.CurrentOpacity = p.BaseOpacity * (0.7 + 0.3*math.Sin(time))
```

**More dramatic flicker:**

```go
const FLICKER_MIN_OPACITY = 0.0  // Can go fully invisible
const FLICKER_DIM_FACTOR = 0.2   // Much dimmer on alternate frames
```

---

## Temporal Smoothing

### Purpose

Reduce visual jitter and create smooth bar motion.

### The Problem

Raw FFT magnitudes can fluctuate rapidly frame-to-frame due to:
- Transients in audio
- Noise in FFT
- Phase variations
- Spectral leakage

This causes bars to "jitter" or "jump" unnaturally.

### Solution: Exponential Moving Average (EMA)

EMA smooths values over time while still being responsive to changes.

### Formula

```
smoothed[i] = (α × current[i]) + ((1 - α) × previous[i])
```

Where:
- `α` (alpha) = smoothing factor (0.0 to 1.0)
- Higher α = more responsive, less smooth
- Lower α = smoother, more lag

### Implementation

**File:** `viz/smooth.go`

```go
type Smoother struct {
    previousValues []float64
    alpha         float64
}

func NewSmoother(numBands int, alpha float64) *Smoother {
    return &Smoother{
        previousValues: make([]float64, numBands),
        alpha:         alpha,
    }
}

func (s *Smoother) Smooth(current []float64) []float64 {
    smoothed := make([]float64, len(current))
    
    for i := range current {
        smoothed[i] = (s.alpha * current[i]) + 
                      ((1.0 - s.alpha) * s.previousValues[i])
        s.previousValues[i] = smoothed[i]
    }
    
    return smoothed
}
```

### Configuration

**File:** `viz/config.go`

```go
// Smoothing factor (0.0 = max smoothing, 1.0 = no smoothing)
const SMOOTHING_ALPHA = 0.4  // Medium (default)

// Documented alternatives:
// const SMOOTHING_ALPHA = 1.0   // None - raw, responsive, jittery
// const SMOOTHING_ALPHA = 0.7   // Light - slight smoothing
// const SMOOTHING_ALPHA = 0.4   // Medium - balanced (recommended)
// const SMOOTHING_ALPHA = 0.2   // Heavy - very smooth, noticeable lag
```

### Visual Effect Comparison

```
Alpha   Responsiveness   Smoothness   Best For
──────────────────────────────────────────────────────
1.0     Instant          None         Debugging, raw data
0.7     Very fast        Slight       Fast-paced music (EDM, metal)
0.4     Balanced         Medium       Most music (default)
0.2     Slow             Very smooth  Ambient, slow music
0.1     Very slow        Maximum      Demo/presentation mode
```

### How to Modify

**Per-band smoothing:**

```go
// Different smoothing for different frequency ranges
func getSmoothingForBand(bandIndex int) float64 {
    if bandIndex < 4 {
        return 0.3  // More smoothing for bass
    } else if bandIndex < 12 {
        return 0.4  // Medium for mids
    } else {
        return 0.6  // Less smoothing for highs (more responsive)
    }
}
```

**Attack/Release envelope:**

```go
// Fast attack, slow release (like audio compressor)
func (s *Smoother) SmoothWithEnvelope(current []float64) []float64 {
    smoothed := make([]float64, len(current))
    
    for i := range current {
        if current[i] > s.previousValues[i] {
            // Attack: respond quickly to increases
            alpha := 0.7
            smoothed[i] = (alpha * current[i]) + ((1.0 - alpha) * s.previousValues[i])
        } else {
            // Release: respond slowly to decreases
            alpha := 0.2
            smoothed[i] = (alpha * current[i]) + ((1.0 - alpha) * s.previousValues[i])
        }
        s.previousValues[i] = smoothed[i]
    }
    
    return smoothed
}
```

---

## Frequency Band Calculation

### Why Logarithmic Distribution?

Human hearing perceives frequencies logarithmically:
- We can distinguish 100 Hz from 200 Hz (100 Hz difference)
- But can't distinguish 10,100 Hz from 10,200 Hz (same 100 Hz difference)
- Octaves are logarithmic: each doubling of frequency is one octave

Linear distribution would waste resolution in high frequencies and lack detail in bass.

### Implementation

**File:** `dsp/bands.go`

```go
const NUM_BANDS = 16
const MIN_FREQ = 20.0    // Hz
const MAX_FREQ = 20000.0 // Hz

// Calculate logarithmic frequency boundaries
func calculateBandBoundaries() []float64 {
    boundaries := make([]float64, NUM_BANDS+1)
    
    logMin := math.Log10(MIN_FREQ)
    logMax := math.Log10(MAX_FREQ)
    logRange := logMax - logMin
    
    for i := 0; i <= NUM_BANDS; i++ {
        logFreq := logMin + (float64(i) / float64(NUM_BANDS) * logRange)
        boundaries[i] = math.Pow(10, logFreq)
    }
    
    return boundaries
}

// Map FFT bins to bands
func mapFFTToBands(fftMagnitudes []float64, sampleRate, fftSize int) []float64 {
    bands := make([]float64, NUM_BANDS)
    boundaries := calculateBandBoundaries()
    
    for i := 0; i < NUM_BANDS; i++ {
        minFreq := boundaries[i]
        maxFreq := boundaries[i+1]
        
        // Find FFT bins in this frequency range
        minBin := int(minFreq * float64(fftSize) / float64(sampleRate))
        maxBin := int(maxFreq * float64(fftSize) / float64(sampleRate))
        
        // Average magnitudes in this range
        sum := 0.0
        count := 0
        for bin := minBin; bin < maxBin && bin < len(fftMagnitudes); bin++ {
            sum += fftMagnitudes[bin]
            count++
        }
        
        if count > 0 {
            bands[i] = sum / float64(count)
        }
    }
    
    return bands
}
```

### How to Change Number of Bands

**File:** `viz/config.go`

```go
const NUM_BANDS = 16  // Current

// Examples:
// const NUM_BANDS = 8   // Classic, less detail
// const NUM_BANDS = 32  // More detail, narrower bars
// const NUM_BANDS = 64  // Maximum detail
```

**Note:** Changing NUM_BANDS requires adjusting:
- Bar spacing calculation in renderer
- Terminal width requirements
- Possibly color gradients

---

## Color Interpolation

### Purpose

Smooth color transitions based on bar height.

### Implementation

**File:** `viz/colors.go`

```go
// Interpolate between two hex colors
func interpolateColor(color1, color2 string, factor float64) lipgloss.Color {
    // Parse hex colors
    r1, g1, b1 := parseHex(color1)
    r2, g2, b2 := parseHex(color2)
    
    // Linear interpolation
    r := uint8(float64(r1) + factor*float64(r2-r1))
    g := uint8(float64(g1) + factor*float64(g2-g1))
    b := uint8(float64(b1) + factor*float64(b2-b1))
    
    return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
}

// Get color for height percentage in Classic scheme
func ClassicColor(heightPercent float64) lipgloss.Color {
    switch {
    case heightPercent < 0.33:
        return interpolateColor("#00FF00", "#88FF00", heightPercent/0.33)
    case heightPercent < 0.66:
        return interpolateColor("#FFFF00", "#FF8800", (heightPercent-0.33)/0.33)
    default:
        return interpolateColor("#FF4400", "#FF0000", (heightPercent-0.66)/0.34)
    }
}
```

### Adding New Color Schemes

```go
// File: viz/colors.go

func VaporwaveColor(heightPercent float64) lipgloss.Color {
    switch {
    case heightPercent < 0.5:
        return interpolateColor("#FF71CE", "#01CDFE", heightPercent/0.5)
    default:
        return interpolateColor("#01CDFE", "#B967FF", (heightPercent-0.5)/0.5)
    }
}

func MatrixColor(heightPercent float64) lipgloss.Color {
    // All green, varying intensity
    intensity := uint8(100 + heightPercent*155)
    return lipgloss.Color(fmt.Sprintf("#00%02x00", intensity))
}
```

---

*Last Updated: September 1, 2026*
