# CLAUDE.md — cyberspice

Applies to sessions working in this repo. `~/ai/CLAUDE.md` applies on top of this.

## Build

When code changes are made, run `go build ./...` (and `go vet ./...`) automatically
right after — don't wait to be asked, and don't skip it because a change looks
trivial.

## Audio capture must stay passive

CyberSpice is eye candy and must never change what the user hears. `audio/capture.go` runs
`pw-record` with `stream.capture.sink = true` (follow the default sink's monitor),
`node.passive = true` (never keep the output device awake) and `node.rate = ""` (no
preferred rate; PipeWire resamples only our copy to 48 kHz). The old pulse-simple record
stream, still the fallback when `pw-record` is missing, held the user's FiiO K11 R2R at
48 kHz and made it toggle against their rate-resync watcher (2026-10-02). Don't add stream
properties or a fixed device target that would let the capture influence playback.
The installed copy is `~/.local/bin/cyberspice` (a plain copy, not a link): rebuild with
`go build -o cyberspice .` and copy it there.
