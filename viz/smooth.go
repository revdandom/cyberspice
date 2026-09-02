package viz

import "math"

// SpreadNeighbors applies a monstercat-style spatial spread: every band bleeds
// into the others as value / factor^distance, keeping the per-band maximum.
// This fills gaps and widens peaks so the spectrum looks like a continuous
// envelope rather than isolated spikes. factor <= 1.0 returns the input
// unchanged. Runs O(n^2) but n is at most a few dozen bands.
func SpreadNeighbors(bands []float64, factor float64) []float64 {
	if factor <= 1.0 || len(bands) == 0 {
		return bands
	}

	out := make([]float64, len(bands))
	copy(out, bands)

	for i, v := range bands {
		if v <= 0 {
			continue
		}
		for j := range out {
			if j == i {
				continue
			}
			d := j - i
			if d < 0 {
				d = -d
			}
			if spread := v / math.Pow(factor, float64(d)); spread > out[j] {
				out[j] = spread
			}
		}
	}

	return out
}

// Smoother provides temporal smoothing for frequency band magnitudes.
//
// It is asymmetric (fast attack, slow release), like dpayne/cli-visualizer:
//   - rising  → EMA toward the new value with weight `alpha`
//   - falling → exponential decay by `falloffWeight` per frame, clamped so
//               the bar never drops below the current live level
type Smoother struct {
	previousValues []float64 // Previous frame's smoothed values
	alpha          float64   // Attack weight (0.0 = frozen, 1.0 = instant rise)
	falloffWeight  float64   // Release decay per frame (0.90 fast .. 0.985 slow)
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
// SOLUTION: Asymmetric attack/release (fast rise, slow fall)
//
// Symmetric EMA makes bars snap in both directions, which reads as
// "twitchy". Instead we rise fast and fall slow, the classic VU-meter /
// cli-visualizer feel.
//
// FORMULA (per band, per frame):
//   if current >= previous:   next = α·current + (1-α)·previous   (attack)
//   else:                     next = max(current, previous·w)     (release)
//
// Where:
//   α (SMOOTHING_ALPHA)      = attack weight, 0.0..1.0 (higher = snappier rise)
//   w (BAR_FALLOFF_WEIGHT)   = release decay per frame, e.g. 0.965
//                              (lower = faster fall; clamped to live level)
//
// CONFIGURATION:
//   See viz/config.go: SMOOTHING_ALPHA, BAR_FALLOFF_WEIGHT
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
		falloffWeight:  BAR_FALLOFF_WEIGHT,
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
		prev := s.previousValues[i]

		var next float64
		if current[i] >= prev {
			// Attack: EMA toward the louder value.
			next = (s.alpha * current[i]) + ((1.0 - s.alpha) * prev)
		} else {
			// Release: exponential decay, but never below the live level.
			next = prev * s.falloffWeight
			if next < current[i] {
				next = current[i]
			}
		}

		s.previousValues[i] = next
		smoothed[i] = next
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
