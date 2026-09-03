package viz

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// splashArt is the HACKERMAN wordmark in a 5-row block font.
var splashArt = []string{
	"█   █  ███  ████ █   █ █████ ████  █   █  ███  █   █",
	"█   █ █   █ █     █  █  █     █   █ ██ ██ █   █ ██  █",
	"█████ █████ █     ███   ████  ████  █ █ █ █████ █ █ █",
	"█   █ █   █ █     █  █  █     █  █  █   █ █   █ █  ██",
	"█   █ █   █  ████ █   █ █████ █   █ █   █ █   █ █   █",
}

type splashPhase int

const (
	splashHold splashPhase = iota
	splashDecay
	splashDone
)

type sparticle struct {
	x     int
	y, v  float64
	glyph rune
	rest  bool
}

// Splash is the intro: the HACKERMAN wordmark holds briefly, then crumbles
// into dots that fall — to the bottom in the vertical layout, and inward to
// the centre row in butterfly — before the visualiser takes over.
type Splash struct {
	w, h   int
	layout string
	scheme ColorScheme
	parts  []sparticle
	phase  splashPhase
	start  time.Time

	restLow []int // vertical: next rest row per column, growing up from h-1
	restUp  []int // butterfly: growing up from mid-1
	restDn  []int // butterfly: growing down from mid+1
}

// NewSplash builds the splash for the given terminal size, layout and scheme.
func NewSplash(w, h int, layout string, scheme ColorScheme) *Splash {
	s := &Splash{scheme: scheme, start: time.Now()}
	s.rebuild(w, h, layout)
	return s
}

// Resize re-lays the splash for new dimensions and restarts the hold.
func (s *Splash) Resize(w, h int, layout string) {
	if s.phase == splashDone {
		return
	}
	s.start = time.Now()
	s.phase = splashHold
	s.rebuild(w, h, layout)
}

func (s *Splash) rebuild(w, h int, layout string) {
	if w < 10 {
		w = 10
	}
	if h < 6 {
		h = 6
	}
	s.w, s.h, s.layout = w, h, layout

	aw := 0
	for _, ln := range splashArt {
		if l := len([]rune(ln)); l > aw {
			aw = l
		}
	}
	ox := (w - aw) / 2
	oy := (h - len(splashArt)) / 2
	if oy < 0 {
		oy = 0
	}

	s.parts = s.parts[:0]
	for r, ln := range splashArt {
		for c, ch := range []rune(ln) {
			if ch == ' ' {
				continue
			}
			x, y := ox+c, oy+r
			if x < 0 || x >= w || y < 0 || y >= h {
				continue
			}
			s.parts = append(s.parts, sparticle{x: x, y: float64(y), glyph: '█'})
		}
	}

	s.restLow = make([]int, w)
	s.restUp = make([]int, w)
	s.restDn = make([]int, w)
	mid := h / 2
	for i := range s.restLow {
		s.restLow[i] = h - 1
		s.restUp[i] = mid - 1
		s.restDn[i] = mid + 1
	}
}

var splashDots = []rune{'.', '·', ':', '\'', '`', '*'}

// Update advances the animation one frame; call it once per render tick.
func (s *Splash) Update() {
	switch s.phase {
	case splashHold:
		if time.Since(s.start) >= SPLASH_HOLD_MS*time.Millisecond {
			s.phase = splashDecay
			s.start = time.Now()
			for i := range s.parts {
				s.parts[i].glyph = splashDots[(s.parts[i].x*7+i*3)%len(splashDots)]
			}
		}
	case splashDecay:
		if time.Since(s.start) >= SPLASH_DECAY_MS*time.Millisecond || s.stepFall() {
			s.phase = splashDone
		}
	}
}

// stepFall moves every unsettled particle one frame; returns true when all
// have come to rest.
func (s *Splash) stepFall() bool {
	allRest := true

	if s.layout == "butterfly" {
		mid := float64(s.h / 2)
		for i := range s.parts {
			p := &s.parts[i]
			if p.rest || p.x < 0 || p.x >= s.w {
				continue
			}
			allRest = false
			p.v += SPLASH_GRAVITY
			if p.y < mid { // above centre → fall down toward it, stack up
				p.y += p.v
				if p.y >= float64(s.restUp[p.x]) {
					p.y = float64(s.restUp[p.x])
					p.rest = true
					if s.restUp[p.x] > 0 {
						s.restUp[p.x]--
					}
				}
			} else { // below centre → rise up toward it, stack down
				p.y -= p.v
				if p.y <= float64(s.restDn[p.x]) {
					p.y = float64(s.restDn[p.x])
					p.rest = true
					if s.restDn[p.x] < s.h-1 {
						s.restDn[p.x]++
					}
				}
			}
		}
		return allRest
	}

	for i := range s.parts {
		p := &s.parts[i]
		if p.rest || p.x < 0 || p.x >= s.w {
			continue
		}
		allRest = false
		p.v += SPLASH_GRAVITY
		p.y += p.v
		if p.y >= float64(s.restLow[p.x]) {
			p.y = float64(s.restLow[p.x])
			p.rest = true
			if s.restLow[p.x] > 0 {
				s.restLow[p.x]--
			}
		}
	}
	return allRest
}

// Done reports whether the splash has finished.
func (s *Splash) Done() bool { return s.phase == splashDone }

// Render draws the current frame (s.h lines, no trailing newline).
func (s *Splash) Render() string {
	grid := make([][]rune, s.h)
	for y := range grid {
		grid[y] = make([]rune, s.w)
		for x := range grid[y] {
			grid[y][x] = ' '
		}
	}
	for _, p := range s.parts {
		yi := int(p.y + 0.5)
		if yi >= 0 && yi < s.h && p.x >= 0 && p.x < s.w {
			grid[yi][p.x] = p.glyph
		}
	}

	denom := float64(s.h - 1)
	if denom < 1 {
		denom = 1
	}
	hold := s.phase == splashHold
	peak := GetPeakColor(s.scheme)

	var b strings.Builder
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			ch := grid[y][x]
			if ch == ' ' {
				b.WriteByte(' ')
				continue
			}
			c := peak
			if !hold {
				c = GetColorForHeight(s.scheme, 1-float64(y)/denom)
			}
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(ch)))
		}
		if y < s.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
