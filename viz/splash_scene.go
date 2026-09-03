package viz

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	_ "image/jpeg" // decoders for image.Decode
	_ "image/png"
	"math"
	"sort"
	"sync"
)

// hackerbot.jpg is the "HACKERBOT" still — a 1950s-tin-robot scene: a
// robot (ball antenna, glowing eyes, mouth grille, segmented neck/arms,
// leather jacket, arms crossed) in the same cluttered office, with a chrome
// "HACKERBOT" wordmark across the bottom. The splash renders it as one
// braille halftone.
//
//go:embed hackerbot.jpg
var splashStill []byte

const sceneWorkW = 1000 // processing resolution; downscaled from the ~1670px source

var (
	sceneOnce sync.Once
	sceneGray *image.Gray // processed luminance (levels + local contrast + edges)
	sceneRGBA *image.RGBA // plain downscale, for the per-cell colour tint
)

// sceneImages decodes and processes the embedded still once, then returns
// the shared results. Either return may be nil if the decode fails.
func sceneImages() (*image.Gray, *image.RGBA) {
	sceneOnce.Do(loadScene)
	return sceneGray, sceneRGBA
}

func loadScene() {
	src, _, err := image.Decode(bytes.NewReader(splashStill))
	if err != nil {
		return
	}
	sb := src.Bounds()
	w := sceneWorkW
	h := sb.Dy() * w / sb.Dx()
	if h < 1 {
		h = 1
	}
	rgba := downscaleRGBA(src, w, h)
	sceneRGBA = rgba

	lum := make([]float64, w*h)
	for i := 0; i < w*h; i++ {
		p := rgba.Pix[i*4 : i*4+3]
		lum[i] = (0.299*float64(p[0]) + 0.587*float64(p[1]) + 0.114*float64(p[2])) / 255
	}

	stretchLevels(lum, 0.02, 0.995) // pin the histogram, keep highlights off the clip

	blur := boxBlur(lum, w, h, 2)
	for i := range lum { // unsharp mask — local contrast so features read
		lum[i] = clampf(lum[i]+0.7*(lum[i]-blur[i]), 0, 1)
	}

	// Radial vignette: the far office corners (cabinet tops, the bulletin
	// boards, deep background) sink toward black — which in braille is empty
	// space — so the figure and the wordmark carry the frame.
	cx, cy := 0.5, 0.46
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := (float64(x)/float64(w) - cx) / 0.64
			dy := (float64(y)/float64(h) - cy) / 0.70
			v := clampf(1.30-0.82*math.Hypot(dx, dy), 0.18, 1.0)
			lum[y*w+x] *= v
		}
	}

	edge := sobel(lum, w, h)
	for i := range lum {
		// S-curve for tonal separation; add edges only into the darker
		// zones (they define the figure against shadow) and taper them out
		// of the bright wordmark so its letterforms stay open.
		v := scurve(lum[i], 2.0)
		lum[i] = clampf(v+0.42*edge[i]*(1-v), 0, 1)
	}

	g := image.NewGray(image.Rect(0, 0, w, h))
	for i, v := range lum {
		g.Pix[i] = uint8(clampf(v, 0, 1)*255 + 0.5)
	}
	sceneGray = g
}

// ---------------------------------------------------------------------------
// image / buffer ops
// ---------------------------------------------------------------------------

// downscaleRGBA box-averages src into a dw×dh RGBA. Runs once at startup.
func downscaleRGBA(src image.Image, dw, dh int) *image.RGBA {
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for dy := 0; dy < dh; dy++ {
		sy0 := sb.Min.Y + dy*sb.Dy()/dh
		sy1 := sb.Min.Y + (dy+1)*sb.Dy()/dh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < dw; dx++ {
			sx0 := sb.Min.X + dx*sb.Dx()/dw
			sx1 := sb.Min.X + (dx+1)*sb.Dx()/dw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, b, n uint64
			for y := sy0; y < sy1; y++ {
				for x := sx0; x < sx1; x++ {
					cr, cg, cb, _ := src.At(x, y).RGBA()
					r += uint64(cr)
					g += uint64(cg)
					b += uint64(cb)
					n++
				}
			}
			dst.SetRGBA(dx, dy, color.RGBA{
				R: uint8(r / n >> 8),
				G: uint8(g / n >> 8),
				B: uint8(b / n >> 8),
				A: 255,
			})
		}
	}
	return dst
}

