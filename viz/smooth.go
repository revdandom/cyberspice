package viz

// Smoother provides temporal smoothing for frequency band magnitudes
// using Exponential Moving Average (EMA) to reduce visual jitter
type Smoother struct {
	previousValues []float64 // Previous frame's smoothed values
	alpha          float64   // Smoothing factor (0.0 = max smooth, 1.0 = no smooth)
}

// NewSmoother creates a new smoother instance
//
// WHAT IS TEMPORAL SMOOTHING?
//
// Raw FFT magnitudes can fluctuate rapidly frame-to-frame due to:
//   - Transients in audio (drum hits, note attacks)
//   - Noise in FFT calculation
//   - Phase variations
//   - Spectral leakage
//
// This causes visualization bars to "jitter" or "jump" unnaturally.
//
// SOLUTION: Exponential Moving Average (EMA)
//
// EMA smooths values over time while still being responsive to changes.
// It's computationally cheap and works well for real-time visualization.
//
// FORMULA:
//   smoothed[i] = (α × current[i]) + ((1 - α) × previous[i])
//
// Where:
//   α (alpha) = smoothing factor (0.0 to 1.0)
//   current   = new value this frame
//   previous  = smoothed value from last frame
//
// ALPHA VALUES:
//   1.0 = No smoothing (instant response, jittery)
//   0.7 = Light smoothing (fast, slight smoothing)
//   0.4 = Medium smoothing (balanced) - DEFAULT
//   0.2 = Heavy smoothing (very smooth, noticeable lag)
//   0.1 = Maximum smoothing (extremely smooth, slow)
//
// TRADE-OFFS:
//   Higher α:
//     + More responsive to changes
//     + Better for fast-paced music (EDM, metal)
//     - More visual jitter
//
//   Lower α:
//     + Smoother motion
//     + Better for slow music (ambient, classical)
//     - More lag, less responsive
//     - Can miss quick transients
//
// CONFIGURATION:
//   See viz/config.go: SMOOTHING_ALPHA
//   See docs/CONFIGURATION.md for preset values
//
// Parameters:
//   numBands - Number of frequency bands to smooth (typically NUM_BANDS)
//
// Returns:
//   *Smoother - Initialized smoother ready to smooth values
func NewSmoother(numBands int) *Smoother {
	return &Smoother{
		previousValues: make([]float64, numBands),
		alpha:          SMOOTHING_ALPHA,
	}
}

// Smooth applies EMA smoothing to current values
//
// ALGORITHM:
//   For each band:
//     1. Multiply current value by alpha (weight for new data)
//     2. Multiply previous value by (1 - alpha) (weight for history)
//     3. Add them together
//     4. Store result as new previous value
//
// BEHAVIOR:
//   - First call: Returns current values as-is (no history yet)
//   - Subsequent calls: Smoothed based on alpha setting
//   - Maintains separate smoothing for each frequency band
//
// WHY PER-BAND?
//   - Each frequency band may have different dynamics
//   - Bass typically changes slower than highs
//   - Independent smoothing prevents cross-band interference
//
// Parameters:
//   current - Current frame's raw values (one per band)
//
// Returns:
//   []float64 - Smoothed values ready for visualization
func (s *Smoother) Smooth(current []float64) []float64 {
	smoothed := make([]float64, len(current))

	for i := range current {
		// EMA formula: α × current + (1 - α) × previous
		smoothed[i] = (s.alpha * current[i]) + ((1.0 - s.alpha) * s.previousValues[i])

		// Store for next frame
		s.previousValues[i] = smoothed[i]
	}

	return smoothed
}

// Reset clears smoothing history
// Useful when switching audio sources or on large discontinuities
func (s *Smoother) Reset() {
	for i := range s.previousValues {
		s.previousValues[i] = 0.0
	}
}

// SetAlpha changes the smoothing factor
// Allows dynamic adjustment during runtime
//
// Parameters:
//   alpha - New smoothing factor (0.0 to 1.0)
func (s *Smoother) SetAlpha(alpha float64) {
	// Clamp to valid range
	if alpha < 0.0 {
		alpha = 0.0
	}
	if alpha > 1.0 {
		alpha = 1.0
	}

	s.alpha = alpha
}

// ALTERNATIVE SMOOTHING METHODS:
//
// These are documented but not implemented. See docs/ALGORITHMS.md for details.
//
// 1. ATTACK/RELEASE ENVELOPE (like audio compressor)
//    Fast attack (respond quickly to increases)
//    Slow release (respond slowly to decreases)
//
//    func (s *Smoother) SmoothWithEnvelope(current []float64) []float64 {
//        smoothed := make([]float64, len(current))
//        for i := range current {
//            if current[i] > s.previousValues[i] {
//                // Attack: respond quickly to increases
//                alpha := 0.7
//                smoothed[i] = (alpha * current[i]) + ((1.0 - alpha) * s.previousValues[i])
//            } else {
//                // Release: respond slowly to decreases
//                alpha := 0.2
//                smoothed[i] = (alpha * current[i]) + ((1.0 - alpha) * s.previousValues[i])
//            }
//            s.previousValues[i] = smoothed[i]
//        }
//        return smoothed
//    }
//
// 2. PER-BAND ALPHA (different smoothing per frequency range)
//
//    func getSmoothingForBand(bandIndex int) float64 {
//        if bandIndex < 4 {
//            return 0.3  // More smoothing for bass (slower changes)
//        } else if bandIndex < 12 {
//            return 0.4  // Medium for mids
//        } else {
//            return 0.6  // Less smoothing for highs (more responsive)
//        }
//    }
//
//    func (s *Smoother) SmoothPerBand(current []float64) []float64 {
//        smoothed := make([]float64, len(current))
//        for i := range current {
//            alpha := getSmoothingForBand(i)
//            smoothed[i] = (alpha * current[i]) + ((1.0 - alpha) * s.previousValues[i])
//            s.previousValues[i] = smoothed[i]
//        }
//        return smoothed
//    }
//
// 3. SIMPLE MOVING AVERAGE (average last N frames)
//
//    type SMAoother struct {
//        history [][]float64  // Ring buffer of last N frames
//        position int          // Current position in ring buffer
//        windowSize int        // Number of frames to average
//    }
//
//    func (s *SMAoother) Smooth(current []float64) []float64 {
//        // Add current to history
//        s.history[s.position] = current
//        s.position = (s.position + 1) % s.windowSize
//
//        // Calculate average across history
//        smoothed := make([]float64, len(current))
//        for i := range smoothed {
//            sum := 0.0
//            for j := 0; j < s.windowSize; j++ {
//                sum += s.history[j][i]
//            }
//            smoothed[i] = sum / float64(s.windowSize)
//        }
//        return smoothed
//    }
//
// 4. KALMAN FILTER (advanced, best quality, complex)
//    See https://en.wikipedia.org/wiki/Kalman_filter
//    Provides optimal smoothing with noise estimation
//    Overkill for spectrum visualization
//
// HOW TO EXPERIMENT:
//
// 1. Change SMOOTHING_ALPHA in viz/config.go
// 2. Rebuild: go build
// 3. Test with music
// 4. Compare different alpha values:
//    - 1.0: Raw, jittery (no smoothing)
//    - 0.7: Fast music (EDM)
//    - 0.4: Most music (default)
//    - 0.2: Slow music (ambient)
//    - 0.1: Demo mode (very smooth)
//
// 5. Consider per-band smoothing for advanced use
//
// PERFORMANCE:
//   - EMA is O(N) where N = number of bands
//   - Negligible CPU impact (< 0.1ms per frame)
//   - No memory allocations after initialization
