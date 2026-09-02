package viz

// Peak represents a single peak-hold tracker for one frequency band.
type Peak struct {
	Height       float64 // Current peak height (0.0-1.0)
	Age          int64   // ms since this band last rose to a new peak
	DecayStarted bool    // whether the hold time has elapsed and it is falling
}

// PeakTracker tracks the peak-hold marker for every frequency band.
type PeakTracker struct {
	peaks []Peak
	fall  bool // true: marker falls after the hold; false: it only fades
}

// NewPeakTracker creates a peak tracker for numBands bands.
func NewPeakTracker(numBands int) *PeakTracker {
	return &PeakTracker{peaks: make([]Peak, numBands), fall: PEAK_FALL_DEFAULT}
}

// SetFall selects whether the marker falls after its hold (true) or holds
// its captured height and only fades out (false).
func (pt *PeakTracker) SetFall(on bool) { pt.fall = on }

// ToggleFall flips the falling animation on/off.
func (pt *PeakTracker) ToggleFall() { pt.fall = !pt.fall }

// Fall reports whether the falling animation is on.
func (pt *PeakTracker) Fall() bool { return pt.fall }

// Update advances every peak from the current band magnitudes.
//
//   - current > peak.Height : capture a new peak, reset Age.
//   - fall on, after the hold : decay the height by PEAK_FALLOFF_WEIGHT per
//     frame (kept above BAR_FALLOFF_WEIGHT so a falling peak can never
//     descend onto a falling bar), clamped to the live level.
//   - fall off : the marker holds its captured height; it only fades out
//     (renderer.FadePeakColor via GetPeakFade) and then vanishes.
//
// Age always accrues, so the fade works the same in both modes.
func (pt *PeakTracker) Update(currentMagnitudes []float64, deltaMs int64) {
	for i := range pt.peaks {
		peak := &pt.peaks[i]
		current := currentMagnitudes[i]

		if current > peak.Height {
			peak.Height = current
			peak.Age = 0
			peak.DecayStarted = false
			continue
		}

		peak.Age += deltaMs

		if !peak.DecayStarted && peak.Age >= PEAK_HOLD_MS {
			peak.DecayStarted = true
		}

		if pt.fall && peak.DecayStarted {
			peak.Height *= PEAK_FALLOFF_WEIGHT
			if peak.Height < current {
				peak.Height = current
			}
			if peak.Height < 0.0 {
				peak.Height = 0.0
			}
		}
	}
}

// GetPeakHeight returns the current peak height for a band (0.0-1.0).
func (pt *PeakTracker) GetPeakHeight(bandIndex int) float64 {
	if bandIndex < 0 || bandIndex >= len(pt.peaks) {
		return 0.0
	}
	return pt.peaks[bandIndex].Height
}

// GetPeakFade returns the marker's fade progress: 0 while the band is still
// rising / at a fresh peak, ramping to 1 (fully invisible) over PEAK_FADE_MS
// once it goes quiet. renderer.FadePeakColor applies the gamma curve.
func (pt *PeakTracker) GetPeakFade(bandIndex int) float64 {
	if bandIndex < 0 || bandIndex >= len(pt.peaks) {
		return 1.0
	}
	f := float64(pt.peaks[bandIndex].Age) / float64(PEAK_FADE_MS)
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
