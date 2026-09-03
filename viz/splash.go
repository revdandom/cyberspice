package viz

import (
	"image"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

type splashPhase int

const (
	splashHold splashPhase = iota // the still is shown
	splashMove                    // particles pour into the pool
	splashFade                    // the pool dims to black
	splashDone
)

// braille bit layout for a 2×4 cell (dot numbers 1..8):
//
//	1 4   0x01 0x08
//	2 5   0x02 0x10
//	3 6   0x04 0x20
//	7 8   0x40 0x80
var brailleBits = [4][2]byte{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// scell is one rendered braille cell of the held scene.
type scell struct {
	r  rune
	fg string // "#rrggbb"
}

type sparticle struct {
	x, y   float64 // dot-space position (0..uw, 0..uh)
	vx, vy float64
	fg     string
}

// Splash is the intro: a braille halftone of the HACKERBOT still holds
// briefly, then dissolves into dots that pour into a pool — along the
// bottom in the vertical layout, down the centre axis in butterfly — and
// the pool fades out before the visualiser takes over.
type Splash struct {
	w, h   int
	layout string
	scheme ColorScheme

	hold   [][]scell // h×w, the scene halftone (zero cell = blank)
	parts  []sparticle
	uw, uh int // dot dims: w*2, h*4

	poolDepth int // vertical: dot rows of puddle along the bottom
	poolHalf  int // butterfly: half-width of the centre column, in dots
	rng       *rand.Rand

	phase splashPhase
	start time.Time // wall-clock start of the current phase run
	frame int       // frames elapsed in the move / fade phase
	fade  float64
}

// NewSplash builds the splash for the given terminal size, layout and scheme.
func NewSplash(w, h int, layout string, scheme ColorScheme) *Splash {
	s := &Splash{scheme: scheme, start: time.Now(), rng: rand.New(rand.NewSource(1))}
	s.rebuild(w, h, layout)
	return s
}

// Resize re-lays the splash for new dimensions and restarts the hold.
func (s *Splash) Resize(w, h int, layout string) {
	if s.phase == splashDone {
		return
	}
	s.phase = splashHold
	s.start = time.Now()
	s.frame = 0
	s.fade = 0
	s.parts = s.parts[:0]
	s.rebuild(w, h, layout)
}

func (s *Splash) rebuild(w, h int, layout string) {
	if w < 20 {
		w = 20
	}
	if h < 10 {
		h = 10
	}
	s.w, s.h, s.layout = w, h, layout
	s.uw, s.uh = w*2, h*4

	if s.poolDepth = s.uh / 8; s.poolDepth < 3 {
		s.poolDepth = 3
	}
	if s.poolHalf = s.uw / 28; s.poolHalf < 3 {
		s.poolHalf = 3
	}
	s.buildHold()
}

// buildHold renders the still into an h×w grid of braille cells, centred.
func (s *Splash) buildHold() {
	s.hold = make([][]scell, s.h)
	for i := range s.hold {
		s.hold[i] = make([]scell, s.w)
	}

	gray, rgba := sceneImages()
	if gray == nil || rgba == nil {
		return // decode failed — a blank hold, Update finishes it quickly
	}

	cols := s.w
	if cols > 260 {
		cols = 260
	}
	rows := aspectRows(gray, cols)
	if rows > s.h && rows > 0 {
		cols = cols * s.h / rows
		rows = aspectRows(gray, cols)
	}
	cols = clampi(cols, 1, s.w)
	rows = clampi(rows, 1, s.h)

	dw, dh := cols*2, rows*4
	lum := boxSampleGrayF(gray, dw, dh)

	bright := make([]float64, cols*rows)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			sum := 0.0
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					sum += lum[(r*4+dy)*dw+c*2+dx]
				}
			}
			bright[r*cols+c] = sum / 8
		}
	}

	atkinson(lum, dw, dh)

	iw, ih := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	ox := (s.w - cols) / 2
	oy := (s.h - rows) / 2
	if oy < 0 {
		oy = 0
	}
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			var m byte
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					if lum[(r*4+dy)*dw+c*2+dx] >= 0.5 {
						m |= brailleBits[dy][dx]
					}
				}
			}
			if m == 0 {
				continue
			}
			s.hold[oy+r][ox+c] = scell{
				r:  rune(0x2800 + int(m)),
				fg: sceneTint(rgba, iw, ih, r, c, rows, cols, bright[r*cols+c]),
			}
		}
	}
}

