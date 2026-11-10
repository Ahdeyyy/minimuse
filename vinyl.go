package main

import (
	"image"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// The record turns at 10 rpm: slow enough to read lyrics beside it, fast
// enough to read as playing. Its angle follows the playback position, so
// seeking back turns it backwards.
const (
	vinylDegreesPerSecond = 60
	// Jumps spin at most one and a half turns, however far they go.
	vinylMaxSpin = 540
	// A paused record coasts this far as it slows down, and starts from
	// rest when playback resumes.
	vinylCoast = 12
	// Larger than any record, so corners stay round while one resizes.
	roundRadius = 10000
)

// How long the record takes to settle on a new position: drift between
// playback samples, a seek, the pointer scrubbing, and pausing or resuming.
const (
	vinylDrift = 300 * time.Millisecond
	vinylJump  = 700 * time.Millisecond
	vinylScrub = 350 * time.Millisecond
	vinylStop  = 600 * time.Millisecond
)

// Shared identities let the compact player's pieces fly into the song
// screen and back.
type (
	coverMatch     struct{}
	titleMatch     struct{}
	artistMatch    struct{}
	transportMatch struct{}
	controlsMatch  struct{}
)

var (
	morphMotion  = ui.Animation{Spring: ui.SpringOptions{Duration: 520 * time.Millisecond, Bounce: .06}}
	coverFly     = ui.ElementTransition{Animation: &morphMotion, Match: coverMatch{}, Fly: true}
	titleFly     = ui.ElementTransition{Animation: &morphMotion, Match: titleMatch{}, Fly: true}
	artistFly    = ui.ElementTransition{Animation: &morphMotion, Match: artistMatch{}, Fly: true}
	transportFly = ui.ElementTransition{Animation: &morphMotion, Match: transportMatch{}, Fly: true}
	controlsFly  = ui.ElementTransition{Animation: &morphMotion, Match: controlsMatch{}, Fly: true}
	textMorph    = ui.ContentTransition{Morph: true, Animation: &morphMotion}
	// What the library and song screens do not share fades as the rest flies.
	// Colors alone leaves their size and position to follow the panel.
	screenFade = ui.ElementTransition{Duration: 260 * time.Millisecond, ExitDuration: 140 * time.Millisecond, Colors: true, Enter: &ui.Motion{Y: 10}, Exit: &ui.Motion{}}
)

// vinylClock is the record's angle as a function of time, so a Draw can turn
// it between playback samples without building the view. Each sample rebases
// it on the position's angle; the difference from what was showing eases
// away, turning the record back after a rewind instead of jumping.
type vinylClock struct {
	angle  float64 // Degrees at time at.
	at     time.Time
	rate   float64 // Degrees per second.
	offset float64 // Shown minus true angle at time at, easing to zero.
	settle time.Duration
}

func (v vinylClock) value(now time.Time) float64 {
	if v.at.IsZero() {
		return v.angle
	}
	elapsed := now.Sub(v.at)
	angle := v.angle + v.rate*elapsed.Seconds()
	if elapsed < v.settle {
		left := 1 - elapsed.Seconds()/v.settle.Seconds()
		angle += v.offset * left * left * left
	}
	return angle
}

// moving reports whether the record still turns at now.
func (v vinylClock) moving(now time.Time) bool {
	return v.rate != 0 || v.offset != 0 && now.Sub(v.at) < v.settle
}

// turnRecord rebases the record's clock when playback changes. Builds with
// the same position and state leave it alone, so it keeps turning smoothly
// however often the view rebuilds.
func (a *app) turnRecord(c *ui.Context) vinylClock {
	if c.Preferences().ReduceMotion {
		return vinylClock{}
	}
	position := a.seek
	if a.status.Path != a.vinylPath {
		// A new track starts its own position; keep the record where it is.
		a.vinylPath, a.vinylPosition = a.status.Path, position
	}
	playing := a.status.Playing && !a.status.Ended && !a.seeking
	delta := position - a.vinylPosition
	if delta == 0 && playing == a.vinylPlaying && !a.vinylClock.at.IsZero() {
		return a.vinylClock
	}
	settle := vinylDrift
	switch {
	case a.seeking:
		settle = vinylScrub
	case playing != a.vinylPlaying:
		settle = vinylStop
	case delta < 0 || delta > 2.5:
		settle = vinylJump
	}
	a.vinylPosition, a.vinylPlaying = position, playing
	a.vinylAngle += max(-vinylMaxSpin, min(vinylMaxSpin, delta*vinylDegreesPerSecond))
	now := c.Now()
	shown := a.vinylClock.value(now)
	next := vinylClock{angle: a.vinylAngle, at: now, settle: settle}
	if playing {
		next.rate = vinylDegreesPerSecond
	} else if !a.seeking {
		next.angle += vinylCoast
	}
	if !a.vinylClock.at.IsZero() {
		next.offset = shown - next.angle
	}
	a.vinylClock = next
	return next
}

// vinyl shows cover art as a turning record. It turns in its own Draw, so
// frames between playback samples repaint only the record, and only the
// cover turns: the grooves, sheen and spindle hole are one still overlay,
// which is also what makes the turning visible on covers with little detail.
func (a *app) vinyl(c *ui.Context, tr *track, size float32, label string) ui.Element {
	clock := a.turnRecord(c)
	cover := a.coverFor(tr)
	overlay := a.vinylOverlay(size, c.Theme().Surface)
	turner := a.vinylTurner(cover, size)
	return ui.Box(c.Key("vinyl")).Label(label).Role(ui.RoleImage).Size(size, size).Shrink(0).Radius(roundRadius).Transition(coverFly).
		Draw(func(p *ui.Painter, r ui.Rect) {
			radius := min(r.W, r.H) / 2
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			disc := ui.Rect{X: cx - radius, Y: cy - radius, W: 2 * radius, H: 2 * radius}
			now := p.Now()
			if clock.moving(now) {
				// A steady turn reads as smooth at 30 frames; seeks spin faster.
				if clock.rate != 0 && now.Sub(clock.at) >= clock.settle {
					p.AnimationFrameRate(30)
				} else {
					p.AnimationFrame()
				}
			}
			angle := math.Mod(clock.value(now), 360)
			if p.Accelerated() {
				p.Clip(disc, radius, func() {
					p.Transform(ui.Rotation(float32(angle)), cx, cy, func() { p.Image(cover, disc, ui.Cover) })
				})
			} else {
				// Turned at the record's size in device pixels; while flying it
				// scales, as an image does.
				px := int(math.Ceil(float64(size * p.Scale())))
				p.Image(turner.turn(angle, px), disc, ui.Cover)
			}
			p.Image(overlay, disc, ui.Cover)
		})
}

type vinylOverlayKey struct {
	px   int
	hole [4]uint8
}

// vinylOverlay returns the record's still surface for a size, rendered once.
func (a *app) vinylOverlay(size float32, hole ui.Color) *ui.Bitmap {
	px := vinylPixels(size)
	key := vinylOverlayKey{px, [4]uint8{hole.R, hole.G, hole.B, hole.A}}
	if b := a.vinylOverlays[key]; b != nil {
		return b
	}
	if a.vinylOverlays == nil || len(a.vinylOverlays) >= 8 {
		a.vinylOverlays = map[vinylOverlayKey]*ui.Bitmap{}
	}
	b := ui.NewBitmap(renderVinylOverlay(px, hole))
	a.vinylOverlays[key] = b
	return b
}

func renderVinylOverlay(px int, hole ui.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	radius := float64(px) / 2
	// Lines are measured in pixels of the overlay; widths follow the
	// record's size as strokes of the same proportion would.
	grooves := max(2, min(9, px/60))
	groove := max(1, radius/90)
	clamp := func(v float64) float64 { return max(0, min(1, v)) }
	type layer struct{ r, g, b, a float64 }
	black := func(a float64) layer { return layer{0, 0, 0, a} }
	white := func(a float64) layer { return layer{1, 1, 1, a} }
	holeColor := layer{float64(hole.R) / 255, float64(hole.G) / 255, float64(hole.B) / 255, 1}
	// The sheen's two highlights, as fractions of a turn from the top.
	sheen := [][2]float64{{0, 0}, {.08, .16}, {.2, 0}, {.5, 0}, {.58, .12}, {.7, 0}, {1, 0}}
	for y := range px {
		for x := range px {
			dx, dy := float64(x)+.5-radius, float64(y)+.5-radius
			d := math.Hypot(dx, dy)
			edge := clamp(radius - d + .5)
			if edge == 0 {
				continue
			}
			u := d / radius
			// Premultiplied colour, painted back to front.
			var r, g, b, al float64
			over := func(l layer, coverage float64) {
				a := l.a * coverage
				r, g, b, al = l.r*a+r*(1-a), l.g*a+g*(1-a), l.b*a+b*(1-a), a+al*(1-a)
			}
			ring := func(f, width float64) float64 { return clamp(width/2 + .5 - math.Abs(d-f*radius)) }
			// Darken towards the rim.
			t := clamp((u - .62) / .38)
			over(black(.38), t*t*(3-2*t))
			for i := range grooves {
				f := .94 - float64(i)*(.94-.34)/float64(grooves)
				over(black(.16), ring(f, groove))
				over(white(.05), ring(f-.02, groove))
			}
			over(black(.45), ring(.99, max(1.5, radius/40)))
			over(white(.22), ring(.3, max(1, radius/70)))
			turn := math.Mod(math.Atan2(dx, -dy)/(2*math.Pi)-20.0/360+2, 1)
			for i := 1; i < len(sheen); i++ {
				if from, to := sheen[i-1], sheen[i]; turn <= to[0] {
					over(white(from[1]+(to[1]-from[1])*(turn-from[0])/(to[0]-from[0])), 1)
					break
				}
			}
			// The spindle hole.
			over(holeColor, clamp(.07*radius-d+.5))
			over(black(.35), ring(.07, max(1, radius/80)))
			o := img.PixOffset(x, y)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = uint8(r*edge*255+.5), uint8(g*edge*255+.5), uint8(b*edge*255+.5), uint8(al*edge*255+.5)
		}
	}
	return img
}
