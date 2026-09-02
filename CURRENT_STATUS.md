# CyberSpec — Current Status

## Project

Cyberpunk-themed CLI spectrum analyzer in Go. Captures system audio via the
PipeWire/PulseAudio monitor source, renders a log-spaced frequency
visualization with LED-style bars, flickering peak-hold indicators, and two
color schemes.

- Location: `~/code/cyberspec/`
- Module: `cyberspec`
- Branch: `master`
- Build: `go build -o cyberspec` (compiles + `go vet` clean)
- Run: `./cyberspec` (see flags below)

## Command-line flags

| Flag | Values | Default | Notes |
|------|--------|---------|-------|
| `-style` | `led`, `solid`, `braille`, `fibonacci` | `solid` | Bar rendering style |
| `-color` | `classic`, `synthwave` | `synthwave` | Color scheme |
| `-bands` | integer | `0` | `0` = auto-size to terminal width |
| `-gain`  | float | `1.0` | Initial gain multiplier (trim on top of AGC) |
| `-tilt`  | float | `3.0` | Spectral tilt, dB/octave high-freq lift (0 = flat, max 6) |
| `-amp`   | `stevens`, `linear` | `stevens` | Amplitude scale: perceptual loudness vs raw amplitude |

## Working

- **Audio source** — records the default sink's `.monitor` (detected via
  `pactl info`), not the mic.
- **Latency** — capture runs on a dedicated `readLoop` goroutine that drains
  continuously into a rolling `FFT_SIZE`-sample window; the stream is opened
  with an explicit `pa_buffer_attr` (`Fragsize` ~16 ms, `Maxlength` ~130 ms).
  A slow render frame can no longer let audio back up. `ReadSamples()` copies
  the whole window, so the FFT always gets `FFT_SIZE` of real samples (no
  zero-padding).
- **Spectrum shape** — A-weighting is **off** by default
  (`ENABLE_A_WEIGHTING = false`); it was gutting the bass by 20-40 dB. `FFT_SIZE`
  is `4096` (~11.7 Hz bins), `MIN_FREQ` is `30`. Bands narrower than one FFT
  bin sample that bin instead of reading zero. A monstercat spatial spread
  (`viz.SpreadNeighbors`, `MONSTERCAT_FACTOR 1.5`) runs after temporal
  smoothing so the spectrum is a smooth envelope, not isolated spikes.
