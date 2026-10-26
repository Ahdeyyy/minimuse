package main

import (
	"embed"
	"fmt"
	"hash/fnv"

	"github.com/egoist/mygo/ui"
)

//go:embed resources/fonts/*.ttf
var fonts embed.FS

//go:embed resources/logo.svg
var vinylSVG []byte

var vinylLogo = ui.MustParseSVG(vinylSVG)

func init() {
	for _, weight := range []string{"Regular", "Medium", "Bold"} {
		b, err := fonts.ReadFile("resources/fonts/MartianGrotesk-" + weight + ".ttf")
		if err == nil {
			err = ui.RegisterFont(b, "Martian Grotesk")
		}
		if err != nil {
			panic(err)
		}
	}
}

func (a *app) setTheme() {
	t := ui.LightTheme()
	t.Font = "Martian Grotesk"
	t.FontSize = 13
	t.Radius = 8
	t.Spacing = 4
	t.Background = ui.Hex("#f1f3f8")
	t.Surface = ui.Hex("#ffffff")
	t.SurfaceHover = ui.Hex("#e9edf6")
	t.SurfacePressed = ui.Hex("#dfe5f5")
	t.Border = ui.Hex("#dce2ed")
	t.Text = ui.Hex("#20293e")
	t.TextMuted = ui.Hex("#5d697f")
	t.Accent = ui.Hex("#5260d8")
	t.AccentText = ui.Hex("#ffffff")
	if a.prefs.Dark {
		t.Dark = true
		t.Background = ui.Hex("#08090f")
		t.Surface = ui.Hex("#101119")
		t.SurfaceHover = ui.Hex("#1a1c2b")
		t.SurfacePressed = ui.Hex("#25283d")
		t.Border = ui.Hex("#252737")
		t.Text = ui.Hex("#e9edf7")
		t.TextMuted = ui.Hex("#9ba9c3")
		t.Accent = ui.Hex("#a3adff")
		t.AccentText = ui.Hex("#171d35")
	}
	t.AccentHover = t.Accent.Mix(t.Text, .12)
	t.AccentPressed = t.Accent.Mix(t.Text, .22)
	t.Selection = ui.Hex("#e8ebfb")
	if t.Dark {
		t.Selection = ui.Hex("#25283d")
	}
	t.Focus = t.Accent.Alpha(.6)
	t.Scrollbar = t.TextMuted.Alpha(.3)
	a.theme = t
}

