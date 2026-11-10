package main

import (
	"image"
	"math"

	"github.com/egoist/mygo/ui"
)

// vinylTurner turns a cover on the CPU. Drawn in memory (MYGO_GPU=0, or
// without a GPU MyGo can use), the renderer's transformed images cost about
// a microsecond a pixel, far too much for a record turning 30 times a
// second; turning the pixels here and drawing them untransformed costs a
// few milliseconds.
type vinylTurner struct {
	src    *image.RGBA // The cover, rendered once at the turner's size.
	bufs   [2]*image.RGBA
	next   int
	bitmap *ui.Bitmap
	angle  float64 // Last turned, NaN before the first turn.
}

type vinylTurnerKey struct {
	cover ui.ImageSource
	px    int
}

// vinylTurner returns the turner for a cover at a size, rendering the cover's
// pixels the first time. Sizes share the overlay's buckets.
func (a *app) vinylTurner(cover ui.ImageSource, size float32) *vinylTurner {
	px := vinylPixels(size)
	key := vinylTurnerKey{cover, px}
	if t := a.vinylTurners[key]; t != nil {
		return t
	}
	if a.vinylTurners == nil || len(a.vinylTurners) >= 4 {
		a.vinylTurners = map[vinylTurnerKey]*vinylTurner{}
	}
	src := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Image(c, cover).Size(float32(px), float32(px))
	}, px, px, 1)
	t := &vinylTurner{src: src, angle: math.NaN()}
	a.vinylTurners[key] = t
	return t
}

// vinylPixels is the pixel size for a record's overlay and turned cover:
// twice its size for high-DPI displays, in 32 DIP buckets so a window being
// resized does not render one per frame.
func vinylPixels(size float32) int {
	return min(640, int(math.Ceil(float64(size)/32))*64)
}

// turn returns the cover turned by angle degrees clockwise, px pixels
// square and cut to a circle with a smooth edge. Two buffers alternate, as
// Bitmap.Update keeps the last one it was given.
func (t *vinylTurner) turn(angle float64, px int) *ui.Bitmap {
	px = max(1, min(px, t.src.Rect.Dx()))
	buf := t.bufs[t.next]
	if buf == nil || buf.Rect.Dx() != px {
		t.bufs = [2]*image.RGBA{image.NewRGBA(image.Rect(0, 0, px, px)), image.NewRGBA(image.Rect(0, 0, px, px))}
		t.angle, buf = math.NaN(), t.bufs[t.next]
	}
	if angle == t.angle && t.bitmap != nil {
		return t.bitmap
	}
	t.angle = angle
	rotateDisc(buf, t.src, angle)
	if t.bitmap == nil {
		t.bitmap = ui.NewBitmap(buf)
	} else {
		t.bitmap.Update(buf)
	}
	t.next ^= 1
	return t.bitmap
}

// rotateDisc fills the disc inscribed in dst with src turned by degrees
// clockwise around its center, sampled bilinearly. Pixels outside the disc
// are left as they are, transparent in a new buffer.
func rotateDisc(dst, src *image.RGBA, degrees float64) {
	n, m := dst.Rect.Dx(), src.Rect.Dx()
	radius := float64(n) / 2
	scale := float64(m) / float64(n)
	sin, cos := math.Sincos(degrees * math.Pi / 180)
	// Stepping one pixel right in dst steps this far in src.
	stepX, stepY := cos*scale, -sin*scale
	last := float64(m - 1)
	inner := (radius - 1) * (radius - 1)
	sp, stride := src.Pix, src.Stride
	for y := range n {
		dy := float64(y) + .5 - radius
		span := math.Sqrt(max(0, radius*radius-dy*dy))
		x0, x1 := max(0, int(radius-span)-1), min(n, int(radius+span)+2)
		dx := float64(x0) + .5 - radius
		// The source point of dst's (x0, y), by the inverse turn.
		sx := (cos*dx+sin*dy)*scale + float64(m)/2 - .5
		sy := (-sin*dx+cos*dy)*scale + float64(m)/2 - .5
		row := dst.Pix[y*dst.Stride:]
		for x := x0; x < x1; x, dx, sx, sy = x+1, dx+1, sx+stepX, sy+stepY {
			d2 := dx*dx + dy*dy
			coverage := 1.0
			if d2 > inner {
				coverage = max(0, min(1, radius-math.Sqrt(d2)+.5))
			}
			o := x * 4
			if coverage == 0 {
				row[o], row[o+1], row[o+2], row[o+3] = 0, 0, 0, 0
				continue
			}
			fx, fy := max(0, min(last, sx)), max(0, min(last, sy))
			ix, iy := int(fx), int(fy)
			tx, ty := fx-float64(ix), fy-float64(iy)
			jx, jy := min(ix+1, m-1), min(iy+1, m-1)
			a, b := iy*stride+ix*4, iy*stride+jx*4
			c, d := jy*stride+ix*4, jy*stride+jx*4
			w00, w10 := (1-tx)*(1-ty)*coverage, tx*(1-ty)*coverage
			w01, w11 := (1-tx)*ty*coverage, tx*ty*coverage
			for k := range 4 {
				v := float64(sp[a+k])*w00 + float64(sp[b+k])*w10 + float64(sp[c+k])*w01 + float64(sp[d+k])*w11
				row[o+k] = uint8(v + .5)
			}
		}
	}
}
