# CyberSpec - Complete Implementation Plan

## Project Overview

**Name:** `cyberspec`  
**Location:** `~/code/cyberspec/`  
**Purpose:** Cyberpunk CLI spectrum analyzer for system audio on Linux (PipeWire/Hyprland)  
**Language:** Go with modules  
**Target Terminals:** Ghostty, Kitty, Alacritty (all support Unicode block characters)  
**Target FPS:** 30 frames per second

---

## Technical Specifications

### Audio Capture

- **Backend:** PipeWire 1.6.8 via PulseAudio Simple API
- **Source:** Monitor of default sink (system audio output)
- **Sample Rate:** 48000 Hz
- **Format:** Float32LE stereo
- **Buffer Size:** 1600 samples (~33ms at 30 FPS)

### DSP Processing

#### FFT Configuration
- **FFT Size:** 2048 samples
- **Window Function:** Hann window (reduces spectral leakage)
- **Frequency Range:** 20 Hz - 20 kHz (human hearing range)

#### Frequency Band Distribution (16 Logarithmic Bands)

```
Band  1:    20 -    40 Hz  (Sub-bass)
Band  2:    40 -    80 Hz  (Sub-bass)
Band  3:    80 -   160 Hz  (Bass)
Band  4:   160 -   250 Hz  (Bass)
Band  5:   250 -   400 Hz  (Low-mid)
Band  6:   400 -   630 Hz  (Low-mid)
Band  7:   630 -  1000 Hz  (Mid)
Band  8:  1000 -  1600 Hz  (Mid)
Band  9:  1600 -  2500 Hz  (Mid)
Band 10:  2500 -  4000 Hz  (High-mid)
Band 11:  4000 -  6300 Hz  (Presence)
Band 12:  6300 - 10000 Hz  (Presence)
Band 13: 10000 - 14000 Hz  (Brilliance)
Band 14: 14000 - 16000 Hz  (Brilliance)
Band 15: 16000 - 18000 Hz  (Air)
Band 16: 18000 - 20000 Hz  (Air)
```

#### A-Weighting Curve

**Purpose:** Compensate for bass-heavy raw FFT, create balanced visual representation

**Why it's needed:**
- Raw FFT magnitudes are bass-heavy because bass frequencies have more physical energy
- Lower frequencies tend to have higher amplitudes in music
- Human ears perceive loudness differently across frequencies
- Without correction, bass dominates the visualization

**Formula:** Applied per frequency bin before band aggregation

```
A(f) = 20 × log10(
  (12194² × f⁴) / 
  ((f² + 20.6²) × √((f² + 107.7²) × (f² + 737.9²)) × (f² + 12194²))
)
```

**Implementation:** Pre-calculate multipliers for each FFT bin on startup, apply during magnitude calculation

See `docs/ALGORITHMS.md` for detailed explanation and alternatives.

#### Temporal Smoothing

**Purpose:** Reduce bar jitter, create smooth motion

**Algorithm:** Exponential Moving Average (EMA)

```
smoothed[i] = (alpha × current[i]) + ((1 - alpha) × previous[i])
```

**Alpha value:** 0.4 (medium smoothing - default)

See `docs/CONFIGURATION.md` for all smoothing options and how to adjust.

---

## Visualization Design

### Bar Rendering

- **Characters:** `█▇▆▅▄▃▂▁` (8 intensity levels)
- **Spacing:** Medium - 3 chars per band: `██  ██  ██`
- **Height:** Auto-scale to terminal height (minus header/footer space)
- **Width:** 16 bands × 3 chars = 48 chars minimum
- **Auto-scaling:** Bars adapt to terminal dimensions on resize

### Fibonacci Decay Pattern

**Inspiration:** User-provided screenshot (`docs/decay-inspiration.png`)

**Concept:** As bars decay, they fragment into horizontal segments with vertical gaps that grow following the Fibonacci sequence, creating a "disintegrating" cyberpunk effect.