var icons = func() map[string]*ui.SVG {
	paths := map[string]string{
		"record":   `<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="7"/><circle cx="12" cy="12" r="3"/><circle cx="12" cy="12" r=".6"/>`,
		"refresh":  `<path d="M20 7v5h-5M4 17v-5h5"/><path d="M5.1 7a8 8 0 0 1 13.2-2L20 7M4 17l1.7 2A8 8 0 0 0 19 17"/>`,
		"minimize": `<path d="M5 12h14"/>`,
		"maximize": `<rect x="5" y="5" width="14" height="14" rx="1"/>`,
		"restore":  `<path d="M9 9V4h11v11h-5"/><rect x="4" y="9" width="11" height="11" rx="1"/>`,
		"lyrics":   `<path d="M4 5h16M4 10h12M4 15h16M4 20h9"/>`,
		"music":    `<path d="M9 18V5l12-2v13M9 8l12-2"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>`,
		"songs":    `<rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/>`,
		"heart":    `<path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z"/>`,
		"queue":    `<path d="M4 6h16M4 12h16M4 18h10m4-2 3 2-3 2"/>`,
		"folder":   `<path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/><path d="M9 14h6m-3-3v6"/>`,
		"plus":     `<path d="M12 5v14M5 12h14"/>`,
		"search":   `<circle cx="10.5" cy="10.5" r="7.5"/><path d="m16 16 5 5"/>`,
		"play":     `<path d="m8 4 12 8-12 8Z" fill="currentColor" stroke="none"/>`,
		"pause":    `<path d="M7 5h3v14H7zM14 5h3v14h-3z" fill="currentColor" stroke="none"/>`,
		"next":     `<path d="m4 5 11 7-11 7Z" fill="currentColor"/><path d="M19 5v14"/>`,
		"previous": `<path d="m20 5-11 7 11 7Z" fill="currentColor"/><path d="M5 5v14"/>`,
		"shuffle":  `<path d="m18 3 3 3-3 3M3 18h3c5 0 6-12 11-12h4M3 6h3c2 0 3 2 4 4m4 4c1 2 2 4 4 4h3m-3-3 3 3-3 3"/>`,
		"repeat":   `<path d="m17 2 4 4-4 4M3 11V8a2 2 0 0 1 2-2h16M7 22l-4-4 4-4m14-3v5a2 2 0 0 1-2 2H3"/>`,
		"volume":   `<path d="m11 5-6 4H2v6h3l6 4ZM15 8a6 6 0 0 1 0 8m3-11a10 10 0 0 1 0 14"/>`,
		"mute":     `<path d="m11 5-6 4H2v6h3l6 4ZM17 9l5 6m0-6-5 6"/>`,
		"sun":      `<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/>`,
		"moon":     `<path d="M21 13a9 9 0 0 1-10-10 9 9 0 1 0 10 10Z"/>`,
		"settings": `<path d="M4 6h16M4 12h16M4 18h16"/><circle cx="9" cy="6" r="2" fill="currentColor"/><circle cx="15" cy="12" r="2" fill="currentColor"/><circle cx="8" cy="18" r="2" fill="currentColor"/>`,
		"close":    `<path d="m6 6 12 12M6 18 18 6"/>`,
		"more":     `<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`,
		"albums":   `<rect x="4" y="4" width="16" height="16" rx="3"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1"/>`,
		"mini":     `<rect x="3" y="4" width="18" height="16" rx="2"/><rect x="12" y="11" width="7" height="6" rx="1"/>`,
		"expand":   `<path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5M3 3l6 6m12-6-6 6M3 21l6-6m12 6-6-6"/>`,
		"back":     `<path d="m14 5-7 7 7 7"/>`,
		"check":    `<path d="m5 12 4 4L19 6"/>`,
		"follow":   `<path d="M5 4h14M5 20h14M12 7v10m-3-3 3 3 3-3"/>`,
	}
	paths["repeat-one"] = paths["repeat"] + `<path d="m10 10 2-2v8m-2 0h4"/>`
	paths["heart-filled"] = `<g fill="currentColor">` + paths["heart"] + `</g>`
	out := make(map[string]*ui.SVG, len(paths))
	for name, path := range paths {
		out[name] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`))
	}
	return out
}()

// Small shared vector covers replace a potentially unbounded bitmap cache.
var covers = func() []*ui.SVG {
	colors := [][3]string{{"#d7dfef", "#596b9d", "#a0b1d3"}, {"#e8dfd0", "#8f7457", "#c6b395"}, {"#d7e4de", "#537b6a", "#9db9ab"}, {"#e5dbea", "#8b6896", "#bba3c5"}, {"#eedcd8", "#a3675e", "#d0a59c"}}
	var out []*ui.SVG
	for _, c := range colors {
		out = append(out, ui.MustParseSVG([]byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200"><rect width="200" height="200" fill="%s"/><circle cx="100" cy="100" r="77" fill="%s"/><circle cx="100" cy="100" r="59" fill="none" stroke="%s" stroke-width="1"/><circle cx="100" cy="100" r="48" fill="none" stroke="%s" stroke-width="1"/><circle cx="100" cy="100" r="37" fill="none" stroke="%s" stroke-width="1"/><circle cx="100" cy="100" r="20" fill="%s"/><circle cx="100" cy="100" r="4" fill="%s"/></svg>`, c[0], c[1], c[2], c[2], c[2], c[0], c[1]))))
	}
	return out
}()

func coverFor(t *track) *ui.SVG {
	if t == nil {
		return covers[0]
	}
	h := fnv.New32a()
	h.Write([]byte(t.Artist + t.Album))
	return covers[int(h.Sum32())%len(covers)]
}

func iconButton(c *ui.Context, name, label string, active bool) ui.Element {
	t := c.Theme()
	if name == "heart" && active {
		name = "heart-filled"
	}
	b := ui.Button(c, "").Label(label).Tooltip(label).Size(34, 34).Padding(0).Gap(0).Center().Shrink(0).Border(0, ui.Transparent).Background(ui.Transparent).TextColor(t.Text).Radius(8)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	if active {
		b.TextColor(t.Accent).Background(t.Selection)
	}
	b.Children(func() { ui.Icon(c, icons[name]).Size(18, 18).Shrink(0).Label(label + " icon") })
	return b
}

func textButton(c *ui.Context, name, label string) ui.Element {
	b := ui.Button(c, "").Label(label).Padding(9, 12).Gap(8).Center()
	b.Children(func() {
		if name != "" {
			ui.Icon(c, icons[name]).Size(16, 16).Shrink(0)
		}
		ui.Text(c, label).NoWrap().Shrink(0)
	})
	return b
}