// boxSampleGrayF area-averages g into an outW×outH float grid in [0,1].
func boxSampleGrayF(g *image.Gray, outW, outH int) []float64 {
	b := g.Bounds()
	out := make([]float64, outW*outH)
	for oy := 0; oy < outH; oy++ {
		y0 := b.Min.Y + oy*b.Dy()/outH
		y1 := b.Min.Y + (oy+1)*b.Dy()/outH
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for ox := 0; ox < outW; ox++ {
			x0 := b.Min.X + ox*b.Dx()/outW
			x1 := b.Min.X + (ox+1)*b.Dx()/outW
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sum, n float64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					sum += float64(g.GrayAt(xx, yy).Y)
					n++
				}
			}
			out[oy*outW+ox] = sum / n / 255
		}
	}
	return out
}

// atkinson dithers buf (row-major, w×h) in place to 0/1.
func atkinson(buf []float64, w, h int) {
	idx := func(x, y int) int { return y*w + x }
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			old := buf[idx(x, y)]
			nv := 0.0
			if old >= 0.5 {
				nv = 1
			}
			buf[idx(x, y)] = nv
			err := (old - nv) / 8
			for _, d := range [][2]int{{1, 0}, {2, 0}, {-1, 1}, {0, 1}, {1, 1}, {0, 2}} {
				nx, ny := x+d[0], y+d[1]
				if nx >= 0 && nx < w && ny >= 0 && ny < h {
					buf[idx(nx, ny)] += err
				}
			}
		}
	}
}

// stretchLevels remaps the lo/hi quantiles of buf to 0 and 1.
func stretchLevels(buf []float64, loQ, hiQ float64) {
	tmp := make([]float64, len(buf))
	copy(tmp, buf)
	sort.Float64s(tmp)
	lo := tmp[clampi(int(loQ*float64(len(tmp))), 0, len(tmp)-1)]
	hi := tmp[clampi(int(hiQ*float64(len(tmp))), 0, len(tmp)-1)]
	if hi-lo < 1e-6 {
		return
	}
	for i := range buf {
		buf[i] = clampf((buf[i]-lo)/(hi-lo), 0, 1)
	}
}

// boxBlur is a separable r-radius mean blur.
func boxBlur(buf []float64, w, h, r int) []float64 {
	out := make([]float64, len(buf))
	if r < 1 {
		copy(out, buf)
		return out
	}
	tmp := make([]float64, len(buf))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var s, n float64
			for k := -r; k <= r; k++ {
				if xx := x + k; xx >= 0 && xx < w {
					s += buf[y*w+xx]
					n++
				}
			}
			tmp[y*w+x] = s / n
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var s, n float64
			for k := -r; k <= r; k++ {
				if yy := y + k; yy >= 0 && yy < h {
					s += tmp[yy*w+x]
					n++
				}
			}
			out[y*w+x] = s / n
		}
	}
	return out
}

// sobel returns the normalised gradient magnitude of buf in [0,1].
func sobel(buf []float64, w, h int) []float64 {
	out := make([]float64, len(buf))
	at := func(x, y int) float64 {
		x = clampi(x, 0, w-1)
		y = clampi(y, 0, h-1)
		return buf[y*w+x]
	}
	var maxMag float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gx := -at(x-1, y-1) - 2*at(x-1, y) - at(x-1, y+1) +
				at(x+1, y-1) + 2*at(x+1, y) + at(x+1, y+1)
			gy := -at(x-1, y-1) - 2*at(x, y-1) - at(x+1, y-1) +
				at(x-1, y+1) + 2*at(x, y+1) + at(x+1, y+1)
			m := math.Hypot(gx, gy)
			out[y*w+x] = m
			if m > maxMag {
				maxMag = m
			}
		}
	}
	if maxMag > 0 {
		for i := range out {
			out[i] = math.Pow(out[i]/maxMag, 0.75) // lift faint edges
		}
	}
	return out
}

// scurve pushes midtones toward 0/1 with an S-shaped contrast curve.
func scurve(x, k float64) float64 {
	x = clampf(x, 0, 1)
	return clampf(0.5+0.5*math.Tanh((x-0.5)*k)/math.Tanh(0.5*k), 0, 1)
}

// aspectRows: how many cell-rows the still occupies at cols cells wide. A
// terminal cell is ~1:2 (w:h); with square braille dots that works out to
// rows = cols/2 · imgH/imgW.
func aspectRows(g *image.Gray, cols int) int {
	b := g.Bounds()
	r := float64(cols) / 2 * float64(b.Dy()) / float64(b.Dx())
	if n := int(math.Round(r)); n >= 1 {
		return n
	}
	return 1
}

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