// sceneTint averages the source region behind cell (r,c) and returns it with
// the chroma pushed up and the lightness pinned to the cell's brightness, so
// warm skin / cyan-orange chrome / cool background all survive at cell size.
func sceneTint(rgba *image.RGBA, iw, ih, r, c, rows, cols int, bright float64) string {
	x0, x1 := c*iw/cols, (c+1)*iw/cols
	y0, y1 := r*ih/rows, (r+1)*ih/rows
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	var rs, gs, bs, n float64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			o := rgba.PixOffset(x, y)
			rs += float64(rgba.Pix[o]) / 255
			gs += float64(rgba.Pix[o+1]) / 255
			bs += float64(rgba.Pix[o+2]) / 255
			n++
		}
	}
	col := colorful.Color{R: clampf(rs/n, 0, 1), G: clampf(gs/n, 0, 1), B: clampf(bs/n, 0, 1)}
	hh, ss, ll := col.Hsl()
	return colorful.Hsl(hh, clampf(ss*1.7, 0, 1), clampf(ll*0.55+0.35*bright, 0.05, 0.95)).Clamped().Hex()
}

func msFrames(ms int) int {
	if f := ms * TARGET_FPS / 1000; f > 1 {
		return f
	}
	return 1
}

// Update advances the animation one frame; call it once per render tick.
func (s *Splash) Update() {
	switch s.phase {
	case splashHold:
		if time.Since(s.start) >= SPLASH_HOLD_MS*time.Millisecond {
			s.phase, s.frame = splashMove, 0
			s.start = time.Now()
			s.seed()
			if len(s.parts) == 0 {
				s.phase = splashDone
			}
		}

	case splashMove:
		s.frame++
		pooled := 0
		for i := range s.parts {
			s.moveParticle(&s.parts[i])
			if s.inPool(&s.parts[i]) {
				pooled++
			}
		}
		settled := len(s.parts) > 0 && pooled >= int(float64(len(s.parts))*SPLASH_POOL_FRACTION)
		hardCap := time.Since(s.start) >= SPLASH_DECAY_MS*time.Millisecond
		if hardCap || (s.frame >= msFrames(SPLASH_MOVE_MIN_MS) && settled) {
			s.phase, s.frame = splashFade, 0
			s.start = time.Now()
		}

	case splashFade:
		s.frame++
		s.fade = clampf(float64(s.frame)/float64(msFrames(SPLASH_FADE_MS)), 0, 1)
		for i := range s.parts {
			p := &s.parts[i]
			p.x += p.vx
			p.y += p.vy
			p.vx *= 0.7
			p.vy *= 0.7
			s.clampPool(p)
		}
		if s.fade >= 1 {
			s.phase = splashDone
		}
	}
}

// seed turns every lit dot of the held scene into a particle.
func (s *Splash) seed() {
	s.parts = s.parts[:0]
	for r := range s.hold {
		for c := range s.hold[r] {
			cl := s.hold[r][c]
			if cl.r < 0x2800 || cl.r > 0x28FF {
				continue
			}
			m := byte(cl.r - 0x2800)
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					if m&brailleBits[dy][dx] == 0 {
						continue
					}
					s.parts = append(s.parts, sparticle{
						x:  float64(c*2 + dx),
						y:  float64(r*4 + dy),
						fg: cl.fg,
					})
				}
			}
		}
	}
}

func (s *Splash) inPool(p *sparticle) bool {
	if s.layout == "butterfly" {
		return math.Abs(p.x-float64(s.uw)/2) <= float64(s.poolHalf)
	}
	return p.y >= float64(s.uh-s.poolDepth)
}

