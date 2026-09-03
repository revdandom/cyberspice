package dsp

import (
	"cyberspice/viz"
	"math"

	"github.com/madelynnblue/go-dsp/fft"
)

// FFTProcessor handles Fast Fourier Transform processing for audio
type FFTProcessor struct {
	fftSize      int
	sampleRate   int
	numBands     int
	window       []float64       // Hann window coefficients
	aWeighting   []float64       // Pre-calculated A-weighting multipliers
	bandCalc     *BandCalculator // Band calculator
	buffer       []float64       // Reusable buffer for FFT input
	tiltDBPerOct float64         // Spectral tilt slope (high-freq lift)
	tiltGains    []float64       // Per-band linear tilt multipliers

	// Auto-gain (cava / cli-visualizer style): one scalar over all bands.
	sensitivity float64 // current gain multiplier
	sensInit    bool    // true during the fast initial calibration ramp
	lowFrames   int     // consecutive frames the peak has stayed low
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
//  1. Window function: Reduces spectral leakage
//  2. FFT algorithm: Actual time→frequency conversion
//  3. Magnitude calculation: Convert complex output to real magnitudes
//  4. A-weighting: Balance frequency response (optional)
//  5. Band mapping: Group FFT bins into display bands
//
// Parameters:
//
//	sampleRate - Audio sample rate in Hz
//
// Returns:
//
//	*FFTProcessor - Initialized processor ready to process audio
func NewFFTProcessor(sampleRate int) *FFTProcessor {
	fp := &FFTProcessor{
		fftSize:      viz.FFT_SIZE,
		sampleRate:   sampleRate,
		numBands:     viz.NUM_BANDS,
		buffer:       make([]float64, viz.FFT_SIZE),
		tiltDBPerOct: viz.SPECTRAL_TILT_DB_PER_OCT,
		sensitivity:  1.0,
		sensInit:     true,
	}

	// Pre-calculate Hann window
	fp.window = calculateHannWindow(fp.fftSize)

	// Pre-calculate A-weighting multipliers
	fp.aWeighting = CalculateAWeighting(sampleRate, fp.fftSize)

	// Create band calculator + tilt table
	fp.bandCalc = NewBandCalculator(sampleRate, fp.fftSize, fp.numBands)
	fp.rebuildTilt()

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
	fp.rebuildTilt()
}

// SetTilt sets the spectral tilt slope in dB per octave (high-frequency
// lift). 0 is flat. Rebuilds the per-band multiplier table.
func (fp *FFTProcessor) SetTilt(dbPerOctave float64) {
	fp.tiltDBPerOct = dbPerOctave
	fp.rebuildTilt()
}

// rebuildTilt recomputes the per-band tilt multipliers: a boost-only high
// shelf that counters music's natural bass-heavy roll-off without
// A-weighting's aggressive low cut. Band 0 (≈ MIN_FREQ) is unity; each
// octave above it adds tiltDBPerOct dB, with a uniform slope all the way up
// so that raising the tilt always raises the high end.
func (fp *FFTProcessor) rebuildTilt() {
	fp.tiltGains = make([]float64, fp.numBands)
	for i := 0; i < fp.numBands; i++ {
		if fp.tiltDBPerOct <= 0 {
			fp.tiltGains[i] = 1.0
			continue
		}
		lo, hi, _ := fp.bandCalc.GetBandInfo(i)
		center := math.Sqrt(lo * hi)
		if center < viz.MIN_FREQ {
			center = viz.MIN_FREQ
		}
		db := fp.tiltDBPerOct * math.Log2(center/viz.MIN_FREQ)
		fp.tiltGains[i] = math.Pow(10.0, db/20.0)
	}
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
//
//	w[n] = 0.5 × (1 - cos(2π × n / (N-1)))
//	where n = 0 to N-1, N = window size
//
// Parameters:
//
//	size - Window size (same as FFT size)
//
// Returns:
//
//	[]float64 - Array of window coefficients
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
//  1. Take audio samples (may be more or less than FFT size)
//  2. Apply Hann window to reduce spectral leakage
//  3. Perform FFT (time → frequency conversion)
//  4. Calculate magnitude from complex FFT output
//  5. Apply A-weighting (if enabled)
//  6. Map FFT bins to logarithmic frequency bands
//  7. Return band magnitudes ready for visualization
//
// INPUT SIZE HANDLING:
//   - If input < FFT_SIZE: Zero-pad to FFT_SIZE
//   - If input > FFT_SIZE: Use only first FFT_SIZE samples
//   - Typical: input == FFT_SIZE (most efficient)
//
// Parameters:
//
//	samples - Audio samples (float64, typically -1.0 to +1.0)
//
// Returns:
//
//	[]float64 - Array of NUM_BANDS magnitudes ready for visualization
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

	// Apply A-weighting only when enabled. A-weighting cuts the low end by
	// 20-40 dB, which makes bass "disappear" from the visualisation, so it
	// is off by default (see viz.ENABLE_A_WEIGHTING).
	if viz.ENABLE_A_WEIGHTING {
		magnitudes = ApplyWeighting(magnitudes, fp.aWeighting)
	}

	// Map FFT bins to display bands
	bands := fp.bandCalc.MapFFTToBands(magnitudes)

	// Gentle spectral tilt (high-frequency lift) so bass and treble read at
	// comparable heights on typical music.
	for i := range bands {
		bands[i] *= fp.tiltGains[i]
	}

	// Normalize bands to 0.0-1.0 range
	// This will be further scaled by gain control
	return fp.normalizeBands(bands)
}

// ProcessRaw runs FFT → magnitudes → (optional A-weighting) → band mapping →
// spectral tilt, but NOT the AGC normalise. The caller drives the shared AGC
// with NormalizeShared. fp.buffer is reused, so consume each result before
// the next call.
func (fp *FFTProcessor) ProcessRaw(samples []float64) []float64 {
	fp.prepareFFTInput(samples)
	fftOutput := fft.FFTReal(fp.buffer)

	numBins := len(fftOutput) / 2
	magnitudes := make([]float64, numBins)
	for i := 0; i < numBins; i++ {
		re, im := real(fftOutput[i]), imag(fftOutput[i])
		magnitudes[i] = math.Sqrt(re*re + im*im)
	}
	if viz.ENABLE_A_WEIGHTING {
		magnitudes = ApplyWeighting(magnitudes, fp.aWeighting)
	}

	bands := fp.bandCalc.MapFFTToBands(magnitudes)
	for i := range bands {
		bands[i] *= fp.tiltGains[i]
	}
	return bands
}

// NormalizeShared advances the AGC once from the loudest band across every
// supplied set, then scales each set in place. Use it so a stereo layout's
// left and right channels share one gain (a hard-panned mix would otherwise
// blow up one side and shrink the other).
func (fp *FFTProcessor) NormalizeShared(sets ...[]float64) {
	frameMax := 0.0
	for _, s := range sets {
		for _, v := range s {
			if v > frameMax {
				frameMax = v
			}
		}
	}
	fp.agcStep(frameMax)
	for _, s := range sets {
		fp.applyGain(s)
	}
}

// prepareFFTInput prepares audio samples for FFT
//
// STEPS:
//  1. Copy samples to buffer (zero-pad or truncate as needed)
//  2. Apply Hann window
//  3. Buffer is ready for FFT
//
// Parameters:
//
//	samples - Raw audio samples
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

// normalizeBands maps raw band magnitudes into 0.0-1.0 for display using a
// cava / cli-visualizer style auto-gain: a single `sensitivity` scalar
// multiplies every band, and the display is allowed to breathe with the
// music rather than being renormalised to full scale every frame.
//
// Adaptation:
//   - Initial ramp: sensitivity climbs fast (AGC_INIT_UP per frame) until a
//     peak first reaches AGC_TARGET, so it self-calibrates within ~1 s.
//   - Clip: if the scaled peak exceeds AGC_TARGET, pull sensitivity down
//     toward the value that would put it back on target (AGC_DOWN strength).
//   - Quiet: if the scaled peak stays below AGC_TARGET*AGC_LOW_RATIO for
//     AGC_LOW_FRAMES frames, nudge sensitivity up (AGC_UP).
//   - Silence (scaled peak below AGC_QUIET_FLOOR): hold steady.
func (fp *FFTProcessor) normalizeBands(bands []float64) []float64 {
	frameMax := 0.0
	for _, val := range bands {
		if val > frameMax {
			frameMax = val
		}
	}
	fp.agcStep(frameMax)

	out := make([]float64, len(bands))
	copy(out, bands)
	fp.applyGain(out)
	return out
}

// agcStep advances the auto-gain sensitivity one frame from this frame's
// loudest raw band value.
func (fp *FFTProcessor) agcStep(frameMax float64) {
	scaledMax := frameMax * fp.sensitivity

	switch {
	case scaledMax > viz.AGC_TARGET:
		// Overshoot — ease sensitivity down toward on-target.
		fp.sensInit = false
		fp.lowFrames = 0
		fp.sensitivity *= math.Pow(viz.AGC_TARGET/scaledMax, viz.AGC_DOWN)

	case scaledMax < viz.AGC_QUIET_FLOOR:
		// Silence — don't chase noise up.
		fp.lowFrames = 0

	case fp.sensInit:
		// Fast initial calibration ramp.
		fp.sensitivity *= viz.AGC_INIT_UP

	case scaledMax < viz.AGC_TARGET*viz.AGC_LOW_RATIO:
		// Consistently low — lift gently after a short hold.
		fp.lowFrames++
		if fp.lowFrames >= viz.AGC_LOW_FRAMES {
			fp.sensitivity *= viz.AGC_UP
			fp.lowFrames = 0
		}

	default:
		fp.lowFrames = 0
	}

	if !(fp.sensitivity > 0) { // guard against NaN / 0 / negative
		fp.sensitivity = 1.0
	}
}

// applyGain scales bands in place by the current sensitivity, applies the
// noise gate, and clamps to [0, 1].
func (fp *FFTProcessor) applyGain(bands []float64) {
	for i, val := range bands {
		n := val * fp.sensitivity
		if n < viz.AGC_NOISE_GATE {
			n = 0.0
		}
		if n > 1.0 {
			n = 1.0
		}
		bands[i] = n
	}
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
