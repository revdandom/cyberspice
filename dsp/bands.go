package dsp

import (
	"cyberspec/viz"
	"math"
)

// BandCalculator handles mapping FFT bins to logarithmic frequency bands
type BandCalculator struct {
	boundaries []float64 // Frequency boundaries for each band
	sampleRate int
	fftSize    int
}

// NewBandCalculator creates a new band calculator
//
// WHY LOGARITHMIC DISTRIBUTION?
//
// Human hearing perceives frequencies logarithmically:
//   - We can distinguish 100 Hz from 200 Hz (100 Hz difference)
//   - But can't distinguish 10,100 Hz from 10,200 Hz (same 100 Hz difference)
//   - Musical octaves are logarithmic: each doubling of frequency = one octave
//
// Linear distribution would:
//   - Waste resolution in high frequencies
//   - Lack detail in musically important bass/mid ranges
//   - Not match how we perceive music
//
// Logarithmic distribution:
//   - Matches human perception
//   - Aligns with musical note spacing
//   - Provides good detail across entire spectrum
//
// Parameters:
//   sampleRate - Audio sample rate in Hz
//   fftSize    - FFT window size
//
// Returns:
//   *BandCalculator - Initialized calculator ready to map bins to bands
func NewBandCalculator(sampleRate, fftSize int) *BandCalculator {
	bc := &BandCalculator{
		sampleRate: sampleRate,
		fftSize:    fftSize,
		boundaries: make([]float64, viz.NUM_BANDS+1),
	}

	// Calculate logarithmic frequency boundaries
	bc.calculateBoundaries()

	return bc
}

// calculateBoundaries calculates logarithmic frequency boundaries for all bands
//
// ALGORITHM:
//   1. Convert min/max frequencies to logarithmic scale (log₁₀)
//   2. Divide the log range into NUM_BANDS equal parts
//   3. Convert each boundary back to linear frequency
//
// RESULT:
//   Lower frequencies get narrower bands (more detail)
//   Higher frequencies get wider bands (less wasted resolution)
//
// EXAMPLE (16 bands, 20-20000 Hz):
//   Band  1:    20 -    40 Hz  (20 Hz wide)
//   Band  2:    40 -    80 Hz  (40 Hz wide)
//   Band  8:  1000 -  1600 Hz  (600 Hz wide)
//   Band 16: 18000 - 20000 Hz  (2000 Hz wide)
func (bc *BandCalculator) calculateBoundaries() {
	// Convert to logarithmic scale
	logMin := math.Log10(viz.MIN_FREQ)
	logMax := math.Log10(viz.MAX_FREQ)
	logRange := logMax - logMin

	// Calculate each boundary
	for i := 0; i <= viz.NUM_BANDS; i++ {
		// Position in logarithmic range (0.0 to 1.0)
		position := float64(i) / float64(viz.NUM_BANDS)

		// Frequency at this position
		logFreq := logMin + (position * logRange)

		// Convert back to linear frequency
		bc.boundaries[i] = math.Pow(10, logFreq)
	}
}

// GetBoundaries returns the frequency boundaries for all bands
// Useful for debugging or displaying band information
//
// Returns:
//   []float64 - Array of (NUM_BANDS + 1) frequencies
//               boundaries[i] to boundaries[i+1] defines band i
func (bc *BandCalculator) GetBoundaries() []float64 {
	return bc.boundaries
}