**Visual Pattern:**
```
100-90%: Solid bar (gap = 0)
 90-70%: 1 line gap between segments
 70-50%: 2 line gap
 50-30%: 3 line gap
 30-15%: 5 line gap
 15-5%:  8 line gap
  5-0%:  13 line gap (barely visible fragments)
```

**Example:**
```
Full energy:     ████     Medium decay:   ████     Low energy:     ████
                 ████                     
                 ████                     ████     
                 ████                              
                 ████                     ████                     ████
```

See `docs/ALGORITHMS.md` for:
- Complete algorithm details
- Alternative decay patterns (documented but not implemented)
- How to modify or change decay behavior

### Peak Bar Behavior

**Character:** `━` (heavy horizontal line)  
**Position:** One line above current bar top  
**Color:** Matches bar color scheme but brighter

**Lifecycle:**
1. **Capture:** Jumps to new maximum when band reaches peak
2. **Hold:** Stays at maximum for 500ms
3. **Decay:** Falls at 15 units/second
4. **Flicker:** Random flicker effect throughout lifecycle

**Flicker Effect:**

**Inspiration:** "Light bulb flickering out when it fails"

- **Frequency:** Random 3-5 Hz (200-333ms between flickers)
- **Opacity:** Random jumps with overall decreasing trend
- **Brightness:** Alternates between bright/dim of same color
- **Behavior:** Creates stuttering, failing light effect

See `docs/ALGORITHMS.md` for complete flicker algorithm details.

---

## Color Schemes

### Scheme 1: Classic (Green → Yellow → Red)

**Bar Colors by Height:**
```
 0-33%:  Green shades   (#00FF00 → #88FF00)
34-66%:  Yellow/Orange  (#FFFF00 → #FF8800)
67-100%: Red shades     (#FF4400 → #FF0000)
```

