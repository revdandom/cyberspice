# How CyberSpice works

The signal path, and the maths behind each stage. Every constant named here
lives in [`viz/config.go`](../viz/config.go) with an inline comment; this doc
explains how they fit together.

```
system audio ─► PipeWire / PulseAudio default-sink monitor source
             ─► capture @ 48 kHz stereo  (audio/capture.go)
             ─► rolling FFT_SIZE-sample window
             ─► Hann window ─► FFT ─► magnitudes            (dsp/fft.go)
             ─► [optional A-weighting]                       (dsp/weighting.go)
             ─► log-spaced band mapping                      (dsp/bands.go)
             ─► per-band spectral tilt
             ─► auto-gain (single sensitivity scalar)
             ─► asymmetric temporal smoothing               (viz/smooth.go)
             ─► monstercat spatial spread
             ─► loudness curve (bar-height mapping)          (viz/renderer.go)
             ─► peak-hold + fall + fade                      (viz/peaks.go)
             ─► colour ramp + glyphs                         (viz/colors.go)
             ─► terminal, TARGET_FPS (30)
```

Motion feel (attack/release pacing, peak markers that fall about half as fast
as the bars and then fade out) is modelled on
[**vis-cli-visualizer**](https://github.com/dtoraelek/vis-cli-visualizer).
The auto-gain is [**cava**](https://github.com/karlstav/cava)-style; the
spatial "monstercat" smoothing and the fast-attack/slow-release envelope come
from [**dpayne/cli-visualizer**](https://github.com/dpayne/cli-visualizer).

---

## 1. Audio capture

CyberSpice records the **default sink's `.monitor` source** (found via
`pactl info`), i.e. what your speakers are playing — not the microphone.

Latency is kept low deliberately. Capture runs on its own goroutine
(`readLoop`) that drains the PulseAudio stream continuously into a rolling
window of the most recent `FFT_SIZE` samples. The stream is opened with an
explicit `pa_buffer_attr` (`Fragsize` ≈ 16 ms, `Maxlength` ≈ 130 ms) so a
slow render frame can't let audio back up in the kernel buffer. Each frame
the FFT gets a full `FFT_SIZE` window of real samples — no zero-padding.

Stereo is kept around: the vertical layout mixes to mono, the butterfly
layout processes left and right separately (see §10).

## 2. FFT and frequency bands

- **Window:** Hann, to cut spectral leakage.
- **Size:** `FFT_SIZE = 4096` at 48 kHz → ~11.7 Hz per bin, ~85 ms window.
  Big enough to resolve the low end without turning transients to mush.
- **Magnitudes:** `sqrt(re² + im²)` per bin, normalised by `FFT_SIZE`.
- **Bands:** the usable spectrum `[MIN_FREQ, MAX_FREQ]` = `[30 Hz, 20 kHz]`
  is split into `N` **logarithmically**-spaced bands (equal ratio per band),
  because pitch is logarithmic. Each band averages the FFT bins that fall in
  its range; a band narrower than one bin samples the containing bin instead
  of reading zero.
- `N` is chosen from the terminal size (see §11), fallback `NUM_BANDS = 32`.

**A-weighting** (`dsp/weighting.go`) is implemented but **off by default**
(`ENABLE_A_WEIGHTING = false`) — it cuts the low end by 20–40 dB and made the
bass vanish. Spectral tilt (next) is the gentler middle ground.

## 3. Spectral tilt

A boost-only high shelf that lifts each octave above `MIN_FREQ` by a fixed
number of dB, so bass and treble read at similar heights without killing
either. Per band, with centre frequency `f`:

```
gain_dB(f) = TILT_DB_PER_OCT · log2(f / MIN_FREQ)          (never negative)
gain(f)    = 10^(gain_dB(f) / 20)
band[i]   *= gain(f_i)
```

The slope is **uniform all the way up** — no cap. An earlier version capped
the shelf; above the cap frequency there was no more HF compensation, so
raising the tilt past the cap actually *dropped* the highs (the AGC then
chased a mid-band peak). Removing the cap makes "more tilt" always mean
"more high end".

- Default `SPECTRAL_TILT_DB_PER_OCT = 3.0` (balanced for music).
- Live: `[` / `]` in steps of 0.5, clamped to `[0, SPECTRAL_TILT_MAX_SLOPE]`
  (6). `-tilt` sets the launch value. Shown in the header as `tilt:`.
- Higher = brighter; the bass shrinks as the auto-gain follows the highs.

## 4. Auto-gain (AGC)

One `sensitivity` scalar multiplies every band. It calibrates fast on
launch, then adapts *gently*, so the picture **breathes** with the music
(loud passages fill the screen, quiet passages sit low) instead of being
renormalised to full scale every frame.

Each frame, with `scaledMax = frameMax · sensitivity` (the loudest band
after gain):

| condition | action |
|-----------|--------|
| `scaledMax > AGC_TARGET` (0.90) | clip — ease down: `sensitivity *= (AGC_TARGET / scaledMax) ^ AGC_DOWN` (0.5) |
| `scaledMax < AGC_QUIET_FLOOR` (0.02) | silence — hold steady, don't chase noise |
| still in the initial ramp | `sensitivity *= AGC_INIT_UP` (1.10) each frame until a peak first hits `AGC_TARGET` |
| `scaledMax < AGC_TARGET · AGC_LOW_RATIO` (0.63) for `AGC_LOW_FRAMES` (15) frames | lift gently: `sensitivity *= AGC_UP` (1.02) |
| otherwise | steady |

After scaling, bands below `AGC_NOISE_GATE` (0.03 of full scale) render as
zero, and everything is clamped to `[0, 1]`.

`-gain` / `+` / `-` / `0` apply a manual trim *on top* of the AGC.

## 5. Loudness curve (bar-height mapping)

The pipeline is linear in amplitude, but the ear is not, so a slightly
louder sound would shoot up disproportionately. `renderer.ampValue` maps the
`[0,1]` band value to a display-height fraction (applied to both bars and
peak markers). Cycle with `a`, or `-curve`; shown as `curve:` in the header.

| mode | mapping | character |
|------|---------|-----------|
| `linear` | `h = v` | raw amplitude |
| `stevens` *(default)* | `h = v ^ AMPLITUDE_EXPONENT` (0.6) | Stevens' power law — perceived loudness ≈ (sound pressure)^0.6, so bar height ≈ loudness |
| `db` | `d = 20·log10(v)`; `h = (d − FLOOR) / −FLOOR` for `d > FLOOR` (`AMPLITUDE_DB_FLOOR = −60 dB`), else 0 | equal dB steps → equal bar steps; opens up quiet detail the most, analyzer-style |

## 6. Temporal smoothing (attack / release)

Raw FFT magnitudes jitter frame to frame (transients, leakage, phase). The
smoother is **asymmetric** — bars jump up fast, ooze down slow, like a VU
meter. Per band, per frame:

```
if current >= previous:   next = α·current + (1−α)·previous        (attack)
else:                      next = max(current, previous · w_bar)    (release)
```

- `α = SMOOTHING_ALPHA = 0.7` — attack weight; higher is snappier.
- `w_bar = BAR_FALLOFF_WEIGHT = 0.93` — release decay per frame, clamped so
  a falling bar never drops below the live level. ~1.05 s from full to 10%
  at 30 FPS — a touch snappier than cli-visualizer so the peak marker
  separates cleanly above the bar.

## 7. Monstercat spatial spread

After temporal smoothing, each band bleeds into its neighbours so the
spectrum reads as a continuous envelope rather than isolated spikes
(dpayne/cli-visualizer's "monstercat" filter). For every pair of bands
`i, j` at distance `d = |i − j|`:

```
out[j] = max(out[j],  band[i] / MONSTERCAT_FACTOR ^ d)
```

`MONSTERCAT_FACTOR = 1.5` (balanced). `≤ 1.0` disables it. `~1.3` is heavy
and blobby, `~2.5` is a light touch. O(n²) but `n` is a few dozen bands.

## 8. Peak-hold markers

One marker per band (`viz/peaks.go`), drawn only when enabled (`p` /
`-peaks`). A marker captures the band's height, holds `PEAK_HOLD_MS` (150),
then:

- **fade** *(always on)* — once the band goes quiet the marker dims to
  black over `PEAK_FADE_MS` (1200) and stops being drawn. The blend is
  **gamma-bent** so perceived brightness drops evenly instead of hanging
  bright then snapping dark:

  ```
  colour = lerp(peakColour → black,  (Age / PEAK_FADE_MS) ^ PEAK_FADE_GAMMA)
  ```

  `PEAK_FADE_GAMMA = 0.45` (≈ the inverse of display gamma). `Age` resets on
  every new peak, so active bands stay bright. This is the
  "gamma t^0.45 · sRGB" fade — the cheapest option that tracks the eye
  (one `math.Pow`, no colour library).

- **fall** *(toggle `f` / `-fall`, on by default)* — after the hold the
  marker also descends: `height *= PEAK_FALLOFF_WEIGHT` (0.97) per frame,
  clamped to the live level. `0.97 > 0.93` (the bar's release), held
  **strictly above** on purpose, so a falling marker can *never* catch up to
  a falling bar. With `fall` off the marker keeps its captured height and
  only fades.

Once a marker has fully faded, its held height is dropped to the live level
so any later rise can re-capture a fresh one (otherwise only the all-time
loudest bands would ever show a marker).

## 9. Colour

`viz/colors.go` `ledRamp(t)` models an **RGB LED being driven harder** as
`t` goes 0 → 1: a plateau at the base colour below `t = 0.30`, a transition
that reaches the top colour by `t = 0.90` and holds it above that (bars
rarely fill the screen, so the top colour arrives a little early). Both
schemes use the same stop positions.

- **Classic:** green → (add red) yellow at the midpoint → (drop green) red.
- **Synthwave:** cyan → one smooth lerp (drop green, add red, blue held) →
  magenta. No midpoint waypoint — a pure-blue mid read as a hard band.
- **Peak marker** = the top-of-ramp colour (Classic red, Synthwave magenta),
  before it starts fading.

`interpolateColor` blends two hex colours in sRGB, casting each channel to
`float64` *before* subtracting — an earlier version did `float64(uint8 −
uint8)`, which underflowed on a decreasing channel and stuck it at the start
value (why Classic never quite reached red and Synthwave looked stepped).

## 10. Layouts

- **`vertical`** *(default)* — the classic view. Mono, bars rise, frequency
  low→high left→right. Band count from terminal **width**.
- **`butterfly`** — horizontal. Frequency runs low→high **bottom→top**, one
  band per `BAND_ROWS` (2) rows, band count from terminal **height**. It is
  **stereo**: the capturer keeps separate L/R windows, both run through
  `ProcessRaw` + one **shared** AGC step (`NormalizeShared`) so a
  hard-panned mix doesn't blow up one side. Left-channel energy grows left
  from the centre, right grows right, no seam by default
  (`BUTTERFLY_CENTER_GAP = 0`). Each row is coloured by its band index.

Toggle live with `l`. All four bar styles have a horizontal form; the peak
marker becomes a vertical `┃`.

## 11. Auto band count

Recomputed on every terminal resize:

```
vertical:   N = (width + 1) / BAND_COLUMNS      (BAR_WIDTH + 1 = 3)
butterfly:  N = height / BAND_ROWS              (2)
```

clamped to `[MIN_BANDS, MAX_BANDS]` = `[8, 96]`. `model.resize` then rebuilds
the FFT band map and every channel's smoother + peak tracker (the AGC ceiling
is preserved across the rebuild). `-bands N` pins a fixed count and disables
the auto-sizing.

## 12. Bar styles

Cycle with `s`. Default `solid`.

| style | what it draws |
|-------|---------------|
| `solid` | one continuous block column, one colour per bar |
| `led` | one amplitude level per cell row — a `▄` half-block `BAR_WIDTH` wide with a black upper half as the built-in gap (1:1 lit:gap). Coloured by absolute row position. Peak marker is a thin `━━` line. |
| `braille` | vertical braille dot-fill, 4× sub-row resolution, dotted texture and dotted peak marker. Needs a font with U+28xx glyphs. |
| `gradient` | solid column with a vertical brightness gradient of the scheme's peak colour — full at the base, fading to `GRADIENT_TIP_FLOOR` (0.05) at the tip |

## 13. Intro splash (HACKERBOT)

`viz/splash.go` + `viz/splash_scene.go`. A braille halftone of an embedded
still (`viz/hackerbot.jpg` — a Kung Fury riff with a 1950s tin robot and a
chrome "HACKERBOT" wordmark) holds `SPLASH_HOLD_MS` (1600 ms), then
dissolves into dots that **pour into a pool and fade out**. Any key
dismisses it; audio keeps calibrating underneath it.

**Halftone pipeline** (once, ~140 ms at launch, `sync.Once`):

1. Decode (`image.Decode`, PNG/JPEG) → box-average downscale to 1000 px wide.
2. Luminance, then **levels stretch** to the 2nd/98.5th percentiles.
3. **Unsharp mask** (subtract a box blur, add 0.7× the difference back) for
   local contrast.
4. **Radial vignette** centred on the figure — the far office corners sink
   toward black, which in braille is empty space, so the robot and the
   wordmark carry the frame.
5. **Sobel** edge magnitude, folded into the shadows only (`v + 0.42·edge·(1
   − v)` after an S-curve) so the figure keeps its outline but the bright
   wordmark's letterforms stay open.
6. **Atkinson dither** to a 2×4-dots-per-cell grid, packed into U+28xx
   braille glyphs, centred and aspect-corrected to the terminal. Each cell's
   colour is sampled from the source pixels with the chroma pushed up
   (HSL, via go-colorful).

**Pour-and-fade decay** (dot-space, `subX = 2`, `subY = 4`):

- Every lit dot becomes a particle.
- **vertical:** particles fall under `SPLASH_GRAVITY · 1.5` per frame² into a
  puddle `poolDepth = uh/8` dot-rows deep along the bottom; in the puddle
  they damp vertically and jitter sideways so it levels like liquid.
- **butterfly:** particles slide horizontally (`grav · 1.6`) into a column
  `poolHalf = uw/28` wide down the centre axis, then jitter vertically to
  fill it.
- Once `SPLASH_POOL_FRACTION` (0.95) of particles are pooled (after
  `SPLASH_MOVE_MIN_MS`, hard cap `SPLASH_DECAY_MS`), the whole pool fades to
  black over `SPLASH_FADE_MS` (850) with the same `t^0.6` gamma ramp as the
  peak markers, then the splash reports `Done()`.

The halftone and the pour-and-fade decay were prototyped in a standalone
`hackerman-lab` tool that compares eight ways to render the still (block-font
wordmarks through to this scene halftone) and both decay modes side by side.

## Config precedence

```
built-in constants (viz/config.go)
   └─► ~/.config/cyberspice/config.toml
          └─► command-line flags
```

`w` in the app writes the current live settings to `config.toml`. Only keys
actually present in the file override the defaults, so a partial file is
fine. `gain` is snapped to two decimals on read and on every `+`/`-` step.
