package dsp

import (
	"cyberspec/viz"
	"math"

	"github.com/madelynnblue/go-dsp/fft"
)

// FFTProcessor handles Fast Fourier Transform processing for audio
type FFTProcessor struct {
	fftSize    int
	sampleRate int
	numBands   int
	window     []float64       // Hann window coefficients
	aWeighting []float64       // Pre-calculated A-weighting multipliers
	bandCalc   *BandCalculator // Band calculator
	buffer     []float64       // Reusable buffer for FFT input
	agcMax     float64         // Running loudness ceiling for auto gain control
}

// NewFFTProcessor creates a new FFT processor
//
// WHAT IS FFT?
//
// FFT (Fast Fourier Transform) converts audio from time domain to frequency domain:
//   - Input: Array of audio samples over time
//   - Output: Array of frequency bins showing magnitude at each frequency
//
// WHY WE NEED IT:
//   - Audio capture gives us amplitude values over time
//   - Spectrum analyzer needs frequency content (bass, mids, highs)
//   - FFT tells us "how much" of each frequency is present
//
// COMPONENTS:
//   1. Window function: Reduces spectral leakage
//   2. FFT algorithm: Actual time→frequency conversion
//   3. Magnitude calculation: Convert complex output to real magnitudes
//   4. A-weighting: Balance frequency response (optional)
//   5. Band mapping: Group FFT bins into display bands
//
// Parameters:
//   sampleRate - Audio sample rate in Hz
//
// Returns:
//   *FFTProcessor - Initialized processor ready to process audio
func NewFFTProcessor(sampleRate int) *FFTProcessor {
	fp := &FFTProcessor{
		fftSize:    viz.FFT_SIZE,
		sampleRate: sampleRate,
		numBands:   viz.NUM_BANDS,
		buffer:     make([]float64, viz.FFT_SIZE),
	}

	// Pre-calculate Hann window
	fp.window = calculateHannWindow(fp.fftSize)

	// Pre-calculate A-weighting multipliers
	fp.aWeighting = CalculateAWeighting(sampleRate, fp.fftSize)

	// Create band calculator
	fp.bandCalc = NewBandCalculator(sampleRate, fp.fftSize, fp.numBands)

	return fp
}

// SetNumBands rebuilds the band mapping for a new band count (e.g. after a
// terminal resize). No-op if unchanged. The AGC ceiling is preserved so the
// gain does not jump on resize.
func (fp *FFTProcessor) SetNumBands(numBands int) {
	if numBands < 1 || numBands == fp.numBands {
		return
	}
	fp.numBands = numBands
	fp.bandCalc = NewBandCalculator(fp.sampleRate, fp.fftSize, numBands)
}

// NumBands returns the current band count.
func (fp *FFTProcessor) NumBands() int {
	return fp.numBands
}

// calculateHannWindow calculates Hann window coefficients
//
// WHAT IS A WINDOW FUNCTION?
//
// FFT assumes the input signal repeats infinitely. When analyzing a finite chunk
// of audio, discontinuities at the edges cause "spectral leakage" - energy
// bleeding into adjacent frequency bins.
//
// A window function smoothly tapers the signal to zero at edges, reducing leakage.
//
// WHY HANN WINDOW?
//   - Good frequency resolution
//   - Low spectral leakage
//   - Standard choice for music analysis
//   - Balances time and frequency resolution
//
// ALTERNATIVES:
//   - Hamming: Similar to Hann, slightly different shape
//   - Blackman: Lower leakage, worse frequency resolution
//   - Rectangular: No windowing (maximum leakage)
//
// FORMULA:
//   w[n] = 0.5 × (1 - cos(2π × n / (N-1)))
//   where n = 0 to N-1, N = window size
//
// Parameters:
//   size - Window size (same as FFT size)
//
// Returns:
//   []float64 - Array of window coefficients
func calculateHannWindow(size int) []float64 {
	window := make([]float64, size)

	for i := 0; i < size; i++ {
		// Hann window formula
		window[i] = 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(size-1)))
	}

	return window
}

// Process converts audio samples to frequency band magnitudes
//
// PROCESSING PIPELINE:
//   1. Take audio samples (may be more or less than FFT size)
//   2. Apply Hann window to reduce spectral leakage
//   3. Perform FFT (time → frequency conversion)
//   4. Calculate magnitude from complex FFT output
//   5. Apply A-weighting (if enabled)
//   6. Map FFT bins to logarithmic frequency bands
//   7. Return band magnitudes ready for visualization
//
// INPUT SIZE HANDLING:
//   - If input < FFT_SIZE: Zero-pad to FFT_SIZE
//   - If input > FFT_SIZE: Use only first FFT_SIZE samples
//   - Typical: input == FFT_SIZE (most efficient)
//
// Parameters:
//   samples - Audio samples (float64, typically -1.0 to +1.0)
//
// Returns:
//   []float64 - Array of NUM_BANDS magnitudes ready for visualization
func (fp *FFTProcessor) Process(samples []float64) []float64 {
	// Prepare FFT input buffer
	fp.prepareFFTInput(samples)

	// Perform FFT
	// Input: Real-valued audio samples
	// Output: Complex frequency bins (real + imaginary components)
	fftOutput := fft.FFTReal(fp.buffer)

	// Calculate magnitudes from complex FFT output
	// Only use first half (positive frequencies)
	// FFT output is symmetric for real-valued input
	numBins := len(fftOutput) / 2
	magnitudes := make([]float64, numBins)

	for i := 0; i < numBins; i++ {
		// Magnitude = √(real² + imag²)
		real := real(fftOutput[i])
		imag := imag(fftOutput[i])
		magnitudes[i] = math.Sqrt(real*real + imag*imag)
	}

	// Apply A-weighting (if enabled)
	// This balances bass/mid/treble for better visualization
	weightedMagnitudes := ApplyWeighting(magnitudes, fp.aWeighting)

	// Map FFT bins to display bands
	// Converts many FFT bins to NUM_BANDS for visualization
	bands := fp.bandCalc.MapFFTToBands(weightedMagnitudes)

	// Normalize bands to 0.0-1.0 range
	// This will be further scaled by gain control
	return fp.normalizeBands(bands)
}

