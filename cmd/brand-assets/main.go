// Regenerate the small native app/tray icons from the shared vector logo.
package main

import (
	"image/png"
	"log"
	"os"

	"github.com/egoist/mygo/ui"
)

func main() {
	b, err := os.ReadFile("resources/logo.svg")
	if err != nil {
		log.Fatal(err)
	}
	logo := ui.MustParseSVG(b)
	for _, asset := range []struct {
		path string
		size int
	}{{"resources/icon.png", 256}, {"resources/icon-32.png", 32}} {
		img := ui.Render(func(c *ui.Context) {
			c.Root().Background(ui.Transparent)
			ui.Image(c, logo).Size(float32(asset.size), float32(asset.size))
		}, asset.size, asset.size, 1)
		f, err := os.Create(asset.path)
		if err != nil {
			log.Fatal(err)
		}
		err = png.Encode(f, img)
		f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}
}
