package viz

import (
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// Constellation is the "constellation" bar style: not bars at all, but a
// field of drifting points connected to their neighbours by faint lines —
// ported from the effect of the same name in oiwn/tarts
// (https://github.com/oiwn/tarts, MIT) and made audio-reactive:
//
//   - overall loudness scales how fast the points drift
//   - bass widens the connection radius (a loud kick pulls in more lines)
//   - treble speeds up the twinkle
//   - a sudden loudness jump (an onset/transient) fires a brief brightness
//     flash across the whole field
//
// Colour is two layers, both from the active ColorScheme's GetColorForHeight
// ramp (never tarts' own fixed blue/purple palette):
//
//  1. A dim, whole-field ambient hue from the loudest current band overall
//     — mostly there so the field isn't pure black between hits.
//  2. Expanding colour rings (waveEmitter/cwave): each time a frequency
//     band peaks, a ring spawns AT THAT BAND'S OWN SPATIAL POSITION in the
//     field, coloured like that band's own bar would be, and travels
//     outward from there, fading with distance and age. So the colour
//     genuinely starts "where the frequency is" and propagates, rather
//     than the whole field flashing one shared hue at once.
//
// Spatial mapping of "where a frequency is" (see verticalWaveOrigin /
// butterflyWaveOrigin) matches the plain bar styles' own frequency layout,
// so a ring starts where that band's bar would be:
//
//   - vertical: bands sit left→right across the field's width (column i is
//     band i, same as the plain vertical bars), y fixed at vertical centre.
//   - butterfly: bands sit down the shared field's inner edge (the seam
//     against the centre gap), low→high bottom→top — the same convention
//     the plain butterfly bars use. A ring's origin sits ON that edge, so
//     roughly half its circle falls outside the field and is invisible,
//     leaving a ~180° fan into the pane. One shared field (so the two
//     sides' star positions mirror exactly) but two independent
//     waveEmitters (bandsL / bandsR), so the left ring set only reflects
//     the left channel and fans into the left pane, and likewise right —
//     tinted separately, then the right side's already-coloured cells are
//     mirrored for display (mirrorCells), not the raw field.

// cstar is one drifting point.
type cstar struct {
	x, y         float64
	vx, vy       float64
	twinklePhase float64
	twinkleFreq  float64 // per-star multiplier on the shared twinkle speed
	glyphIdx     int
}

// cmask is one cell of a rendered field: a glyph plus a 0..~1.15 brightness
// level (headroom above 1 lets a flash push a cell into tintConstellation's
// hot-colour blend). A zero rune means the cell is empty.
type cmask struct {
	r     rune
	level float64
}

// ConstellationField is one independent star simulation over a w×h grid of
// terminal cells (vertical: the whole screen; butterfly: one shared
// half-width field, rendered mirrored — see mirrorMask).
type ConstellationField struct {
	w, h        int
	stars       []cstar
	connectDist float64
	prevEnergy  float64
	flash       float64
	rng         *rand.Rand
}

// NewConstellationField builds a field sized to w×h.
func NewConstellationField(w, h int) *ConstellationField {
	f := &ConstellationField{rng: rand.New(rand.NewSource(2))}
	f.Resize(w, h)
	return f
}

// Resize re-seeds the field for new dimensions. A no-op if the size hasn't
// actually changed, so a caller can call it unconditionally every frame
// without resetting the stars.
func (f *ConstellationField) Resize(w, h int) {
	if w < 4 {
		w = 4
	}
	if h < 4 {
		h = 4
	}
	if w == f.w && h == f.h && f.stars != nil {
		return
	}
	f.w, f.h = w, h

	n := int(float64(w*h) * CONSTELLATION_STAR_DENSITY)
	if n < CONSTELLATION_MIN_STARS {
		n = CONSTELLATION_MIN_STARS
	}
	if n > CONSTELLATION_MAX_STARS {
		n = CONSTELLATION_MAX_STARS
	}

	f.stars = make([]cstar, n)
	for i := range f.stars {
		f.stars[i] = f.randomStar()
	}
	f.connectDist = math.Hypot(float64(w), float64(h)) * CONSTELLATION_CONNECT_RADIUS
	f.prevEnergy = 0
	f.flash = 0
}

func (f *ConstellationField) randomStar() cstar {
	speed := CONSTELLATION_MIN_SPEED + f.rng.Float64()*(CONSTELLATION_MAX_SPEED-CONSTELLATION_MIN_SPEED)
	angle := f.rng.Float64() * 2 * math.Pi
	return cstar{
		x:            f.rng.Float64() * float64(f.w),
		y:            f.rng.Float64() * float64(f.h),
		vx:           math.Cos(angle) * speed,
		vy:           math.Sin(angle) * speed,
		twinklePhase: f.rng.Float64() * 2 * math.Pi,
		twinkleFreq:  0.4 + f.rng.Float64()*0.8,
		glyphIdx:     f.rng.Intn(len(CONSTELLATION_GLYPHS)),
	}
}

// Update advances the field by dtSec. bass/treble/overall are 0..1 energy
// readings for whatever channel(s) drive this field's motion (see
// Renderer.UpdateConstellation for how vertical/butterfly combine L/R).
func (f *ConstellationField) Update(dtSec, bass, treble, overall float64) {
	speedMul := 1 + overall*CONSTELLATION_ENERGY_SPEED_BOOST
	twinkleSpeed := CONSTELLATION_TWINKLE_BASE + treble*CONSTELLATION_TWINKLE_TREBLE_BOOST

	fw, fh := float64(f.w), float64(f.h)
	for i := range f.stars {
		s := &f.stars[i]
		s.x += s.vx * speedMul * dtSec
		s.y += s.vy * speedMul * dtSec
		s.twinklePhase += s.twinkleFreq * twinkleSpeed * dtSec

		if s.x < 0 {
			s.x = -s.x
			s.vx = -s.vx
		} else if s.x >= fw {
			s.x = 2*fw - s.x
			s.vx = -s.vx
		}
		if s.y < 0 {
			s.y = -s.y
			s.vy = -s.vy
		} else if s.y >= fh {
			s.y = 2*fh - s.y
			s.vy = -s.vy
		}
	}

	f.connectDist = math.Hypot(fw, fh) * CONSTELLATION_CONNECT_RADIUS * (1 + bass*CONSTELLATION_BASS_RADIUS_BOOST)

	if delta := overall - f.prevEnergy; delta > CONSTELLATION_ONSET_THRESHOLD {
		f.flash = 1
	} else {
		f.flash *= CONSTELLATION_FLASH_DECAY
	}
	f.prevEnergy = overall
}

// Render draws the current frame into a w×h mask grid (glyph + brightness,
// no colour — see tintConstellation).
func (f *ConstellationField) Render() [][]cmask {
	grid := make([][]cmask, f.h)
	for y := range grid {
		grid[y] = make([]cmask, f.w)
	}
	f.drawConnections(grid)
	f.drawStars(grid)
	return grid
}

// drawConnections links each star to its nearest neighbours within
// connectDist, up to CONSTELLATION_MAX_CONNECTIONS per star — same
// nearest-first greedy matching as tarts, so a busy field doesn't turn into
// an unreadable tangle.
func (f *ConstellationField) drawConnections(grid [][]cmask) {
	n := len(f.stars)
	connCount := make([]int, n)

	type neighbor struct {
		j int
		d float64
	}

	for i := 0; i < n; i++ {
		var neighbors []neighbor
		for j := i + 1; j < n; j++ {
			dx := f.stars[j].x - f.stars[i].x
			dy := f.stars[j].y - f.stars[i].y
			d := math.Hypot(dx, dy)
			if d <= f.connectDist {
				neighbors = append(neighbors, neighbor{j, d})
			}
		}
		sort.Slice(neighbors, func(a, b int) bool { return neighbors[a].d < neighbors[b].d })

		for _, nb := range neighbors {
			if connCount[i] >= CONSTELLATION_MAX_CONNECTIONS || connCount[nb.j] >= CONSTELLATION_MAX_CONNECTIONS {
				continue
			}
			connCount[i]++
			connCount[nb.j]++

			level := (1 - nb.d/f.connectDist) * 0.6
			level += f.flash * 0.3
			f.drawDottedLine(grid, f.stars[i].x, f.stars[i].y, f.stars[nb.j].x, f.stars[nb.j].y, level)
		}
	}
}

// drawDottedLine plots '·' along the segment, skipping the endpoints (the
// stars themselves draw over them) and never dimming a cell a brighter line
// already claimed.
func (f *ConstellationField) drawDottedLine(grid [][]cmask, x0, y0, x1, y1, level float64) {
	dx, dy := x1-x0, y1-y0
	steps := int(math.Max(math.Abs(dx), math.Abs(dy)))
	if steps < 2 {
		return
	}
	for i := 1; i < steps; i++ {
		t := float64(i) / float64(steps)
		x := int(x0 + dx*t + 0.5)
		y := int(y0 + dy*t + 0.5)
		if x < 0 || x >= f.w || y < 0 || y >= f.h {
			continue
		}
		if grid[y][x].level < level {
			grid[y][x] = cmask{r: '·', level: level}
		}
	}
}

// drawStars plots each star, brightness breathing via its twinkle phase and
// boosted by any live flash. Twinkle alone is capped below the hot-colour
// blend's 0.9 threshold (see cmask) so a star never goes hot on its own —
// that's reserved for an actual audio-driven boost (flash or a colour wave),
// so "hot" stays tied to where the sound is loud, not to random twinkle
// phase scattered evenly across the whole field.
func (f *ConstellationField) drawStars(grid [][]cmask) {
	for i := range f.stars {
		s := &f.stars[i]
		level := 0.55 + 0.30*math.Sin(s.twinklePhase) + f.flash*0.5
		if level > 1.15 {
			level = 1.15
		}
		x, y := int(s.x+0.5), int(s.y+0.5)
		if x < 0 || x >= f.w || y < 0 || y >= f.h {
			continue
		}
		grid[y][x] = cmask{r: CONSTELLATION_GLYPHS[s.glyphIdx], level: level}
	}
}

// cwave is one expanding colour ring, spawned when a frequency band peaks.
// It's coloured like that band's own bar would be (baked at spawn time, so
// a scheme change mid-flight doesn't retroactively recolour it) and expands
// outward from that band's spatial origin (see verticalWaveOrigin /
// butterflyWaveOrigin), fading with both distance from the ring and age
// until waveEmitter culls it.
type cwave struct {
	ox, oy   float64
	hue      string
	born     float64 // the owning waveEmitter's simTime at spawn
	strength float64 // 0..1, the triggering band's own gain-adjusted level
}

// waveEmitter owns one field-side's rings: spawns a new one whenever a band
// crosses CONSTELLATION_WAVE_SPAWN_THRESHOLD (with a per-band cooldown so a
// sustained loud band doesn't spawn a ring every single frame), ages them,
// and culls whatever's fully faded.
type waveEmitter struct {
	simTime   float64
	waves     []cwave
	lastSpawn []float64 // per band index, simTime of its last spawn
}

// update advances the emitter by dtSec and spawns rings for any band in
// `bands` that just crossed the spawn threshold. `curve` shapes a band's
// gain-adjusted level the same way a bar's height is shaped (see
// Renderer.ampValue) before it picks the ring's hue off the scheme ramp.
// `origin` maps a band index to its spatial spawn point.
func (e *waveEmitter) update(dtSec float64, bands []float64, gain float64, scheme ColorScheme, curve func(float64) float64, origin func(i, n int) (float64, float64)) {
	e.simTime += dtSec
	n := len(bands)
	if len(e.lastSpawn) != n {
		e.lastSpawn = make([]float64, n)
		for i := range e.lastSpawn {
			e.lastSpawn[i] = -1000
		}
	}

	for i, raw := range bands {
		v := raw * gain
		if v > 1 {
			v = 1
		}
		if v < CONSTELLATION_WAVE_SPAWN_THRESHOLD {
			continue
		}
		if e.simTime-e.lastSpawn[i] < CONSTELLATION_WAVE_SPAWN_COOLDOWN_S {
			continue
		}
		e.lastSpawn[i] = e.simTime
		ox, oy := origin(i, n)
		hue := string(GetColorForHeight(scheme, curve(v)))
		e.waves = append(e.waves, cwave{ox: ox, oy: oy, hue: hue, born: e.simTime, strength: v})
	}

	if len(e.waves) == 0 {
		return
	}
	alive := e.waves[:0]
	for _, wv := range e.waves {
		if e.simTime-wv.born < CONSTELLATION_WAVE_FADE_S {
			alive = append(alive, wv)
		}
	}
	e.waves = alive
	if len(e.waves) > CONSTELLATION_WAVE_MAX_ACTIVE {
		e.waves = e.waves[len(e.waves)-CONSTELLATION_WAVE_MAX_ACTIVE:]
	}
}

// verticalWaveOrigin places band i's ring origin at x = i's position across
// the field's width, low band → left edge, high band → right edge — the same
// left-to-right convention the plain vertical bars use (column i sits at x =
// i). y is fixed at the field's vertical centre, since the bars have no
// per-band y position for a ring to match.
func verticalWaveOrigin(w, h int) func(i, n int) (float64, float64) {
	cy := float64(h) / 2
	return func(i, n int) (float64, float64) {
		if n <= 1 {
			return float64(w-1) / 2, cy
		}
		return float64(i) / float64(n-1) * float64(w-1), cy
	}
}

// butterflyWaveOrigin places band i's ring origin on the field's inner edge
// (x = w-1, the seam against the centre gap — shared by both the left and
// the mirrored right rendering, see mirrorCells), low band → bottom, high
// band → top, matching the plain butterfly bars' convention (band 0 sits at
// the bottom row, see buildButterfly). A ring spawned on that edge has
// roughly half its circle fall outside the field (x > w-1 doesn't exist), so
// what's visible is a ~180° fan into the pane.
func butterflyWaveOrigin(w, h int) func(i, n int) (float64, float64) {
	ox := float64(w - 1)
	return func(i, n int) (float64, float64) {
		if n <= 1 {
			return ox, float64(h) / 2
		}
		return ox, float64(h-1) * (1 - float64(i)/float64(n-1))
	}
}

// abColor is one band's own current colour: hue off the scheme ramp at that
// band's gain-adjusted, curved level, plus that same 0..1 level for scaling
// brightness. Same "loud band → hot hue" rule waveEmitter uses for a ring's
// colour, just computed for every band every frame instead of only at spawn.
type abColor struct {
	hue   string
	level float64
}

// computeAmbientBands is bandColorAt for every band at once: each band's own
// hue+level, in frequency order, unmodified by CONSTELLATION_AMBIENT_MIX (the
// caller dims brightness, not hue — see tintConstellation's
// `ambLevel*CONSTELLATION_AMBIENT_MIX` —
// so a genuinely loud band still reads as its true hot colour, not washed
// toward green).
func computeAmbientBands(bands []float64, gain float64, scheme ColorScheme, curve func(float64) float64) []abColor {
	out := make([]abColor, len(bands))
	for i, raw := range bands {
		v := raw * gain
		if v > 1 {
			v = 1
		}
		cv := curve(v)
		out[i] = abColor{hue: string(GetColorForHeight(scheme, cv)), level: cv}
	}
	return out
}

// verticalAmbientAt turns per-band colours into a persistent left-to-right
// backdrop, inverting verticalWaveOrigin's column-per-band mapping so column
// x always shows that same column's own current colour — bass (low band,
// left) glows hot the instant it's loud, without waiting for a ring to spawn
// and pass through, and likewise treble on the right.
func verticalAmbientAt(bandColors []abColor, w int) func(x, y int) (string, float64) {
	n := len(bandColors)
	return func(x, y int) (string, float64) {
		if n == 0 {
			return "#000000", 0
		}
		idx := 0
		if w > 1 && n > 1 {
			idx = int(float64(x)/float64(w-1)*float64(n-1) + 0.5)
		}
		if idx < 0 {
			idx = 0
		} else if idx >= n {
			idx = n - 1
		}
		return bandColors[idx].hue, bandColors[idx].level
	}
}

// butterflyAmbientAt mirrors butterflyWaveOrigin's bottom-to-top convention:
// row y always shows that row's own band's current colour, so a sustained
// loud bass glows hot at the bottom (and highs at the top) continuously,
// same as verticalAmbientAt does left-to-right.
func butterflyAmbientAt(bandColors []abColor, h int) func(x, y int) (string, float64) {
	n := len(bandColors)
	return func(x, y int) (string, float64) {
		if n == 0 {
			return "#000000", 0
		}
		idx := 0
		if h > 1 && n > 1 {
			frac := 1 - float64(y)/float64(h-1)
			idx = int(frac*float64(n-1) + 0.5)
		}
		if idx < 0 {
			idx = 0
		} else if idx >= n {
			idx = n - 1
		}
		return bandColors[idx].hue, bandColors[idx].level
	}
}

// tintConstellation bakes a brightness mask into coloured cells. Colour is
// two layers: `ambientAt(x, y)` gives a per-position baseline hue + level off
// the scheme ramp, driven by that spot's OWN band's current loudness (see
// verticalAmbientAt / butterflyAmbientAt) — so bass reliably reads hot on the
// left (or bottom, in butterfly) and highs hot on the right (or top) all the
// time, not just for the brief life of a colour wave. Any active `waves`
// that reach a cell override that baseline with the triggering band's own
// hue and a brightness boost, strongest right at the ring's leading edge and
// fading as it's aged past CONSTELLATION_WAVE_FADE_S. Within whichever hue
// wins, a cell's own brightness (twinkle, flash, ambient level, wave boost)
// blends up from black and, past 90%, on toward the scheme's own top-of-ramp
// colour (GetColorForHeight at 1.0) rather than white — brightest stars stay
// on-palette instead of bleaching out.
func tintConstellation(mask [][]cmask, scheme ColorScheme, ambientAt func(x, y int) (string, float64), waves []cwave, simTime float64) [][]scell {
	half := CONSTELLATION_WAVE_WIDTH / 2
	hotColor := string(GetColorForHeight(scheme, 1.0))

	out := make([][]scell, len(mask))
	for y, row := range mask {
		out[y] = make([]scell, len(row))
		for x, m := range row {
			if m.r == 0 || m.level <= 0.01 {
				continue
			}

			ambHue, ambLevel := ambientAt(x, y)
			hue := ambHue
			var boost float64
			for _, wv := range waves {
				age := simTime - wv.born
				if age < 0 || age >= CONSTELLATION_WAVE_FADE_S {
					continue
				}
				dx := float64(x) - wv.ox
				dy := (float64(y) - wv.oy) * CONSTELLATION_CELL_ASPECT
				delta := math.Abs(math.Hypot(dx, dy) - age*CONSTELLATION_WAVE_SPEED)
				if delta >= half {
					continue
				}
				intensity := wv.strength * (1 - delta/half) * (1 - age/CONSTELLATION_WAVE_FADE_S)
				if intensity > boost {
					boost = intensity
					hue = wv.hue
				}
			}

			b := m.level + ambLevel*CONSTELLATION_AMBIENT_MIX + boost*0.6
			if b > 1 {
				b = 1
			}
			col := interpolateColor("#000000", hue, b)
			if b > 0.9 {
				col = interpolateColor(hue, hotColor, (b-0.9)*10)
			}
			out[y][x] = scell{r: m.r, fg: string(col)}
		}
	}
	return out
}

// mirrorCells flips an already-coloured cell grid horizontally — used to
// render the butterfly right half as a mirror image of the left's tinted
// output (see the package doc comment for why tinting happens before the
// mirror, not after).
func mirrorCells(cells [][]scell) [][]scell {
	out := make([][]scell, len(cells))
	for y, row := range cells {
		w := len(row)
		mirrored := make([]scell, w)
		for x, c := range row {
			mirrored[w-1-x] = c
		}
		out[y] = mirrored
	}
	return out
}

// bandEnergy splits smoothed band magnitudes into bass/mid/treble thirds
// (bands are log-spaced low→high, same convention as the rest of the
// pipeline), applying gain the same way bars do (clamped to 1 per band).
// Each third reports its LOUDEST band, not the average: a spectrum is
// mostly quiet bins with a few loud ones, so averaging across a whole third
// drowns the signal down near 0 and the effect never reads as reactive (the
// hue ramp needs to reach ~0.9 for magenta/red and an average essentially
// never does). Peak-per-band mirrors how a single tall bar looks, and how a
// listener perceives "loud now" — same reasoning as a VU meter reading the
// peak, not the mean. `overall` is the loudest of the three.
func bandEnergy(bands []float64, gain float64) (bass, mid, treble, overall float64) {
	n := len(bands)
	if n == 0 {
		return
	}
	third := n / 3
	if third < 1 {
		third = 1
	}
	peak := func(lo, hi int) float64 {
		var m float64
		for i := lo; i < hi && i < n; i++ {
			v := bands[i] * gain
			if v > 1 {
				v = 1
			}
			if v > m {
				m = v
			}
		}
		return m
	}
	bass = peak(0, third)
	mid = peak(third, 2*third)
	treble = peak(2*third, n)
	overall = math.Max(bass, math.Max(mid, treble))
	return
}

// renderCellGrid joins a w×h scell grid into a frame (no trailing newline).
func renderCellGrid(cells [][]scell, w, h int) string {
	lines := make([]string, h)
	for y := 0; y < h && y < len(cells); y++ {
		lines[y] = renderCellRow(cells[y])
	}
	return strings.Join(lines, "\n")
}

// renderCellRow renders one row of a scell grid. Runs of same-coloured cells
// share one colour escape (see fgEscape) instead of a lipgloss Render per
// cell — the dot styles fill nearly every cell, and per-cell styling was
// most of their frame time and output size.
func renderCellRow(row []scell) string {
	var b strings.Builder
	open := "" // fg of the run currently open, "" = none
	var suffix string
	for _, c := range row {
		if c.r == 0 || c.r == ' ' {
			if open != "" {
				b.WriteString(suffix)
				open = ""
			}
			b.WriteByte(' ')
			continue
		}
		if c.fg != open {
			if open != "" {
				b.WriteString(suffix)
			}
			var prefix string
			prefix, suffix = fgEscape(c.fg)
			b.WriteString(prefix)
			open = c.fg
		}
		b.WriteRune(c.r)
	}
	if open != "" {
		b.WriteString(suffix)
	}
	return b.String()
}

// fgEscapes caches each colour's start/end escape sequences. They come from
// lipgloss itself (split around a sentinel) so the terminal colour-profile
// handling — truecolor, 256, 16, none — stays lipgloss's.
var (
	fgEscapeMu sync.Mutex
	fgEscapes  = map[string][2]string{}
)

func fgEscape(fg string) (prefix, suffix string) {
	fgEscapeMu.Lock()
	defer fgEscapeMu.Unlock()
	if e, ok := fgEscapes[fg]; ok {
		return e[0], e[1]
	}
	if len(fgEscapes) > 4096 { // constellation blends many shades; keep it bounded
		fgEscapes = map[string][2]string{}
	}
	out := lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Render("\x00")
	if i := strings.IndexByte(out, 0); i >= 0 {
		prefix, suffix = out[:i], out[i+1:]
	}
	fgEscapes[fg] = [2]string{prefix, suffix}
	return prefix, suffix
}
