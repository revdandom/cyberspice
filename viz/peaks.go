package viz

import (
	"math"
	"math/rand"
	"time"
)

// Peak represents a single peak tracker for one frequency band
type Peak struct {
	Height          float64 // Current peak height (0.0-1.0)
	Age             int64   // Time since peak was captured (milliseconds)
	DecayStarted    bool    // Whether decay has started (after hold time)
	Flicker         *PeakFlicker
}

// PeakFlicker handles the "failing light bulb" flicker effect
//
// INSPIRATION: "Light bulb flickering out when it fails"
//
// EFFECT COMPONENTS:
//   1. Random timing: Flicker events at 3-5 Hz (random intervals)
//   2. Opacity jumps: Random opacity changes on flicker events
//   3. Brightness alternation: Every frame alternates bright/dim
//   4. Overall decay: Opacity decreases as peak falls
//
// VISUAL RESULT:
//   - Peak bar stutters and flickers randomly
//   - Gets dimmer overall as it decays
//   - Creates "dying neon sign" or "failing bulb" effect
//   - Enhances cyberpunk aesthetic
//
// See docs/ALGORITHMS.md for detailed algorithm explanation
type PeakFlicker struct {
	BaseOpacity      float64 // Overall opacity trend (1.0 → min as peak decays)
	CurrentOpacity   float64 // Current frame opacity (random jumps)
	LastFlickerTime  int64   // Time since last flicker event (ms)
	NextFlickerIn    int64   // Random interval to next flicker (ms)
	BrightFrame      bool    // Alternates each frame (bright/dim effect)
	TimeSincePeak    int64   // Total time since peak was captured (ms)
}

// PeakTracker tracks peaks for all frequency bands
type PeakTracker struct {
	peaks []Peak
	rng   *rand.Rand
}

// NewPeakTracker creates a new peak tracker
//
// Parameters:
//   numBands - Number of frequency bands to track
//
// Returns:
//   *PeakTracker - Initialized tracker
func NewPeakTracker(numBands int) *PeakTracker {
	// Seed random number generator for flicker timing
	source := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(source)

	pt := &PeakTracker{
		peaks: make([]Peak, numBands),
		rng:   rng,
	}

	// Initialize each peak
	for i := range pt.peaks {
		pt.peaks[i] = Peak{
			Height:       0.0,
			Age:          0,
			DecayStarted: false,
			Flicker:      newPeakFlicker(rng),
		}
	}

	return pt
}

// newPeakFlicker creates a new flicker instance
func newPeakFlicker(rng *rand.Rand) *PeakFlicker {
	return &PeakFlicker{
		BaseOpacity:     1.0,
		CurrentOpacity:  1.0,
		LastFlickerTime: 0,
		NextFlickerIn:   randomFlickerInterval(rng),
		BrightFrame:     true,
		TimeSincePeak:   0,
	}
}

// Update updates all peaks based on current band magnitudes
//
// ALGORITHM FOR EACH BAND:
//   1. If current > peak: Capture new peak, reset flicker
//   2. If holding: Age peak, flicker but don't move
//   3. If decaying: Age peak, flicker, and move down
//
// Parameters:
//   currentMagnitudes - Current frame's band magnitudes (0.0-1.0)
//   deltaMs           - Time since last update (milliseconds)
func (pt *PeakTracker) Update(currentMagnitudes []float64, deltaMs int64) {
	for i := range pt.peaks {
		peak := &pt.peaks[i]
		current := currentMagnitudes[i]

		// Check if new peak captured
		if current > peak.Height {
			// New peak! Reset everything
			peak.Height = current
			peak.Age = 0
			peak.DecayStarted = false
			peak.Flicker = newPeakFlicker(pt.rng)
			continue
		}

		// Age the peak
		peak.Age += deltaMs
		peak.Flicker.TimeSincePeak += deltaMs

		// Check if hold time expired
		if !peak.DecayStarted && peak.Age >= PEAK_HOLD_MS {
			peak.DecayStarted = true
		}

		// Decay if hold time expired.
		//
		// Per-frame exponential decay, same model as the bar smoother.
		// PEAK_FALLOFF_WEIGHT > BAR_FALLOFF_WEIGHT guarantees a falling peak
		// sheds a smaller fraction of its height each frame than the bar, so
		// it can never descend onto a falling bar (see config.go).
		if peak.DecayStarted {
			peak.Height *= PEAK_FALLOFF_WEIGHT

			// Never below the live level — lets the peak rejoin a sustained
			// or rising bar instead of floating forever.
			if peak.Height < current {
				peak.Height = current
			}
			if peak.Height < 0.0 {
				peak.Height = 0.0
			}
		}

		// Update flicker effect
		peak.Flicker.Update(deltaMs, pt.rng)
	}
}

// GetPeakHeight returns the current peak height for a band
func (pt *PeakTracker) GetPeakHeight(bandIndex int) float64 {
	if bandIndex < 0 || bandIndex >= len(pt.peaks) {
		return 0.0
	}
	return pt.peaks[bandIndex].Height
}

// GetPeakOpacity returns the current flicker opacity for a band's peak
func (pt *PeakTracker) GetPeakOpacity(bandIndex int) float64 {
	if bandIndex < 0 || bandIndex >= len(pt.peaks) {
		return 0.0
	}
	return pt.peaks[bandIndex].Flicker.GetOpacity()
}