**Peak Bar:** Bright cyan (#00FFFF) or white (#FFFFFF)

### Scheme 2: Synthwave (Cyan → Magenta)

**Bar Colors by Height:**
```
 0-25%:  Deep cyan       (#00AAAA → #00DDFF)
26-50%:  Cyan-Purple    (#00DDFF → #8800FF)
51-75%:  Purple-Magenta (#8800FF → #DD00DD)
76-100%: Hot Magenta    (#DD00DD → #FF00FF)
```

**Peak Bar:** Bright magenta (#FF00FF) or electric pink (#FF10F0)

---

## User Controls

```
c or 1/2:  Cycle color schemes (1=Classic, 2=Synthwave)
+ or =:    Increase gain (+0.1x)
- or _:    Decrease gain (-0.1x)
0:         Reset gain to 1.0x
q or ESC:  Quit
Ctrl+C:    Force quit
```

**Gain Range:** 0.1x to 5.0x (default: 1.0x)  
**Gain Display:** Shows current gain in header (e.g., "Gain: 1.5x")

---

## Project Structure

```
~/code/cyberspec/
├── go.mod
├── go.sum
├── main.go                    # Entry point, Bubbletea setup
├── docs/
│   ├── decay-inspiration.png  # User-provided screenshot
│   ├── IMPLEMENTATION_PLAN.md # This file
│   ├── ALGORITHMS.md          # Detailed algorithm documentation
│   └── CONFIGURATION.md       # All configurable constants
├── audio/
│   └── capture.go             # PipeWire/PulseAudio capture
├── dsp/
│   ├── fft.go                 # FFT processing
│   ├── bands.go               # Frequency band mapping
│   └── weighting.go           # A-weighting curve calculation
├── viz/
│   ├── config.go              # All configurable constants
│   ├── renderer.go            # Bar rendering with decay logic
│   ├── peaks.go               # Peak detection, decay, flicker
│   ├── colors.go              # Color scheme definitions
│   └── smooth.go              # Temporal smoothing (EMA)
└── README.md                  # Usage and build instructions
```

---

## Dependencies

```go
module cyberspec

go 1.21

require (
    github.com/charmbracelet/bubbletea v0.27.0
    github.com/charmbracelet/lipgloss v0.13.0
    github.com/mesilliac/pulse-simple v0.0.0-20170506101341-75ac54e19fdf
    github.com/mjibson/go-dsp v0.0.0-20180508042940-11479a337f12
)
```

**Alternative PulseAudio libraries** (if mesilliac doesn't work):
- `github.com/lawl/pulseaudio`
- `github.com/jfreymuth/pulse`

---

## Performance Targets

**Target:** Maintain 30 FPS consistently

**Optimizations:**
- Reuse FFT buffers (no allocation per frame)
- Pre-calculate A-weighting multipliers on startup
- Cache terminal dimensions (only recalculate on resize)
- Use efficient Lipgloss style caching
- Minimal allocations in render loop

**Expected CPU Usage:** <5% on modern CPU (single core)

---

## Development Phases

### Phase 1: Setup & Documentation ✓
- Project structure created
- Go module initialized
- Documentation written

### Phase 2: Audio Foundation
- PipeWire/PulseAudio connection
- Audio stream capture
- Buffer management

### Phase 3: DSP Processing
- FFT implementation with Hann window
- A-weighting curve calculation
- 16-band logarithmic mapping
- Magnitude calculation

### Phase 4: Smoothing & Gain
- EMA smoothing implementation
- Manual gain control
- Normalization

### Phase 5: Basic Visualization
- Simple solid bar rendering
- Color scheme integration
- Bubbletea TUI setup
- Terminal auto-scaling

### Phase 6: Advanced Effects
- Fibonacci decay algorithm
- Segment fragmentation
- Character selection by energy

### Phase 7: Peak System
- Peak detection and tracking
- Peak hold and decay
- Flicker algorithm implementation
- Random opacity and brightness

### Phase 8: Polish & Testing
- Keyboard controls
- Header/footer display
- Performance optimization
- Real-world audio testing
- FPS verification

---

## Future Enhancement Ideas

*These are documented for future consideration but not implemented in POC:*

1. **Per-band gain adjustment** - Individual frequency band gain controls
2. **Additional color schemes** - More cyberpunk palettes
3. **Config file support** - Load settings from TOML/YAML
4. **Alternative decay patterns** - Switchable decay algorithms
5. **Stereo separation** - Left/right channel visualization
6. **Waveform mode** - Time-domain visualization option
7. **Recording mode** - Save visualization to video/GIF
8. **Beat detection** - Pulse effects on beat
9. **Preset system** - Save/load visualization presets
10. **MIDI control** - External controller support

---

## Troubleshooting

### Audio Issues

**No audio capture:**
- Verify PipeWire is running: `ps aux | grep pipewire`
- Check monitor source exists: `pactl list sources | grep monitor`
- Ensure audio is playing (test with `paplay` or music player)

**Low/no visualization:**
- Increase gain with `+` key
- Check system volume is not muted
- Verify audio is actually playing

### Performance Issues

**FPS drops below 30:**
- Reduce FFT size in config (trade quality for speed)
- Disable A-weighting temporarily
- Increase smoothing alpha (less responsive but faster)
- Check CPU usage with `htop`

**High CPU usage:**
- Check for memory leaks in render loop
- Verify FFT buffer reuse
- Profile with `go tool pprof`

### Visual Issues

**Bars too sensitive/jumpy:**
- Decrease smoothing alpha (more smoothing)
- Reduce gain
- Adjust A-weighting curve

**Bars too slow to respond:**
- Increase smoothing alpha (less smoothing)
- Increase gain

**Peak bars not visible:**
- Verify peak hold time is sufficient
- Check peak color contrast
- Adjust flicker opacity range

---

## License

*To be determined by project owner*

---

## Credits

**Concept & Design:** User specifications  
**Decay Pattern Inspiration:** User-provided screenshot  
**Implementation:** OpenCode  
**Audio System:** PipeWire/PulseAudio  
**TUI Framework:** Charm Bracelet (Bubbletea, Lipgloss)  
**DSP Library:** go-dsp (mjibson)

---

*Last Updated: September 1, 2026*
