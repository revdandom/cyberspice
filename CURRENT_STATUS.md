# CyberSpec — Current Status

## Project

Cyberpunk-themed CLI spectrum analyzer in Go. Captures system audio via
PipeWire/PulseAudio monitor source, renders 16-band frequency visualization
with Fibonacci decay, flickering peaks, and dual color schemes.

- Location: `~/code/cyberspec/`
- Module: `cyberspec`
- Branch: `master` (3 commits; uncommitted changes: monitor-source detection,
  buffer-attr + goroutine reader, solid bars, gain 1.0)
- Build: `go build -o cyberspec` (compiles + `go vet` clean)

## What Works

- Audio capture from the correct playback device (monitor source auto-detected
  via `pactl info` → default sink + `.monitor`)
- FFT pipeline (Hann window, A-weighting, 16 log-spaced bands)
- Bubbletea TUI loop at 30 FPS target
- Color schemes (classic green→red, synthwave cyan→magenta)
- Peak tracking with flicker (peak bar slide-down confirmed working)
- Keyboard controls (gain, scheme, quit)
- Solid bottom-anchored bars (`DECAY_STYLE = "solid"`)
- Bounded-latency capture: dedicated reader goroutine + `pa_buffer_attr` cap

## Recently Fixed (needs on-machine verification with music playing)

### 1. ~5-second audio→visualizer lag

**Root cause:** `tickCmd()` reschedules itself *after* `processAudio()` +
render complete (`main.go:111`), so the true frame period is `33ms +
processing`, while every frame still consumes exactly 33ms of audio. The
consumer falls slightly behind the 48 kHz producer each frame; the backlog
grew until it hit PipeWire's default `Maxlength` (MB-scale) and pinned there
— a permanent multi-second latency. Peak animation still looked smooth
because it is `deltaMs`-driven; it was just showing audio from ~5s ago.

**CORRECTION to earlier notes:** the claim that `pulse-simple` cannot set
`pa_buffer_attr` was wrong. `mesilliac/pulse-simple` ships `bufferattr.go`,
and `NewStream`'s 8th param **is** `battr *BufferAttr` (the old code passed
`nil`, and the diff comment mislabeled it "format").

**Fix applied (`audio/capture.go`):**
- Pass `pulse.NewBufferAttr()` with `Fragsize = chunkBytes/2` (~16ms) and
  `Maxlength = chunkBytes*4` (~130ms hard ceiling — server drops old samples
  on overrun instead of accumulating lag).
- Move the blocking `stream.Read()` into `readLoop`, a dedicated goroutine
  that reads back-to-back and publishes only the newest mono buffer.
  `ReadSamples()` now returns that snapshot without blocking, fully
  decoupling audio capture from render speed.
- Clean shutdown: `Close()` signals `done`, waits for `readLoop` to return
  (200ms safety timeout), then `stream.Free()` — no concurrent free/read.

**Still open (quality, not lag):** `BUFFER_SIZE` (1600) ≠ `FFT_SIZE` (2048),
so `prepareFFTInput()` (`dsp/fft.go:168`) zero-pads 22% of each window.

### 2. Broken / fragmented vertical bars

**Root cause:** the uncommitted `viz/renderer.go` re-enabled Fibonacci decay
(`energyPercent < 90` → `renderFibonacciDecay`). Combined with the
uncommitted `DEFAULT_GAIN = 0.7`, gain-adjusted `magnitude` caps at 0.7 so
`energyPercent` never exceeds 70 — **every band, every frame** was chopped
into 2–3 line segments with gaps. The solid path was unreachable. (Even at
gain 1.0, per-frame peak normalization in `normalizeBands` means only the
single loudest band reaches 100%; all others still fragment.)

**Fix applied:**
- `viz/config.go`: `DECAY_STYLE = "solid"` → all bars route to
  `renderSolidBar`, bottom-anchored.
- `viz/config.go`: `DEFAULT_GAIN` reverted `0.7 → 1.0`.
- `viz/renderer.go` Fibonacci re-enable kept in place but dormant (guarded by
  `DECAY_STYLE == "fibonacci_gaps"`).

**Not reverted:** `PEAK_DECAY_RATE = 5.0`, `PEAK_HOLD_MS = 150` (peak
slide-down confirmed working; revisit if peaks linger too long — try
`PEAK_DECAY_RATE` ~8–10).

## File Inventory

| File | Purpose | Status |
|------|---------|--------|
| `main.go` | Bubbletea app loop, key handling | Working |
| `audio/capture.go` | PulseAudio capture, monitor detection, goroutine reader + buffer-attr cap | Working (verify lag on machine) |
| `dsp/fft.go` | FFT, Hann window, A-weighting, normalization | Working (zero-pad issue) |
| `dsp/bands.go` | 16 log-spaced frequency band mapping | Working |
| `dsp/weighting.go` | A-weighting curve | Working |
| `viz/config.go` | All tunable constants | `DECAY_STYLE="solid"`, `DEFAULT_GAIN=1.0` |
| `viz/renderer.go` | Bar rendering, Fibonacci decay (dormant), colors | Working |
| `viz/peaks.go` | Peak tracking, flicker, decay | Working |
| `viz/smooth.go` | EMA smoothing (α=0.4) | Working |
| `docs/decay-inspiration.png` | Reference image for Fibonacci decay | Reference |
| `README.md` | Project documentation | Current |

## Next Steps (for Claude)

### Priority 1: Verify the lag fix

Run `./cyberspec` with music playing and confirm the visualizer now tracks
audio within ~150ms. If lag persists:
- Lower `battr.Fragsize` / `battr.Maxlength` further in `audio/capture.go`.
- Check `readLoop` is actually keeping up (CPU, no dropped-sample spam).

### Priority 2: Re-tune Fibonacci decay (optional, aesthetic)

The `docs/decay-inspiration.png` look needs the fragmentation threshold to
key off *per-bar height* rather than raw normalized magnitude, and taller
segments. Only then flip `DECAY_STYLE` back to `"fibonacci_gaps"`.

### Priority 3: Code cleanup

- `BUFFER_SIZE` and `FFT_SIZE` should be equal (or BUFFER_SIZE should be a
  multiple of FFT_SIZE). Currently 1600 vs 2048.
- `prepareFFTInput()` in `dsp/fft.go:168-187` zero-pads — this is correct
  behavior but wasteful when BUFFER_SIZE < FFT_SIZE.

## Build & Run

```bash
cd ~/code/cyberspec
go build -o cyberspec
./cyberspec
```

## Keyboard Controls

| Key | Action |
|-----|--------|
| `q` / `ESC` / `Ctrl+C` | Quit |
| `c` | Cycle color scheme |
| `1` | Classic scheme |
| `2` | Synthwave scheme |
| `+` / `=` | Increase gain (+0.1) |
| `-` / `_` | Decrease gain (-0.1) |
| `0` | Reset gain to default |
