package main

import "github.com/egoist/mygo/ui"

// Paint a soft halo directly from cached glyphs. Keeping this out of bitmap
// filters avoids full-paragraph blur buffers in the software renderer.
type lyricGlow struct {
	text   string
	font   ui.Font
	width  float32
	layout *ui.TextLayout
	ranges []ui.TextInputRange
	color  ui.Color
}

func (fx *lyricGlow) Paint(p *ui.Painter, box ui.Rect) {
	if fx.layout == nil || fx.width != box.W {
		fx.layout = ui.ShapeText(fx.text, fx.font, box.W)
		fx.width = box.W
	}
	// Clip each shifted draw to its sung fragment, preserving bidi and wrapping.
	for _, r := range fx.layout.SelectionRects(fx.ranges...) {
		for _, tap := range [...]struct{ x, y, weight float32 }{
			{-1.5, -1.5, .05}, {0, -1.5, .1}, {1.5, -1.5, .05},
			{-1.5, 0, .1}, {0, 0, .2}, {1.5, 0, .1},
			{-1.5, 1.5, .05}, {0, 1.5, .1}, {1.5, 1.5, .05},
			{-3.5, 0, .025}, {3.5, 0, .025}, {0, -3.5, .025}, {0, 3.5, .025},
		} {
			clip := ui.Rect{X: box.X + r.X + tap.x, Y: box.Y + r.Y - 2.5 + tap.y, W: r.W, H: r.H}
			p.Transform(ui.Scaling(1.075, 1.075), box.X+r.X+r.W*.5, box.Y+r.Y+r.H*.65, func() {
				p.Clip(clip, 0, func() {
					p.TextLayout(fx.layout, box.X+tap.x, box.Y-2.5+tap.y, fx.color.Alpha(tap.weight*.5))
				})
			})
		}
	}
}