// prepareFFTInput prepares audio samples for FFT
//
// STEPS:
//   1. Copy samples to buffer (zero-pad or truncate as needed)
//   2. Apply Hann window
//   3. Buffer is ready for FFT
//
// Parameters:
//   samples - Raw audio samples
func (fp *FFTProcessor) prepareFFTInput(samples []float64) {
	// Clear buffer
	for i := range fp.buffer {
		fp.buffer[i] = 0.0
	}

	// Copy samples (up to FFT size)
	copySize := len(samples)
	if copySize > fp.fftSize {
		copySize = fp.fftSize
	}

	for i := 0; i < copySize; i++ {
		// Apply window function while copying
		// This reduces spectral leakage
		fp.buffer[i] = samples[i] * fp.window[i]
	}

	// If samples < fftSize, buffer is already zero-padded from clear step
}

// normalizeBands normalizes band magnitudes to 0.0-1.0 range
//
// NORMALIZATION STRATEGIES:
//
// Current: Simple peak normalization
//   - Find maximum value across all bands
//   - Divide all bands by maximum
//   - Result: brightest band = 1.0, others proportional
//
// ALTERNATIVES:
//
// 1. Fixed scaling (no auto-gain):
//    scale := 1000.0  // Fixed divisor
//    normalized[i] = bands[i] / scale
//
// 2. RMS normalization (maintains energy relationships):
//    rms := calculateRMS(bands)
//    normalized[i] = bands[i] / (rms * 3.0)
//
// 3. Per-band normalization (each band independent):
//    normalized[i] = bands[i] / maxSeenForBand[i]
//
// 4. Logarithmic scaling (compress dynamic range):
//    normalized[i] = log10(1 + bands[i]) / log10(1 + maxBand)
//
// Parameters:
//   bands - Raw band magnitudes
//
// Returns:
//   []float64 - Normalized band magnitudes (0.0-1.0 range)
func (fp *FFTProcessor) normalizeBands(bands []float64) []float64 {
	// Current frame's loudest band.
	frameMax := 0.0
	for _, val := range bands {
		if val > frameMax {
			frameMax = val
		}
	}

	// Auto gain control: track a running ceiling that rises quickly (smoothed
	// attack, so a single transient does not slam it) and falls slowly. The
	// visualisation is scaled against this ceiling rather than the raw
	// per-frame max, which gives a stable, self-calibrating gain.
	if frameMax > fp.agcMax {
		fp.agcMax = fp.agcMax*viz.AGC_ATTACK + frameMax*(1.0-viz.AGC_ATTACK)
	} else {
		fp.agcMax = fp.agcMax * viz.AGC_RELEASE
		if fp.agcMax < frameMax {
			fp.agcMax = frameMax
		}
	}

	if fp.agcMax <= 0.0 {
		return make([]float64, len(bands))
	}

	normalized := make([]float64, len(bands))
	for i, val := range bands {
		n := val / fp.agcMax
		if n < viz.AGC_NOISE_GATE {
			n = 0.0
		}
		if n > 1.0 {
			n = 1.0
		}
		normalized[i] = n
	}

	return normalized
}

// GetBandInfo returns information about a frequency band
// Wrapper for BandCalculator.GetBandInfo
func (fp *FFTProcessor) GetBandInfo(bandIndex int) (minFreq, maxFreq float64, label string) {
	return fp.bandCalc.GetBandInfo(bandIndex)
}

// PERFORMANCE NOTES:
//
// FFT is O(N log N) where N = FFT_SIZE
//   - 2048 FFT: ~11 operations per sample
//   - 4096 FFT: ~12 operations per sample
//   - Doubling size adds ~1 extra operation per sample
//
// At 30 FPS with 2048 FFT:
//   - ~23,000 operations per frame
//   - Should be <1ms on modern CPU
//
// OPTIMIZATION OPPORTUNITIES:
//   1. Reuse FFT buffers (already done)
//   2. Use FFTW library for faster FFT (requires CGo)
//   3. Reduce FFT size (trades frequency resolution for speed)
//   4. Process in separate goroutine (parallel with rendering)
//
// HOW TO CHANGE FFT SIZE:
//
// Edit viz/config.go:
//   const FFT_SIZE = 4096  // Higher resolution, slower
//   const FFT_SIZE = 1024  // Lower resolution, faster
//
// Must be power of 2: 512, 1024, 2048, 4096, 8192, etc.
//
// TRADE-OFFS:
//   Larger FFT:
//     + Better frequency resolution (narrower bins)
//     + More accurate for low frequencies
//     - Slower processing
//     - Worse time resolution (longer window)
//
//   Smaller FFT:
//     + Faster processing
//     + Better time resolution (shorter window)
//     - Worse frequency resolution (wider bins)
//     - Less accurate for low frequencies
