package viz

import "math/rand"

// Rain is the "rain" bar style: a digital-rain spectrograph. Single braille
// dots (the same small dots the intro splash uses) spawn at the head of a
// lane and travel down it. Each lane belongs to a frequency, and a dot takes
// its colour from that frequency's loudness AT SPAWN — the scheme's base
// colour when quiet, ramping toward the top-of-ramp colour as the band gets
// louder (GetColorForHeight, same ramp as the bars). The colour is baked into
// the dot, so it keeps it all the way down: the screen reads as a scrolling
// history of the spectrum, newest at the head.
//
//   - vertical: lanes are dot columns, low frequency left → high right (the
//     plain bars' convention); dots fall from the top edge to the bottom.
//   - butterfly: lanes are dot rows, low frequency bottom → high top (the
//     butterfly bars' convention); dots start at the centre seam and stream
//     outward, the left half from the left channel, the right half from the
//     right.
//
// A RainField works in lane/position space (position 0 = the spawn edge);
// the Renderer maps that onto screen dots per layout.

// rdrop is one falling dot.
type rdrop struct {
	pos   float64 // dot units travelled from the spawn edge
	speed float64 // dots/second
	fg    string  // "#rrggbb", baked at spawn
	level float64 // 0..1 curved level at spawn; the hotter dot wins a shared cell
}

// RainField is one independent rain simulation: `lanes` lanes, each `length`
// dots long.
type RainField struct {
	lanes, length int
	drops         [][]rdrop // per lane
	rng           *rand.Rand
}

// rainSeed gives each new field its own seed, so the butterfly halves rain
// independently instead of as exact mirror images.
var rainSeed int64 = 3

// NewRainField builds an empty field of the given size.
func NewRainField(lanes, length int) *RainField {
	rainSeed++
	f := &RainField{rng: rand.New(rand.NewSource(rainSeed))}
	f.Resize(lanes, length)
	return f
}

// Resize clears the field for new dimensions. A no-op if the size hasn't
// changed, so it's safe to call every frame.
func (f *RainField) Resize(lanes, length int) {
	if lanes < 1 {
		lanes = 1
	}
	if length < 1 {
		length = 1
	}
	if lanes == f.lanes && length == f.length && f.drops != nil {
		return
	}
	f.lanes, f.length = lanes, length
	f.drops = make([][]rdrop, lanes)
}

// Update advances every drop by dtSec, culls the ones past the far edge and
// spawns new ones. levelAt(lane) is that lane's frequency's gain-adjusted,
// curved 0..1 level right now; hueFor maps such a level to its colour, ""
// meaning silent (nothing spawns).
func (f *RainField) Update(dtSec float64, levelAt func(lane int) float64, hueFor func(level float64) string) {
	end := float64(f.length)
	for l := range f.drops {
		alive := f.drops[l][:0]
		for _, d := range f.drops[l] {
			d.pos += d.speed * dtSec
			if d.pos < end {
				alive = append(alive, d)
			}
		}
		f.drops[l] = alive

		v := levelAt(l)
		fg := hueFor(v)
		if fg == "" {
			continue // silent lane: no drop
		}
		rate := RAIN_SPAWN_RATE * (1 + v*RAIN_LEVEL_SPAWN_BOOST)
		if f.rng.Float64() >= rate*dtSec {
			continue
		}
		// Never stack a new dot on top of one still sitting at the head.
		if n := len(f.drops[l]); n > 0 && f.drops[l][n-1].pos < RAIN_MIN_GAP {
			continue
		}
		f.drops[l] = append(f.drops[l], rdrop{
			speed: RAIN_MIN_SPEED + f.rng.Float64()*(RAIN_MAX_SPEED-RAIN_MIN_SPEED),
			fg:    fg,
			level: v,
		})
	}
}

// Rasterize packs the drops into a w×h braille cell grid. place maps a
// (lane, dot position) to screen dot coordinates (ux in 0..2w, uy in 0..4h).
func (f *RainField) Rasterize(w, h int, place func(lane, pos int) (ux, uy int)) [][]scell {
	c := newDotCanvas(w, h)
	for l, lane := range f.drops {
		for _, d := range lane {
			ux, uy := place(l, int(d.pos))
			c.plot(ux, uy, d.fg, d.level)
		}
	}
	return c.cells()
}

// dotCanvas accumulates single braille dots into a w×h cell grid. A cell
// can only show one colour, so when several dots share a cell it takes the
// colour of the loudest one.
type dotCanvas struct {
	w, h  int
	masks []byte
	fgs   []string
	lvls  []float64
}

func newDotCanvas(w, h int) *dotCanvas {
	return &dotCanvas{w: w, h: h, masks: make([]byte, w*h), fgs: make([]string, w*h), lvls: make([]float64, w*h)}
}

// plot lights dot (ux, uy); out-of-range dots are ignored.
func (c *dotCanvas) plot(ux, uy int, fg string, level float64) {
	if ux < 0 || ux >= c.w*2 || uy < 0 || uy >= c.h*4 {
		return
	}
	ci := (uy/4)*c.w + ux/2
	c.masks[ci] |= brailleBits[uy%4][ux%2]
	if c.fgs[ci] == "" || level > c.lvls[ci] {
		c.fgs[ci], c.lvls[ci] = fg, level
	}
}