func (s *Splash) moveParticle(p *sparticle) {
	if s.layout == "butterfly" {
		cx, half := float64(s.uw)/2, float64(s.poolHalf)
		if math.Abs(p.x-cx) <= half { // in the column — hold x, drift vertically
			p.vx *= 0.30
			p.vy = p.vy*0.80 + (s.rng.Float64()*2-1)*0.7
		} else {
			if p.x < cx {
				p.vx += SPLASH_GRAVITY * 1.6
			} else {
				p.vx -= SPLASH_GRAVITY * 1.6
			}
			p.vx *= 0.98
		}
		p.x += p.vx
		p.y += p.vy
	} else {
		top := float64(s.uh - s.poolDepth)
		if p.y >= top { // in the puddle — settle down, spread sideways like liquid
			p.vy *= 0.30
			p.vx = p.vx*0.80 + (s.rng.Float64()*2-1)*0.7
			p.x += p.vx
			p.y += p.vy + 0.15
		} else {
			p.vy += SPLASH_GRAVITY * 1.5
			p.y += p.vy
			p.x += p.vx
		}
	}
	s.clampPool(p)
}

func (s *Splash) clampPool(p *sparticle) {
	if p.x < 0 {
		p.x, p.vx = 0, -p.vx*0.3
	}
	if p.x > float64(s.uw-1) {
		p.x, p.vx = float64(s.uw-1), -p.vx*0.3
	}
	if p.y < 0 {
		p.y, p.vy = 0, -p.vy*0.3
	}
	if p.y > float64(s.uh-1) {
		p.y, p.vy = float64(s.uh-1), -p.vy*0.3
	}
	if s.layout == "butterfly" {
		cx, half := float64(s.uw)/2, float64(s.poolHalf)
		if p.x < cx-half && p.vx < 0 {
			p.x, p.vx = cx-half, 0
		}
		if p.x > cx+half && p.vx > 0 {
			p.x, p.vx = cx+half, 0
		}
	} else if top := float64(s.uh - s.poolDepth); p.y > top && p.vy < 0 {
		p.vy = 0
	}
}

// Done reports whether the splash has finished.
func (s *Splash) Done() bool { return s.phase == splashDone }

// fadeFactor is the brightness multiplier for the current frame.
func (s *Splash) fadeFactor() float64 {
	switch s.phase {
	case splashFade:
		return clampf(1-math.Pow(s.fade, 0.6), 0, 1)
	case splashDone:
		return 0
	default:
		return 1
	}
}

// Render draws the current frame (s.h lines, no trailing newline).
func (s *Splash) Render() string {
	cells := s.hold
	if s.phase != splashHold {
		cells = s.rasterParticles()
	}
	dim := s.fadeFactor()

	var b strings.Builder
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			var cl scell
			if y < len(cells) && x < len(cells[y]) {
				cl = cells[y][x]
			}
			if cl.r == 0 || cl.r == ' ' {
				b.WriteByte(' ')
				continue
			}
			col := lipgloss.Color(cl.fg)
			if dim < 0.999 {
				col = interpolateColor(cl.fg, "#000000", 1-dim)
			}
			b.WriteString(lipgloss.NewStyle().Foreground(col).Render(string(cl.r)))
		}
		if y < s.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// rasterParticles packs the live particles back into a braille cell grid.
func (s *Splash) rasterParticles() [][]scell {
	out := make([][]scell, s.h)
	for i := range out {
		out[i] = make([]scell, s.w)
	}
	if s.fadeFactor() <= 0.02 {
		return out
	}
	masks := make([]byte, s.w*s.h)
	fgs := make([]string, s.w*s.h)
	for _, p := range s.parts {
		ux := int(p.x + 0.5)
		uy := int(p.y + 0.5)
		if ux < 0 || ux >= s.uw || uy < 0 || uy >= s.uh {
			continue
		}
		ci := (uy/4)*s.w + ux/2
		masks[ci] |= brailleBits[uy%4][ux%2]
		if fgs[ci] == "" {
			fgs[ci] = p.fg
		}
	}
	for i, m := range masks {
		if m == 0 {
			continue
		}
		out[i/s.w][i%s.w] = scell{r: rune(0x2800 + int(m)), fg: fgs[i]}
	}
	return out
}
