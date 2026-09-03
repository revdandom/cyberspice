# CyberSpice

A cyberpunk-flavoured CLI spectrum analyser for Linux. It captures whatever
your speakers are playing and draws it as a log-spaced frequency
visualisation — auto-sized to the terminal, with peak-hold markers that fall
and then fade out, two colour schemes, four bar styles, a horizontal
"butterfly" stereo layout, and a braille-halftone intro splash.

<!-- A screenshot or GIF would go well here. -->

## Unapologetically vibe-coded

This was built conversationally with an AI coding agent and tuned by
screenshot and feel rather than by spec. It runs well on the author's setup
(EndeavourOS + Hyprland, Kitty/Ghostty). Treat it as a fun artifact, not a
reference implementation — the DSP is "close enough to look good", not
"correct".

The motion — attack/release pacing, and peak markers that fall about half as
fast as the bars before fading — is modelled on
[**vis-cli-visualizer**](https://github.com/dtoraelek/vis-cli-visualizer).
The auto-gain is [**cava**](https://github.com/karlstav/cava)-style; the
"monstercat" spatial smoothing comes from
[**dpayne/cli-visualizer**](https://github.com/dpayne/cli-visualizer).

## Features

- **Live capture** of the default sink's monitor source (PipeWire /
  PulseAudio), low-latency — capture runs on its own goroutine with an
  explicit buffer size so a slow frame can't make audio lag.
- **Auto-sized** band count — fills the terminal, re-flows on resize.
- **Bar styles:** `solid`, `led` (segmented), `braille` (4× sub-row), `gradient`.
- **Layouts:** `vertical` (classic) and `butterfly` (horizontal, stereo — low
  freq at the bottom, left channel grows left, right grows right).
- **Peak markers** that hold, fall, then fade to black with a gamma-corrected
  ramp so the fade looks even. Falling is toggleable; markers can't catch a
  falling bar.
- **cava-style auto-gain** — the display *breathes* with the music instead of
  being renormalised to full scale every frame. Manual trim on top.
- **Spectral tilt** — a boost-only high shelf, the middle ground between raw
  FFT (bass-heavy) and A-weighting (bass gone). Live-adjustable.
- **Loudness curve** — `linear`, `stevens` (perceptual power law), or a fixed
  `db` window.
- **Two colour schemes** — Classic (green→yellow→red) and Synthwave
  (cyan→magenta), driven like an RGB LED.
- **HACKERBOT intro splash** — a braille halftone of a 1950s-tin-robot still
  that dissolves into a pool of dots and fades before the visualiser starts.
- **TOML config** at `~/.config/cyberspice/config.toml`, written in-app with `w`.

Full detail and the maths: **[docs/how-it-works.md](docs/how-it-works.md)**.

## Requirements

- **Linux** with **PipeWire** or **PulseAudio** running (it records the
  default output's `.monitor` source).
- **Go 1.27+** (see `go.mod`).
- **A C toolchain + libpulse headers** — the audio binding is cgo:
  - Arch / EndeavourOS: `sudo pacman -S libpulse base-devel`
  - Debian / Ubuntu: `sudo apt install libpulse-dev build-essential`
- A **truecolor (24-bit) terminal** — Kitty, Alacritty, Ghostty, WezTerm,
  foot, modern xterm. For the `braille` style and the intro splash you also
  want a font with **braille (U+2800–28FF)** coverage — any Nerd Font,
  JetBrains Mono, Cascadia Code, Fira Code, etc.

## Build

```bash
git clone <this-repo> cyberspice && cd cyberspice
go build -o cyberspice .
./cyberspice

# optional
sudo install -m755 cyberspice /usr/local/bin/
```

The Kung Fury robot still is embedded in the binary (`viz/hackerbot.jpg`,
~340 KB), so there are no runtime assets.

## Usage

```
./cyberspice [flags]
```

### Flags

| Flag | Values | Default | Notes |
|------|--------|---------|-------|
| `-style` | `led` `solid` `braille` `gradient` | `solid` | bar rendering style |
| `-color` | `classic` `synthwave` | `synthwave` | colour scheme |
| `-layout` | `vertical` `butterfly` | `vertical` | butterfly = horizontal, stereo split |
| `-bands` | integer | `0` | `0` = auto-size to the terminal |
| `-curve` | `linear` `stevens` `db` | `stevens` | loudness curve (amplitude → bar height) |
| `-tilt` | float | `3.0` | spectral tilt, dB/octave high-freq lift (0 = flat, max 6) |
| `-gain` | float | `1.0` | manual gain trim on top of the auto-gain |
| `-chrome` | bool | `true` | show the header/footer bars on startup |
| `-peaks` | bool | `true` | draw the peak markers |
| `-fall` | bool | `true` | peak markers fall after the hold (false = fade only) |
| `-splash` | bool | `true` | show the HACKERBOT intro |

Precedence: built-in defaults → `~/.config/cyberspice/config.toml` → flags.

### Keys

| Key | Action |
|-----|--------|
| `c` / `1` / `2` | cycle / set colour scheme |
| `s` | cycle bar style |
| `l` | cycle layout (vertical ↔ butterfly) |
| `a` | cycle loudness curve (linear → stevens → db) |
| `p` | toggle peak markers |
| `f` | toggle peak-marker falling (off = fade only) |
| `[` / `]` | spectral tilt − / + 0.5 dB/oct |
| `+` / `-` | gain ± 0.1 |
| `0` | reset gain to the launch value |
| `w` | write current settings to `~/.config/cyberspice/config.toml` |
| any other key | toggle the header/footer bars (or dismiss the splash) |
| `q` / `Esc` / `Ctrl+C` | quit |

## Configuration

Every tunable is a commented constant in
[`viz/config.go`](viz/config.go) — sample rate, FFT size, frequency range,
smoothing weights, AGC behaviour, peak timings, colour stops, splash timings.
Change one, `go build`, run.

Runtime overrides live in `~/.config/cyberspice/config.toml`:

```toml
style  = "led"
color  = "classic"
layout = "butterfly"
curve  = "db"
tilt   = 4.5
chrome = true
```

Press `w` in the app to write your current live settings there. If it's
missing, the loader falls back to an extensionless `config` in the same
directory, and then to `~/.config/cyberspec/` (the project's pre-rename
name), so an old config keeps working until your next `w`.

## Layout

```
cyberspice/
├── main.go            Bubble Tea model/update/view, flags, key handling
├── config_file.go     TOML load / save
├── audio/capture.go   monitor-source detection, low-latency capture loop
├── dsp/
│   ├── fft.go         Hann window, FFT, spectral tilt, auto-gain
│   ├── bands.go       log-spaced FFT-bin → band mapping
│   └── weighting.go   A-weighting curve (off by default)
├── viz/
│   ├── config.go      every tunable constant, commented
│   ├── renderer.go    bar styles, header/footer, butterfly layout
│   ├── smooth.go      attack/release smoother + monstercat spread
│   ├── peaks.go       peak-hold + fall + fade
│   ├── colors.go      RGB-LED colour ramp, peak colours, blending
│   ├── splash.go      HACKERBOT intro: scene hold + pool/fade decay
│   └── splash_scene.go  still embed + braille-halftone pipeline
└── docs/
    ├── how-it-works.md   the signal path and the maths
    └── ideas.md          parking lot
```

## Troubleshooting

- **Bars don't move** — check something is actually playing, and that a
  monitor source exists: `pactl list sources | grep monitor`. Try `+` a few
  times to raise the gain trim.
- **Build fails on `pulse-simple`** — install the libpulse dev package and a
  C compiler (see Requirements).
- **Garbled blocks / no colour** — use a truecolor terminal; for the
  `braille` style and the splash, a braille-capable font.
- **Choppy** — lower `TARGET_FPS` or `FFT_SIZE` in `viz/config.go`.

## Credits

- Motion / decay feel — [vis-cli-visualizer](https://github.com/dtoraelek/vis-cli-visualizer)
- Auto-gain — [cava](https://github.com/karlstav/cava)
- Monstercat smoothing + attack/release envelope — [dpayne/cli-visualizer](https://github.com/dpayne/cli-visualizer)
- TUI — [Bubble Tea](https://github.com/charmbracelet/bubbletea) / [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- FFT — [madelynnblue/go-dsp](https://github.com/madelynnblue/go-dsp)
- Colour — [go-colorful](https://github.com/lucasb-eyer/go-colorful)
- Audio binding — [mesilliac/pulse-simple](https://github.com/mesilliac/pulse-simple)
- Splash still — an AI-generated riff on the *Kung Fury* "HACKERMAN" frame.

## License

Not licensed yet. Until a `LICENSE` file lands, standard copyright applies —
ask before reusing.
