package viz

import "strings"

// Waterfall is the "waterfall" bar style: a falling spectrograph. Every cell
// shows one glyph in one colour. Every WATERFALL_FRAMES_PER_ROW frames, each
// lane's frequency is sampled into a new row of colours at the spawn edge
// (see dotHue: blank when silent, fading in to the base colour when
// quiet, up the scheme ramp as it gets louder) and every
// older row shifts one cell along, keeping the colour it was given.
//
// Between steps the glyph animates through WATERFALL_GLYPHS — by default a
// single dot sliding ⠁ ⠂ ⠄ ⡀ down the cell — so the dot reaches the cell's
// far edge just as its colour moves into the next cell. That gives
// continuous, one-dot-per-frame motion (like the intro splash's particles)
// instead of a whole-cell jump every few frames, while each cell still has
// exactly one colour. A single-glyph WATERFALL_GLYPHS gives a static grid
// whose colours alone scroll.
//
//   - vertical: lanes are cell columns, low frequency left → high right;
//     rows fall from the top edge to the bottom.
//   - butterfly: lanes are cell rows, low frequency bottom → high top; rows
//     stream outward from the centre seam, one channel per side.

// WaterfallField is one independent waterfall: `lanes` lanes, `length`
// cells long. rows[0] is the newest (at the spawn edge); each row holds one
// colour per lane.
type WaterfallField struct {
	lanes, length int
	rows          [][]string
	frame         int // frames since the last step
}

// NewWaterfallField builds an empty field of the given size.
func NewWaterfallField(lanes, length int) *WaterfallField {
	f := &WaterfallField{}
	f.Resize(lanes, length)
	return f
}

// Resize clears the field for new dimensions; a no-op if unchanged.
func (f *WaterfallField) Resize(lanes, length int) {
	if lanes < 1 {
		lanes = 1
	}
	if length < 1 {
		length = 1
	}
	if lanes == f.lanes && length == f.length {
		return
	}
	f.lanes, f.length = lanes, length
	f.rows = nil
}

// Update advances one frame. The first call fills the whole field with
// blank rows; after that, every WATERFALL_FRAMES_PER_ROW frames a freshly
// sampled row enters at the spawn edge and the oldest drops off the far one.
// levelAt and hueFor are as for RainField.Update.
func (f *WaterfallField) Update(levelAt func(lane int) float64, hueFor func(level float64) string) {
	if f.rows == nil {
		blank := make([]string, f.lanes)
		f.rows = make([][]string, f.length)
		for i := range f.rows {
			f.rows[i] = blank
		}
	}

	f.frame++
	every := WATERFALL_FRAMES_PER_ROW
	if every < 1 {
		every = 1
	}
	if f.frame < every {
		return
	}
	f.frame = 0

	row := make([]string, f.lanes)
	for l := range row {
		row[l] = hueFor(levelAt(l))
	}
	copy(f.rows[1:], f.rows[:len(f.rows)-1])
	f.rows[0] = row
}

// phaseGlyph picks the glyph for how far the field is through its current
// step: glyphs[0] right after a step, the last one just before the next.
func (f *WaterfallField) phaseGlyph(glyphs []rune) rune {
	every := WATERFALL_FRAMES_PER_ROW
	if every < 1 {
		every = 1
	}
	i := f.frame * len(glyphs) / every
	if i >= len(glyphs) {
		i = len(glyphs) - 1
	}
	return glyphs[i]
}

// cell returns the glyph cell at (pos, lane) in field space: pos 0 is the
// spawn edge. Silent lanes, and everything before the first Update, are
// blank.
func (f *WaterfallField) cell(pos, lane int, glyph rune) scell {
	if pos < 0 || pos >= len(f.rows) || lane < 0 || lane >= len(f.rows[pos]) || f.rows[pos][lane] == "" {
		return scell{}
	}
	return scell{r: glyph, fg: f.rows[pos][lane]}
}

// UpdateWaterfall advances the waterfall field(s) one frame, with the same
// channel handling as UpdateRain. Call it exactly once per frame — the
// scroll is frame-counted.
func (r *Renderer) UpdateWaterfall(layout string, bandsL, bandsR []float64, gain float64) {
	hueFor := func(v float64) string { return dotHue(r.scheme, v) }

	w, h, mirrored := r.constellationDims(layout)
	if mirrored {
		// The right half runs half a step out of phase with the left. Two
		// mirrored 2-dot-wide patterns in step put the innermost dots
		// alternately touching and three dots apart across the seam — a
		// flicker in the middle. Half a step (one dot) apart, both halves
		// sit on one evenly spaced dot grid every frame while each still
		// moves outward. Re-derived from the left half every frame rather
		// than set once at creation: the render path can create the fields
		// first, and an offset set only in here would then never apply.
		fl, fr := ensureWaterfall(&r.fallL, h, w), ensureWaterfall(&r.fallR, h, w)
		if every := WATERFALL_FRAMES_PER_ROW; every > 1 {
			fr.frame = (fl.frame + every/2) % every
		}
		fl.Update(rainLevelAt(bandsL, h, gain, r.ampValue, true), hueFor)
		fr.Update(rainLevelAt(bandsR, h, gain, r.ampValue, true), hueFor)
		return
	}
	ensureWaterfall(&r.fallMono, w, h).Update(rainLevelAt(bandsL, w, gain, r.ampValue, false), hueFor)
}