func (c *dotCanvas) cells() [][]scell {
	out := make([][]scell, c.h)
	for y := range out {
		out[y] = make([]scell, c.w)
		for x := range out[y] {
			if m := c.masks[y*c.w+x]; m != 0 {
				out[y][x] = scell{r: rune(0x2800 + int(m)), fg: c.fgs[y*c.w+x]}
			}
		}
	}
	return out
}

// renderDotFieldVertical draws f full-screen: lane = dot column, position 0
// at the top edge.
func (r *Renderer) renderDotFieldVertical(f *RainField, w, h int, gain float64, schemeName string, peakFall bool) string {
	body := renderCellGrid(f.Rasterize(w, h, func(lane, pos int) (int, int) { return lane, pos }), w, h)
	return r.frameBody(body, gain, schemeName, peakFall)
}

// renderDotFieldButterfly draws two halfW-wide fields: lane = dot row,
// position 0 at the centre seam, the left field streaming leftward and the
// right field rightward.
func (r *Renderer) renderDotFieldButterfly(left, right *RainField, halfW, h int, gain float64, schemeName string, peakFall bool) string {
	length := halfW * 2
	lc := left.Rasterize(halfW, h, func(lane, pos int) (int, int) { return length - 1 - pos, lane })
	rc := right.Rasterize(halfW, h, func(lane, pos int) (int, int) { return pos, lane })
	return r.frameButterfly(lc, rc, halfW, h, gain, schemeName, peakFall)
}

// rainLevelAt maps a lane to its band's gain-adjusted, curved level. Lane 0
// is band 0 unless `reversed`, where lane 0 is the highest band (butterfly
// lanes run top → bottom but frequency runs bottom → top).
func rainLevelAt(bands []float64, lanes int, gain float64, curve func(float64) float64, reversed bool) func(lane int) float64 {
	n := len(bands)
	return func(lane int) float64 {
		if n == 0 {
			return 0
		}
		frac := 0.0
		if lanes > 1 {
			frac = float64(lane) / float64(lanes-1)
		}
		if reversed {
			frac = 1 - frac
		}
		idx := int(frac*float64(n-1) + 0.5)
		v := bands[idx] * gain
		if v > 1 {
			v = 1
		}
		return curve(v)
	}
}

// ensureRain lazily creates (or resizes) one of the renderer's rain fields.
func ensureRain(f **RainField, lanes, length int) *RainField {
	if *f == nil {
		*f = NewRainField(lanes, length)
	} else {
		(*f).Resize(lanes, length)
	}
	return *f
}

// UpdateRain advances the rain field(s) one frame. vertical runs one field
// from bandsL (the mono mix — vertical rain doesn't need stereo); butterfly
// runs one per channel.
func (r *Renderer) UpdateRain(layout string, bandsL, bandsR []float64, deltaMs int64, gain float64) {
	dt := float64(deltaMs) / 1000.0
	if dt <= 0 {
		dt = 1.0 / TARGET_FPS
	}
	if dt > 0.25 {
		dt = 0.25
	}
	hueFor := func(v float64) string { return dotHue(r.scheme, v) }

	w, h, mirrored := r.constellationDims(layout)
	if mirrored {
		lanes, length := h*4, w*2
		ensureRain(&r.rainL, lanes, length).Update(dt, rainLevelAt(bandsL, lanes, gain, r.ampValue, true), hueFor)
		ensureRain(&r.rainR, lanes, length).Update(dt, rainLevelAt(bandsR, lanes, gain, r.ampValue, true), hueFor)
		return
	}
	lanes, length := w*2, h*4
	ensureRain(&r.rainMono, lanes, length).Update(dt, rainLevelAt(bandsL, lanes, gain, r.ampValue, false), hueFor)
}

// RenderRain draws the vertical-layout rain: lane = dot column, falling down.
func (r *Renderer) RenderRain(gain float64, schemeName string, peakFall bool) string {
	w, h, _ := r.constellationDims("vertical")
	if h < 10 {
		return "Terminal too small - need at least 13 lines"
	}
	return r.renderDotFieldVertical(ensureRain(&r.rainMono, w*2, h*4), w, h, gain, schemeName, peakFall)
}

// RenderRainButterfly draws the two halves: lane = dot row, streaming outward
// from the centre seam (left half leftward, right half rightward).
func (r *Renderer) RenderRainButterfly(gain float64, schemeName string, peakFall bool) string {
	halfW, h, _ := r.constellationDims("butterfly")
	if h < 10 {
		return "Terminal too small - need at least 13 lines"
	}
	lanes, length := h*4, halfW*2
	return r.renderDotFieldButterfly(ensureRain(&r.rainL, lanes, length), ensureRain(&r.rainR, lanes, length),
		halfW, h, gain, schemeName, peakFall)
}
