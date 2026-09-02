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
| `-style` | `led`, `solid`, `braille`, `fibonacci` | `led` | Bar rendering style |
| `-color` | `classic`, `synthwave` | `classic` | Color scheme |
| `-bands` | integer | `0` | `0` = auto-size to terminal width |
| `-gain`  | float | `1.0` | Initial gain multiplier (trim on top of AGC) |

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
- **Auto gain (AGC)** — `dsp/fft.go normalizeBands` tracks a running loudness
  ceiling (fast smoothed attack, slow release, noise gate) and scales bands
  against it. The visual fills the height correctly seconds after launch
  without touching the gain keys. `-gain` / `+` `-` `0` still trim on top.
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
- **Bar styles**
  - `led` — lit `██` blocks (`LED_SEGMENT_ROWS 2`) with unlit gaps
    (`LED_GAP_ROWS 1`), each row colored by absolute vertical position
    (green low → red high).
  - `braille` — vertical braille dot-fill, 4× sub-row resolution, dotted
    texture, dotted peak marker. Needs a font with U+28xx glyphs.
  - `solid` — one continuous block column, one color per bar.
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
- No perceptual tilt now that A-weighting is off — raw FFT is slightly
  bass-heavy on some material. A gentle high-shelf (a few dB/octave) would
  balance it without killing the low end.

## Keyboard controls

| Key | Action |
|-----|--------|
| `q` / `ESC` / `Ctrl+C` | Quit |
| `c` / `1` / `2` | Cycle / set color scheme |
| `s` | Cycle bar style (led → solid → braille → fibonacci) |
| `+` / `-` | Gain ±0.1 |
| `0` | Reset gain to the launch value |