// MapFFTToBands maps FFT bin magnitudes to frequency bands
//
// ALGORITHM:
//   1. For each frequency band:
//      a. Find FFT bins that fall within the band's frequency range
//      b. Average the magnitudes of those bins
//      c. Store as the band's magnitude
//
// WHY AVERAGING?
//   - Each band may contain multiple FFT bins
//   - Averaging provides smooth representation
//   - Alternative: sum, max, RMS (root mean square)
//
// FFT BIN FREQUENCY:
//   bin_freq = bin_index × (sample_rate / fft_size)
//   Example: bin 10 at 48kHz with 2048 FFT = 10 × (48000/2048) = 234 Hz
//
// Parameters:
//   fftMagnitudes - Raw FFT magnitudes (or weighted, if A-weighting applied)
//
// Returns:
//   []float64 - Array of NUM_BANDS magnitudes, one per frequency band
func (bc *BandCalculator) MapFFTToBands(fftMagnitudes []float64) []float64 {
	bands := make([]float64, viz.NUM_BANDS)

	for i := 0; i < viz.NUM_BANDS; i++ {
		// Frequency range for this band
		minFreq := bc.boundaries[i]
		maxFreq := bc.boundaries[i+1]

		// Convert frequencies to FFT bin indices
		// bin = freq × (fft_size / sample_rate)
		minBin := int(minFreq * float64(bc.fftSize) / float64(bc.sampleRate))
		maxBin := int(maxFreq * float64(bc.fftSize) / float64(bc.sampleRate))

		// Clamp to valid FFT bin range
		// FFT output is symmetric, we only use first half (0 to fftSize/2)
		if minBin < 0 {
			minBin = 0
		}
		if maxBin >= len(fftMagnitudes) {
			maxBin = len(fftMagnitudes) - 1
		}

		// Average magnitudes in this frequency range
		sum := 0.0
		count := 0

		for bin := minBin; bin <= maxBin && bin < len(fftMagnitudes); bin++ {
			sum += fftMagnitudes[bin]
			count++
		}

		// Calculate average (or 0 if no bins in range)
		if count > 0 {
			bands[i] = sum / float64(count)
		} else {
			bands[i] = 0.0
		}
	}

	return bands
}

// GetBandInfo returns human-readable info about a specific band
// Useful for debugging or displaying band labels
//
// Parameters:
//   bandIndex - Band number (0 to NUM_BANDS-1)
//
// Returns:
//   minFreq - Lower frequency boundary (Hz)
//   maxFreq - Upper frequency boundary (Hz)
//   label   - Descriptive label (e.g., "Bass", "Mid", "Treble")
func (bc *BandCalculator) GetBandInfo(bandIndex int) (minFreq, maxFreq float64, label string) {
	if bandIndex < 0 || bandIndex >= viz.NUM_BANDS {
		return 0, 0, "Invalid"
	}

	minFreq = bc.boundaries[bandIndex]
	maxFreq = bc.boundaries[bandIndex+1]

	// Assign descriptive label based on frequency range
	// These are standard audio engineering ranges
	switch {
	case maxFreq <= 60:
		label = "Sub-bass"
	case maxFreq <= 250:
		label = "Bass"
	case maxFreq <= 500:
		label = "Low-mid"
	case maxFreq <= 2000:
		label = "Mid"
	case maxFreq <= 4000:
		label = "High-mid"
	case maxFreq <= 8000:
		label = "Presence"
	case maxFreq <= 16000:
		label = "Brilliance"
	default:
		label = "Air"
	}

	return
}

// ALTERNATIVE AGGREGATION METHODS:
//
// Instead of averaging, you could use:
//
// 1. Maximum value (more reactive to peaks):
//    func (bc *BandCalculator) MapFFTToBandsMax(fftMagnitudes []float64) []float64 {
//        // ... find min/max bins ...
//        maxVal := 0.0
//        for bin := minBin; bin <= maxBin; bin++ {
//            if fftMagnitudes[bin] > maxVal {
//                maxVal = fftMagnitudes[bin]
//            }
//        }
//        bands[i] = maxVal
//    }
//
// 2. RMS (Root Mean Square - more accurate energy representation):
//    sumSquares := 0.0
//    for bin := minBin; bin <= maxBin; bin++ {
//        sumSquares += fftMagnitudes[bin] * fftMagnitudes[bin]
//    }
//    bands[i] = math.Sqrt(sumSquares / float64(count))
//
// 3. Weighted average (emphasize center frequency of band):
//    centerFreq := (minFreq + maxFreq) / 2
//    weightedSum := 0.0
//    totalWeight := 0.0
//    for bin := minBin; bin <= maxBin; bin++ {
//        binFreq := float64(bin) * float64(bc.sampleRate) / float64(bc.fftSize)
//        weight := 1.0 - math.Abs(binFreq-centerFreq)/(maxFreq-minFreq)
//        weightedSum += fftMagnitudes[bin] * weight
//        totalWeight += weight
//    }
//    bands[i] = weightedSum / totalWeight
//
// See docs/ALGORITHMS.md for more details

// HOW TO CHANGE NUMBER OF BANDS:
//
// 1. Edit viz/config.go:
//      const NUM_BANDS = 32  // Instead of 16
//
// 2. Consider adjusting BAR_SPACING if terminal width is limited
//
// 3. May want to adjust color gradients to work well with more/fewer bands
//
// No other changes needed - logarithmic calculation adapts automatically