func ensureWaterfall(f **WaterfallField, lanes, length int) *WaterfallField {
	if *f == nil {
		*f = NewWaterfallField(lanes, length)
	} else {
		(*f).Resize(lanes, length)
	}
	return *f
}

// RenderWaterfall draws the vertical-layout waterfall: lane = cell column,
// pos 0 at the top.
func (r *Renderer) RenderWaterfall(gain float64, schemeName string, peakFall bool) string {
	w, h, _ := r.constellationDims("vertical")
	if h < 10 {
		return "Terminal too small - need at least 13 lines"
	}
	f := ensureWaterfall(&r.fallMono, w, h)
	glyph := f.phaseGlyph(glyphRunes(WATERFALL_GLYPHS))
	cells := make([][]scell, h)
	for y := range cells {
		cells[y] = make([]scell, w)
		for x := range cells[y] {
			cells[y][x] = f.cell(y, x, glyph)
		}
	}
	return r.frameBody(renderCellGrid(cells, w, h), gain, schemeName, peakFall)
}

// RenderWaterfallButterfly draws the two halves: lane = cell row, pos 0 at
// the centre seam, streaming outward.
func (r *Renderer) RenderWaterfallButterfly(gain float64, schemeName string, peakFall bool) string {
	halfW, h, _ := r.constellationDims("butterfly")
	if h < 10 {
		return "Terminal too small - need at least 13 lines"
	}
	fl := ensureWaterfall(&r.fallL, h, halfW)
	fr := ensureWaterfall(&r.fallR, h, halfW)
	// The butterfly glyphs step rightward (the right half's outward
	// direction); the left half plays them mirrored.
	gr := glyphRunes(WATERFALL_GLYPHS_BUTTERFLY)
	gl := make([]rune, len(gr))
	for i, g := range gr {
		gl[i] = mirrorBraille(g)
	}
	glyphL, glyphR := fl.phaseGlyph(gl), fr.phaseGlyph(gr)
	lc := make([][]scell, h)
	rc := make([][]scell, h)
	for y := 0; y < h; y++ {
		lc[y] = make([]scell, halfW)
		rc[y] = make([]scell, halfW)
		for x := 0; x < halfW; x++ {
			lc[y][x] = fl.cell(halfW-1-x, y, glyphL)
			rc[y][x] = fr.cell(x, y, glyphR)
		}
	}
	return r.frameButterfly(lc, rc, halfW, h, gain, schemeName, peakFall)
}

func glyphRunes(s string) []rune {
	if rs := []rune(s); len(rs) > 0 {
		return rs
	}
	return []rune{'⣿'}
}

// mirrorBraille flips a braille glyph left↔right; anything else is returned
// unchanged.
func mirrorBraille(g rune) rune {
	if g < 0x2800 || g > 0x28FF {
		return g
	}
	m := byte(g - 0x2800)
	var out byte
	for row := 0; row < 4; row++ {
		if m&brailleBits[row][0] != 0 {
			out |= brailleBits[row][1]
		}
		if m&brailleBits[row][1] != 0 {
			out |= brailleBits[row][0]
		}
	}
	return rune(0x2800 + int(out))
}

// frameBody wraps a full-screen body in the header/footer when chrome is on.
func (r *Renderer) frameBody(body string, gain float64, schemeName string, peakFall bool) string {
	if !r.chrome {
		return body
	}
	return r.buildHeader(gain, schemeName, peakFall) + "\n" + body + "\n" + r.buildFooter()
}

// frameButterfly joins two halfW-wide cell grids around the centre gap and
// wraps them like frameBody.
func (r *Renderer) frameButterfly(lc, rc [][]scell, halfW, h int, gain float64, schemeName string, peakFall bool) string {
	w := r.termWidth
	if w < 8 {
		w = 8
	}
	gap := w - halfW*2
	if gap < 0 {
		gap = 0
	}
	center := strings.Repeat(" ", gap)

	lines := make([]string, h)
	for y := 0; y < h; y++ {
		lines[y] = renderCellRow(lc[y]) + center + renderCellRow(rc[y])
	}
	return r.frameBody(strings.Join(lines, "\n"), gain, schemeName, peakFall)
}