// Update updates the flicker effect
//
// FLICKER ALGORITHM:
//
// 1. Update base opacity (decreases over time)
//    - Uses exponential decay curve
//    - Faster decay near end (OPACITY_DECAY_EXPONENT)
//
// 2. Check for flicker event (random timing)
//    - If time >= next flicker interval:
//      a. Jump current opacity to random value
//      b. Bias toward lower values as base decays
//      c. Set new random interval (200-333ms for 3-5 Hz)
//
// 3. Alternate brightness every frame
//    - Bright frame: Use current opacity
//    - Dim frame: Multiply by FLICKER_DIM_FACTOR (0.5 = 50% dimmer)
//
// VISUAL TIMELINE EXAMPLE:
//   0ms:    Bright (opacity 1.0)
//   33ms:   Dim    (opacity 0.5)
//   66ms:   Bright (opacity 1.0)
//   [flicker at 200ms]
//   200ms:  Dim    (opacity 0.3) - jumped to 0.6, dimmed to 0.3
//   233ms:  Bright (opacity 0.6)
//   266ms:  Dim    (opacity 0.3)
//   [flicker at 520ms]
//   520ms:  Bright (opacity 0.9) - jumped back up
//   ...continues until fully decayed
//
// Parameters:
//   deltaMs - Time since last update (milliseconds)
//   rng     - Random number generator
func (pf *PeakFlicker) Update(deltaMs int64, rng *rand.Rand) {
	// Update base opacity (overall decay trend)
	// Use exponential curve for more dramatic fade near end
	decayProgress := float64(pf.TimeSincePeak) / 3000.0 // 3 seconds to fully decay
	if decayProgress > 1.0 {
		decayProgress = 1.0
	}

	// Apply exponential curve
	decayProgress = math.Pow(decayProgress, OPACITY_DECAY_EXPONENT)

	// Calculate base opacity (1.0 → FLICKER_MIN_OPACITY)
	opacityRange := FLICKER_MAX_OPACITY - FLICKER_MIN_OPACITY
	pf.BaseOpacity = FLICKER_MAX_OPACITY - (decayProgress * opacityRange)

	// Clamp to valid range
	if pf.BaseOpacity < FLICKER_MIN_OPACITY {
		pf.BaseOpacity = FLICKER_MIN_OPACITY
	}

	// Check for flicker event (random timing)
	pf.LastFlickerTime += deltaMs
	if pf.LastFlickerTime >= pf.NextFlickerIn {
		// Flicker event! Jump to random opacity
		// Bias toward lower values as base opacity decreases
		availableRange := pf.BaseOpacity - FLICKER_MIN_OPACITY
		randomFactor := rng.Float64()

		pf.CurrentOpacity = FLICKER_MIN_OPACITY + (randomFactor * availableRange)

		// Set next random flicker interval (3-5 Hz)
		pf.NextFlickerIn = randomFlickerInterval(rng)
		pf.LastFlickerTime = 0
	}

	// Alternate brightness every frame
	pf.BrightFrame = !pf.BrightFrame
}

// GetOpacity returns the current opacity for rendering
//
// Returns:
//   float64 - Opacity value (0.0 to 1.0)
func (pf *PeakFlicker) GetOpacity() float64 {
	if pf.BrightFrame {
		// Bright frame: use current opacity as-is
		return pf.CurrentOpacity
	} else {
		// Dim frame: multiply by dim factor
		return pf.CurrentOpacity * FLICKER_DIM_FACTOR
	}
}

// randomFlickerInterval returns a random flicker interval in milliseconds
//
// Flicker frequency: FLICKER_MIN_HZ to FLICKER_MAX_HZ
// Convert Hz to milliseconds: 1000 / Hz
//
// Example: 3-5 Hz
//   Min interval: 1000 / 5 = 200 ms
//   Max interval: 1000 / 3 = 333 ms
//
// Parameters:
//   rng - Random number generator
//
// Returns:
//   int64 - Random interval in milliseconds
func randomFlickerInterval(rng *rand.Rand) int64 {
	minMs := 1000.0 / FLICKER_MAX_HZ
	maxMs := 1000.0 / FLICKER_MIN_HZ

	intervalMs := minMs + rng.Float64()*(maxMs-minMs)
	return int64(intervalMs)
}

// HOW TO MODIFY FLICKER BEHAVIOR:
//
// See viz/config.go for all configurable constants:
//
// 1. Flicker frequency:
//    const FLICKER_MIN_HZ = 5.0  // Faster
//    const FLICKER_MAX_HZ = 10.0
//
// 2. Opacity range:
//    const FLICKER_MIN_OPACITY = 0.0  // Can go fully invisible
//    const FLICKER_MAX_OPACITY = 1.0
//
// 3. Brightness alternation:
//    const FLICKER_DIM_FACTOR = 0.2  // More dramatic (80% dimmer)
//    const FLICKER_DIM_FACTOR = 1.0  // No alternation
//
// 4. Decay curve:
//    const OPACITY_DECAY_EXPONENT = 2.0  // Faster fade near end
//    const OPACITY_DECAY_EXPONENT = 1.0  // Linear decay
//
// ALTERNATIVE FLICKER PATTERNS:
//
// 1. Regular pulse (no randomness):
//    func (pf *PeakFlicker) Update(deltaMs int64, rng *rand.Rand) {
//        // Fixed frequency sine wave
//        freq := 5.0 // Hz
//        phase := float64(pf.TimeSincePeak) / 1000.0 * freq * 2.0 * math.Pi
//        pf.CurrentOpacity = 0.5 + 0.5*math.Sin(phase)
//    }
//
// 2. Smoother random (Perlin noise):
//    // Requires noise library
//    noise := perlin.Noise1D(float64(pf.TimeSincePeak) / 100.0)
//    pf.CurrentOpacity = 0.5 + 0.5*noise
//
// 3. Stepped flicker (on/off, no gradual):
//    if rng.Float64() < 0.3 {
//        pf.CurrentOpacity = 0.0  // Off
//    } else {
//        pf.CurrentOpacity = 1.0  // On
//    }
//
// See docs/ALGORITHMS.md for more examples
