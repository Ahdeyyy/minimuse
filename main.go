package main

import (
	_ "embed"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/icon-32.png
var appIcon []byte

func main() {
	mygo.Use(native.Plugin)
	base, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	data := flag.String("data", filepath.Join(base, "MiniMuse"), "settings and library directory")
	startInTray := flag.Bool("tray", false, "start in the system tray")
	startPlaying := flag.Bool("play", false, "play the saved current song on startup")
	cpuProfile := flag.String("cpuprofile", "", "write a Go CPU profile until exit")
	flag.Parse()
	// A mostly static player needs no persistent graphics device. Software
	// rendering roughly halves this app's working set on the test machine.
	// Honor an explicit MYGO_GPU=1 for users who prefer GPU rendering.
	if os.Getenv("MYGO_GPU") == "" {
		_ = os.Setenv("MYGO_GPU", "0")
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		if err = pprof.StartCPUProfile(f); err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		defer pprof.StopCPUProfile()
	}
	path := filepath.Join(*data, "library.json")
	p, loadErr := loadPreferences(path)
	a := newApp(p, path)
	if loadErr != nil {
		// Preserve damaged settings for recovery before accepting new changes.
		backup := path + ".backup-" + time.Now().Format("20060102-150405")
		if err := os.Rename(path, backup); err != nil {
			log.Fatal(loadErr, "; could not back up library: ", err)
		}
		a.notice = "Saved library could not be read; a backup was kept at " + backup
	}
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if a.window != nil {
			files, tray, play := parseSecondInstance(args)
			if !tray || a.tray == nil {
				a.show()
			}
			if len(files) > 0 {
				a.importPaths(files, true)
			} else if play && !a.status.Playing {
				a.toggle()
			}
		}
	})
	mygo.App.WhenReady(func() {
		source := mygo.ThemeLight
		if a.prefs.Dark {
			source = mygo.ThemeDark
		}
		mygo.Theme.SetSource(source)
		bg := "#f1f3f8"
		if a.prefs.Dark {
			bg = "#08090f"
		}
		a.visible = !*startInTray
		a.window = mygo.NewWindow(a.windowOptions(runtime.GOOS, bg, *startInTray))
		a.setupUpdates()
		a.start()
		a.setupTray()
		if *startInTray && a.tray == nil {
			a.show()
		}
		if len(flag.Args()) == 0 {
			a.restorePlayback()
			if *startPlaying && !a.status.Playing {
				a.toggle()
			}
		}
		a.window.OnFileDrop(func(e *mygo.FileDropEvent) { a.importPaths(e.Paths) })
		a.window.OnHide(func() { a.visible = false; a.updatePulse() })
		a.window.OnShow(func() {
			a.visible = true
			a.status = a.engine.Snapshot()
			a.seek = a.status.Position.Seconds()
			a.updatePulse()
		})
		a.window.OnMinimize(func() { a.minimized = true; a.updatePulse() })
		a.window.OnRestore(func() { a.minimized = false; a.updatePulse() })
		a.window.OnClose(func(e *mygo.CloseEvent) {
			if !a.quit && a.prefs.CloseToTray && a.tray != nil {
				e.PreventDefault()
				a.persist()
				a.window.Hide()
				return
			}
			// StateKey belongs to the full library window, not the compact
			// always-on-top geometry.
			a.setMini(false)
		})
		mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quit = true; a.setMini(false) })
		a.window.OnClosed(a.shutdown)
		if len(flag.Args()) > 0 {
			a.importPaths(flag.Args(), true)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) windowOptions(goos, background string, hidden bool) mygo.WindowOptions {
	opts := mygo.WindowOptions{
		Title: "MiniMuse", Width: 1080, Height: 740, MinWidth: 820, MinHeight: 580,
		StateKey: "main", Content: ui.View(a.view), BackgroundColor: background, Hidden: hidden,
		TitleBarStyle: mygo.TitleBarHidden, HideWindowButtons: true, TitleBarHeight: 42,
	}
	if goos == "linux" {
		// Hidden title bars can still request compositor decorations on Wayland.
		// Frameless opts out entirely; our title bar supplies drag and controls.
		opts.Frameless = true
		opts.TitleBarStyle = mygo.TitleBarDefault
	}
	return opts
}

func parseSecondInstance(args []string) ([]string, bool, bool) {
	fs := flag.NewFlagSet("minimuse", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("data", "", "")
	fs.String("cpuprofile", "", "")
	tray := fs.Bool("tray", false, "")
	play := fs.Bool("play", false, "")
	if len(args) < 1 {
		return nil, false, false
	}
	if err := fs.Parse(args[1:]); err != nil {
		return nil, false, false
	}
	return fs.Args(), *tray, *play
}

func (a *app) show() { a.window.Show(); a.window.Focus() }
func (a *app) setupTray() {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Open MiniMuse", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
		{Label: "Mini player", Click: func(*mygo.MenuItem, *mygo.Window) { a.setMini(true); a.show() }},
		mygo.Separator(),
		{Label: "Play / Pause", Click: func(*mygo.MenuItem, *mygo.Window) { a.toggle() }},
		{Label: "Previous song", Click: func(*mygo.MenuItem, *mygo.Window) { a.previous() }},
		{Label: "Next song", Click: func(*mygo.MenuItem, *mygo.Window) { a.advance(false) }},
		mygo.Separator(),
		{Label: "Quit MiniMuse", Click: func(*mygo.MenuItem, *mygo.Window) { a.quit = true; a.window.Close() }},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: appIcon, ToolTip: "MiniMuse", Menu: menu})
	if err != nil {
		a.notice = "Tray unavailable; closing the window will quit. " + err.Error()
		return
	}
	a.tray = tray
	tray.OnClick(a.show)
}
func (a *app) shutdown() {
	if a.closed.Swap(true) {
		return
	}
	a.cancel()
	if a.saveTimer != nil {
		a.saveTimer.Stop()
	}
	if a.engine != nil {
		// Close drains pending commands and captures the final audible position
		// before releasing the decoder, including while hidden in the tray.
		a.engine.Close()
		a.rememberPlayback(a.engine.Snapshot())
	}
	a.persist()
	if a.saves != nil {
		close(a.saves)
		<-a.saved
	}
	if a.tray != nil {
		a.tray.Destroy()
	}
}
