package dsp

import (
	"cyberspice/viz"
	"math"
)

// CalculateAWeighting calculates A-weighting multipliers for all FFT bins
//
// A-weighting is an industry-standard frequency weighting curve that models
// human ear sensitivity across frequencies. It solves the "bass-heavy" problem
// common in spectrum analyzers by reducing bass and boosting mids/highs.
//
// WHY IT'S NEEDED:
// - Raw FFT magnitudes are heavily biased toward low frequencies
// - Bass frequencies carry more physical energy in music
// - Without correction, bass dominates visualization while mids/highs are barely visible
// - A-weighting balances the visualization to match perceived loudness
//
// FORMULA:
//
//	A(f) = 20 × log₁₀(
//	  (12194² × f⁴) /
//	  ((f² + 20.6²) × √((f² + 107.7²) × (f² + 737.9²)) × (f² + 12194²))
//	)
//
// Where:
//
//	f = frequency in Hz
//	Result is in dB (decibels)
//	Convert dB to linear multiplier: 10^(A(f)/20)
//
// EFFECT:
//
//	<100 Hz:    Reduced by ~30-40 dB (bass)
//	2-5 kHz:    Slight boost (presence range)
//	>10 kHz:    Slight reduction (very high frequencies)
//
// ALTERNATIVES:
//   - C-weighting: More neutral, less bass reduction
//   - Equal-loudness contours (ISO 226): More accurate but complex
//   - Custom per-band scaling: Simple multipliers, easy to tweak
//
// CONFIGURATION:
//
//	Set viz.ENABLE_A_WEIGHTING = false to use raw FFT magnitudes
//
// Parameters:
//
//	sampleRate - Audio sample rate in Hz (e.g., 48000)
//	fftSize    - FFT window size (must be power of 2)
//
// Returns:
//
//	[]float64 - Array of linear multipliers, one per FFT bin
func CalculateAWeighting(sampleRate, fftSize int) []float64 {
	// Only calculate for positive frequencies (first half of FFT)
	numBins := fftSize / 2
	weights := make([]float64, numBins)

	for i := 0; i < numBins; i++ {
		// Calculate frequency for this bin
		// Frequency = bin_index × (sample_rate / fft_size)
		freq := float64(i) * float64(sampleRate) / float64(fftSize)

		// Handle DC bin (0 Hz) - set to zero to avoid math errors
		if freq < 1.0 {
			weights[i] = 0.0
			continue
		}

		// A-weighting formula
		// Pre-calculate powers for efficiency
		f2 := freq * freq // f²
		f4 := f2 * f2     // f⁴

		// Numerator: 12194² × f⁴
		numerator := 12194.0 * 12194.0 * f4

		// Denominator: (f² + 20.6²) × √((f² + 107.7²) × (f² + 737.9²)) × (f² + 12194²)
		// These constants (20.6, 107.7, 737.9, 12194) are from the A-weighting standard
		term1 := f2 + (20.6 * 20.6)
		term2 := math.Sqrt((f2 + (107.7 * 107.7)) * (f2 + (737.9 * 737.9)))
		term3 := f2 + (12194.0 * 12194.0)

		denominator := term1 * term2 * term3

		// Calculate A-weighting in dB
		aWeightDB := 20.0 * math.Log10(numerator/denominator)

		// Convert dB to linear multiplier
		// dB to linear: 10^(dB/20)
		weights[i] = math.Pow(10.0, aWeightDB/20.0)
	}

	return weights
}

// ApplyWeighting applies pre-calculated weighting to FFT magnitudes
//
// This is called during FFT processing to apply A-weighting to each bin
// before aggregating into frequency bands.
//
// Parameters:
//
//	magnitudes - Raw FFT magnitudes (one per bin)
//	weights    - Pre-calculated weighting multipliers (from CalculateAWeighting)
//
// Returns:
//
//	[]float64 - Weighted magnitudes
func ApplyWeighting(magnitudes, weights []float64) []float64 {
	if !viz.ENABLE_A_WEIGHTING {
		// A-weighting disabled - return raw magnitudes
		return magnitudes
	}

	// Apply weighting to each bin
	weighted := make([]float64, len(magnitudes))
	for i := range magnitudes {
		weighted[i] = magnitudes[i] * weights[i]
	}

	return weighted
}

// HOW TO MODIFY THE CURVE:
//
// 1. Adjust curve strength:
//    Change the division in the dB-to-linear conversion:
//      weights[i] = math.Pow(10.0, aWeightDB/15.0)  // More aggressive
//      weights[i] = math.Pow(10.0, aWeightDB/25.0)  // Gentler
//
// 2. Implement custom curve:
//    func CustomWeighting(freq float64) float64 {
//        switch {
//        case freq < 100:  return 0.2  // Heavily reduce sub-bass
//        case freq < 500:  return 0.4  // Reduce bass
//        case freq < 2000: return 1.0  // Keep mids neutral
//        case freq < 8000: return 1.5  // Boost presence
//        default:          return 1.2  // Slightly boost highs
//        }
//    }
//
// 3. Disable for specific frequency ranges:
//    if freq >= 100 && freq <= 8000 {
//        // Only apply A-weighting to this range
//        weights[i] = math.Pow(10.0, aWeightDB/20.0)
//    } else {
//        weights[i] = 1.0  // No weighting outside range
//    }
//