- **Amplitude scale** — the pipeline is linear in amplitude but the ear is
  not, so raw amplitude makes slightly-louder sounds shoot up. `stevens`
  mode (default) raises the 0-1 display value to `AMPLITUDE_EXPONENT` (0.6,
  Stevens' power law for loudness vs sound pressure) in `renderer.ampValue`,
  applied to both bar and peak heights so bar height ≈ perceived loudness.
  `linear` passes straight through. Toggle with `a`; `-amp` sets the launch
  mode; shown in the header.
- **Spectral tilt** — the middle ground between raw (bass-heavy, no highs)
  and A-weighting (no bass). `dsp/fft.go rebuildTilt` builds a boost-only
  high shelf: `SPECTRAL_TILT_DB_PER_OCT` (1.0) dB/octave above `MIN_FREQ`,
  uniform slope all the way up (no cap — a cap made higher tilt values
  *drop* the highs, since above the cap frequency there was no more HF
  compensation and the AGC then followed a mid peak). Applied per band
  before normalize. Live-adjustable with `[` / `]` (0–6), `-tilt` sets the
  launch value, shown in the header. Default `3.0`; higher is brighter and
  the bass shrinks as the AGC follows the highs.
- **Auto gain (AGC)** — cava / cli-visualizer style. `dsp/fft.go
  normalizeBands` keeps one `sensitivity` scalar over all bands: fast
  calibration ramp on launch (`AGC_INIT_UP` until a peak hits `AGC_TARGET`),
  then gentle adaptation — quick pull-down on clip (`AGC_DOWN`), slow lift
  when the picture stays low (`AGC_UP` after `AGC_LOW_FRAMES`), hold on
  silence. The display **breathes** with the music instead of being
  renormalised to full every frame. `-gain` / `+` `-` `0` still trim on top.
- **Auto band count** — `computeBands(width)` = `(width+1)/BAND_COLUMNS`,
  clamped to `[MIN_BANDS, MAX_BANDS]` (8–96). Recomputed on every
  `WindowSizeMsg`; `model.resize` rebuilds the FFT band map, smoother, and
  peak tracker (AGC ceiling preserved). `-bands N` pins a fixed count.
- **Bar animation** — asymmetric smoother in `viz/smooth.go`: fast EMA attack
  (`SMOOTHING_ALPHA 0.7`), exponential release (`BAR_FALLOFF_WEIGHT 0.93`,
  clamped to the live level). Matches the dpayne/cli-visualizer feel.
- **Peak indicator** — per-frame exponential decay
  (`PEAK_FALLOFF_WEIGHT 0.97`). Held **strictly above** `BAR_FALLOFF_WEIGHT`
  so a falling peak can never descend onto a falling bar; it only rejoins the
  bar when fresh audio pushes the bar up. Plus hold time and flicker.
- **Bar styles** (default `solid`, cycle with `s`)
  - `solid` — one continuous block column, one color per bar.
  - `led` — lit `██` blocks (`LED_SEGMENT_ROWS 2`) with unlit gaps
    (`LED_GAP_ROWS 1`), each row colored by absolute vertical position
    (green low → red high).
  - `braille` — vertical braille dot-fill, 4× sub-row resolution, dotted
    texture, dotted peak marker. Needs a font with U+28xx glyphs.
  - `fibonacci` — the old Fibonacci-gap fragmentation path (still needs the
    threshold rework to look right; see `docs/decay-inspiration.png`).

## Key files

| File | Role |
|------|------|
| `main.go` | Bubbletea loop, `parseFlags`, `computeBands`, `model.resize`, monstercat spread, key handling |
| `audio/capture.go` | Monitor-source detect, `pa_buffer_attr`, `readLoop` goroutine, rolling FFT window |
| `dsp/fft.go` | FFT, Hann window, optional A-weighting, **AGC** normalize, `SetNumBands` |
| `dsp/bands.go` | Log-spaced band mapping, parameterized by `numBands`, sub-bin sampling |
| `viz/config.go` | All tunable constants |
| `viz/renderer.go` | Bar styles (`led`/`solid`/`braille`/`fibonacci`), `CycleBarStyle`, peak, transpose |
| `viz/smooth.go` | Asymmetric attack/release bar smoother + `SpreadNeighbors` (monstercat) |
| `viz/peaks.go` | Peak-hold + exponential fall + flicker |
| `viz/colors.go` | Classic / Synthwave gradients, `interpolateColor` |

## Open / backlog

- `docs/CONFIGURATION.md` and `docs/ALGORITHMS.md` still reference the removed
  `DECAY_STYLE` and `PEAK_DECAY_RATE` constants — replace with `BAR_STYLE` /
  `PEAK_FALLOFF_WEIGHT` / `BAR_STYLE = "fibonacci"`.
- `fibonacci` style: fragmentation threshold keys off raw normalized
  magnitude; needs to key off per-bar height before it looks like the
  reference screenshot.
- Peak `Faint(true)` at low opacity is the only flicker dimming — no true
  per-channel RGB alpha.

## Keyboard controls

| Key | Action |
|-----|--------|
| `q` / `ESC` / `Ctrl+C` | Quit |
| `c` / `1` / `2` | Cycle / set color scheme |
| `s` | Cycle bar style (led → solid → braille → fibonacci) |
| `a` | Toggle amplitude scale (stevens ↔ linear) |
| `[` / `]` | Spectral tilt ∓ / ± 0.5 dB/oct (0–6) |
| `+` / `-` | Gain ±0.1 |
| `0` | Reset gain to the launch value |
