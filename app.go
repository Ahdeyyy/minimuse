package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"minimuse/internal/audio"
)

type playbackEngine interface {
	Load(string) bool
	Toggle()
	Stop()
	Seek(time.Duration)
	Volume(float64)
	Crossfade(time.Duration)
	Snapshot() audio.Status
	Close()
}

type app struct {
	prefs            preferences
	path             string
	window           *mygo.Window
	tray             *mygo.Tray
	engine           playbackEngine
	status           audio.Status
	ctx              context.Context
	cancel           context.CancelFunc
	closed           atomic.Bool
	visible          bool
	minimized        bool
	quit             bool
	saves            chan preferences
	saved            chan struct{}
	theme            *ui.Theme
	section          string
	query            string
	filtered         []int
	byPath           map[string]int
	favoriteCount    int
	selected         int
	list             ui.ListState
	searchHandle     ui.Handle
	albums           []albumGroup
	albumIndex       map[string]int
	visibleAlbums    []int
	albumKey         string
	albumList        ui.ListState
	mini             bool
	miniVolumeOpen   bool
	mainBounds       mygo.Rectangle
	mainMaximized    bool
	miniBounds       mygo.Rectangle
	settings         bool
	foldersOpen      bool
	folderList       ui.ListState
	playlistDialog   bool
	playlistName     string
	playlistList     ui.ListState
	importing        bool
	notice           string
	queuePos         int
	bag              []string
	history          []string
	seek             float64
	muted            bool
	volumeSave       *time.Timer
	pulse            chan time.Duration
	songScreen       bool
	lyrics           lyricDocument
	lyricsPath       string
	lyricsMessage    string
	lyricsBusy       bool
	lyricsCancel     context.CancelFunc
	lyricsGeneration uint64
	lyricMode        string
	lyricList        ui.ListState
	lyricActive      int
	followLyrics     bool
	lyricTimingOpen  bool
	art              map[string]*artEntry
	artJobs          chan track
	artClock         uint64
}

func newApp(p preferences, path string) *app {
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{prefs: p, path: path, ctx: ctx, cancel: cancel, section: "All songs", selected: -1, queuePos: -1, visible: true, pulse: make(chan time.Duration, 1)}
	a.list.Selected = &a.selected
	a.art = make(map[string]*artEntry)
	a.artJobs = make(chan track, 64)
	a.lyricActive = -1
	a.followLyrics = true
	if a.prefs.CrossfadeSeconds <= 0 {
		a.prefs.CrossfadeSeconds = 4
	}
	// Older libraries did not retain their folder selections. Seed refresh
	// sources from their existing song directories once during migration.
	if len(a.prefs.Folders) == 0 {
		seen := make(map[string]bool)
		for _, t := range a.prefs.Tracks {
			folder := filepath.Dir(t.Path)
			key := pathKey(folder)
			if !seen[key] {
				a.prefs.Folders = append(a.prefs.Folders, folder)
				seen[key] = true
			}
		}
	}
	a.reindex()
	a.refreshFilter()
	a.setTheme()
	return a
}

func (a *app) start() {
	a.startArtwork()
	a.upgradeMetadata()
	a.saves = make(chan preferences, 1)
	a.saved = make(chan struct{})
	go func() {
		defer close(a.saved)
		for p := range a.saves {
			if err := savePreferences(a.path, p); err != nil && !a.closed.Load() {
				a.window.Update(func() { a.notice = "Could not save library: " + err.Error() })
			}
		}
	}()
	a.engine = audio.New(func(s audio.Status) {
		if !a.closed.Load() {
			a.window.Update(func() {
				if !a.closed.Load() {
					a.acceptStatus(s)
				}
			})
		}
	})
	a.engine.Volume(a.prefs.Volume / 100)
	a.configureCrossfade()
	// This timer exists only while a visible, non-minimized window is playing.
	go func() {
		var ticker *time.Ticker
		var ticks <-chan time.Time
		defer func() {
			if ticker != nil {
				ticker.Stop()
			}
		}()
		for {
			select {
			case <-a.ctx.Done():
				return
			case interval := <-a.pulse:
				if ticker != nil {
					ticker.Stop()
					ticker = nil
					ticks = nil
				}
				if interval > 0 {
					ticker = time.NewTicker(interval)
					ticks = ticker.C
				}
			case <-ticks:
				a.window.Update(func() {
					if !a.closed.Load() && a.visible && !a.minimized {
						s := a.engine.Snapshot()
						if s.Path == a.prefs.Current {
							a.status = s
							a.seek = s.Position.Seconds()
						}
					}
				})
			}
		}
	}()
}

func (a *app) updatePulse() {
	var interval time.Duration
	if a.visible && !a.minimized && a.status.Playing {
		interval = time.Second
		if a.songScreen && !a.mini && a.lyrics.Timed && a.lyricMode != "Static" {
			interval = 80 * time.Millisecond
		}
	}
	select {
	case <-a.pulse:
	default:
	}
	select {
	case a.pulse <- interval:
	default:
	}
}

func (a *app) persist() {
	if a.saves == nil {
		return
	}
	p := a.prefs
	p.Tracks = slices.Clone(p.Tracks)
	p.Folders = slices.Clone(p.Folders)
	p.Queue = slices.Clone(p.Queue)
	p.Playlists = slices.Clone(p.Playlists)
	for i := range p.Playlists {
		p.Playlists[i].Paths = slices.Clone(p.Playlists[i].Paths)
	}
	select {
	case <-a.saves:
	default:
	}
	a.saves <- p
}

func (a *app) reindex() {
	a.byPath = make(map[string]int, len(a.prefs.Tracks))
	a.favoriteCount = 0
	for i := range a.prefs.Tracks {
		t := &a.prefs.Tracks[i]
		t.index()
		a.byPath[pathKey(t.Path)] = i
		if t.Favorite {
			a.favoriteCount++
		}
	}
	// Restore only queue entries still present in the library.
	queue := a.prefs.Queue[:0]
	seen := make(map[string]bool)
	for _, p := range a.prefs.Queue {
		key := pathKey(p)
		if _, ok := a.byPath[key]; ok && !seen[key] {
			queue = append(queue, p)
			seen[key] = true
		}
	}
	a.prefs.Queue = queue
	for i := range a.prefs.Playlists {
		pl := &a.prefs.Playlists[i]
		pl.Paths = slices.DeleteFunc(pl.Paths, func(path string) bool { _, ok := a.byPath[pathKey(path)]; return !ok })
	}
	a.queuePos = slices.Index(a.prefs.Queue, a.prefs.Current)
	a.indexAlbums()
}

func (a *app) refreshFilter() {
	a.filtered = a.filtered[:0]
	a.selected = -1
	q := strings.ToLower(strings.TrimSpace(a.query))
	if a.section == "Albums" {
		a.filterAlbums(q)
		return
	}
	var paths []string
	if a.section == "Queue" {
		paths = a.prefs.Queue
	}
	if strings.HasPrefix(a.section, "playlist:") {
		for _, p := range a.prefs.Playlists {
			if a.section == "playlist:"+p.Name {
				paths = p.Paths
				break
			}
		}
	}
	if a.section == "Queue" || strings.HasPrefix(a.section, "playlist:") {
		for _, path := range paths {
			if i, ok := a.byPath[pathKey(path)]; ok && strings.Contains(a.prefs.Tracks[i].search, q) {
				a.filtered = append(a.filtered, i)
			}
		}
	} else {
		for i, t := range a.prefs.Tracks {
			if (a.section != "Favorites" || t.Favorite) && strings.Contains(t.search, q) {
				a.filtered = append(a.filtered, i)
			}
		}
	}
}

func (a *app) current() *track {
	if i, ok := a.byPath[pathKey(a.prefs.Current)]; ok {
		return &a.prefs.Tracks[i]
	}
	return nil
}

func (a *app) importPaths(paths []string, autoplay ...bool) {
	a.importMusic(paths, false, autoplay...)
}

func (a *app) importMusic(paths []string, refresh bool, autoplay ...bool) {
	if a.importing || len(paths) == 0 {
		return
	}
	a.importing = true
	a.notice = "Reading your music…"
	known := make(map[string]bool, len(a.byPath))
	for key := range a.byPath {
		known[key] = true
	}
	var oldTracks []track
	if refresh {
		oldTracks = slices.Clone(a.prefs.Tracks)
	}
	go func() {
		var missingFolders, missingTracks map[string]bool
		if refresh {
			paths, missingFolders, missingTracks = missingMusic(a.ctx, paths, oldTracks)
		}
		var folders []string
		for _, path := range paths {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				if abs, err := filepath.Abs(path); err == nil {
					folders = append(folders, abs)
				}
			}
		}
		tracks, skipped, err := scanTracks(a.ctx, paths, known)
		if a.closed.Load() {
			return
		}
		a.window.Update(func() {
			if a.closed.Load() {
				return
			}
			a.importing = false
			if a.ctx.Err() != nil {
				return
			}
			a.pruneMissingMusic(missingFolders, missingTracks)
			for _, folder := range folders {
				if !slices.ContainsFunc(a.prefs.Folders, func(p string) bool { return pathKey(p) == pathKey(folder) }) {
					a.prefs.Folders = append(a.prefs.Folders, folder)
				}
			}
			a.prefs.Tracks = append(a.prefs.Tracks, tracks...)
			a.reindex()
			a.refreshFilter()
			a.persist()
			a.notice = fmt.Sprintf("Added %d songs", len(tracks))
			if len(tracks) == 0 {
				a.notice = "No new supported songs found. Try MP3, FLAC, WAV, or Ogg Vorbis."
			}
			if refresh {
				a.notice = fmt.Sprintf("Refresh complete · %d added · %d missing songs removed · %d missing folders removed", len(tracks), len(missingTracks), len(missingFolders))
			}
			if skipped > 0 {
				a.notice += fmt.Sprintf(" · %d unreadable entries", skipped)
			}
			if err != nil {
				a.notice += " · " + err.Error()
			}
			if len(autoplay) > 0 && autoplay[0] {
				var queue []string
				for _, path := range paths {
					if abs, e := filepath.Abs(path); e == nil {
						if i, ok := a.byPath[pathKey(abs)]; ok {
							queue = append(queue, a.prefs.Tracks[i].Path)
						}
					}
				}
				if len(queue) == 0 {
					for _, t := range tracks {
						queue = append(queue, t.Path)
					}
				}
				if len(queue) > 0 {
					a.prefs.Queue = queue
					a.bag = nil
					a.history = nil
					a.refreshFilter()
					a.play(queue[0])
				}
			}
		})
	}()
}

func (a *app) browse(directory bool) {
	if a.window == nil || a.importing {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Title: "Add music", Directory: directory, Multiple: true, Filters: []mygo.FileFilter{{Name: "Music", Extensions: []string{"mp3", "flac", "wav", "ogg", "oga"}}}})
		if a.closed.Load() {
			return
		}
		a.window.Update(func() {
			if err != nil {
				a.notice = err.Error()
			} else {
				a.importPaths(paths)
			}
		})
	}()
}

func (a *app) play(path string) {
	if a.engine == nil {
		return
	}
	if !a.engine.Load(path) {
		a.notice = "Player is busy. Try again in a moment."
		return
	}
	a.prefs.Current = path
	a.status = audio.Status{Path: path}
	a.seek = 0
	a.updatePulse()
	if a.songScreen {
		a.loadLyrics(false)
	} else {
		a.clearLyrics()
	}
	a.queuePos = slices.Index(a.prefs.Queue, path)
	a.notice = ""
	a.persist()
}

func (a *app) playFrom(i int) {
	if i < 0 || i >= len(a.filtered) {
		return
	}
	if a.section != "Queue" {
		a.prefs.Queue = make([]string, len(a.filtered))
		for j, index := range a.filtered {
			a.prefs.Queue[j] = a.prefs.Tracks[index].Path
		}
	}
	a.bag = nil
	a.history = nil
	a.play(a.prefs.Tracks[a.filtered[i]].Path)
}

func (a *app) toggle() {
	if a.engine == nil {
		return
	}
	if a.status.Path == "" || a.status.Ended || a.status.Error != "" {
		if t := a.current(); t != nil {
			if !slices.Contains(a.prefs.Queue, t.Path) {
				a.prefs.Queue = append(a.prefs.Queue, t.Path)
			}
			a.play(t.Path)
		} else {
			a.playFrom(0)
		}
	} else {
		a.engine.Toggle()
	}
}

func (a *app) acceptStatus(s audio.Status) {
	// A quick song change can leave an older worker notification in the UI
	// queue. It must not replace the selected song or advance the new queue.
	if s.Path != a.prefs.Current {
		return
	}
	a.status = s
	a.seek = s.Position.Seconds()
	a.updatePulse()
	if s.Error != "" {
		a.notice = "Playback stopped: " + s.Error
	}
	if a.tray != nil {
		label := "MiniMuse"
		if t := a.current(); t != nil {
			label += " · " + t.Artist + " — " + t.Title
		}
		a.tray.SetToolTip(label)
	}
	if s.Ended || s.NearEnd && s.Playing && a.prefs.Crossfade && a.prefs.Repeat != 2 {
		a.advance(true)
	}
}

func (a *app) configureCrossfade() {
	if a.engine != nil {
		var duration time.Duration
		if a.prefs.Crossfade {
			duration = time.Duration(a.prefs.CrossfadeSeconds * float64(time.Second))
		}
		a.engine.Crossfade(duration)
	}
}

func (a *app) crossfadeChanged() {
	a.configureCrossfade()
	a.persist()
}

func (a *app) advance(automatic bool) {
	if len(a.prefs.Queue) == 0 {
		return
	}
	if automatic && a.prefs.Repeat == 2 {
		a.play(a.prefs.Current)
		return
	}
	var next string
	if a.prefs.Shuffle {
		if len(a.bag) == 0 {
			if automatic && a.prefs.Repeat == 0 && len(a.history) > 0 {
				return
			}
			a.bag = slices.Clone(a.prefs.Queue)
			a.bag = slices.DeleteFunc(a.bag, func(p string) bool { return p == a.prefs.Current })
			rand.Shuffle(len(a.bag), func(i, j int) { a.bag[i], a.bag[j] = a.bag[j], a.bag[i] })
		}
		if len(a.bag) > 0 {
			next = a.bag[len(a.bag)-1]
			a.bag = a.bag[:len(a.bag)-1]
		} else if !automatic || a.prefs.Repeat == 1 {
			next = a.prefs.Current
		}
	} else {
		pos := slices.Index(a.prefs.Queue, a.prefs.Current) + 1
		if pos >= len(a.prefs.Queue) {
			if automatic && a.prefs.Repeat == 0 {
				return
			}
			pos = 0
		}
		next = a.prefs.Queue[pos]
	}
	if next != "" {
		a.history = append(a.history, a.prefs.Current)
		if len(a.history) > 256 {
			a.history = slices.Clone(a.history[len(a.history)-256:])
		}
		a.play(next)
	}
}

func (a *app) previous() {
	if a.engine == nil {
		return
	}
	if a.status.Position > 3*time.Second {
		a.engine.Seek(0)
		return
	}
	if a.prefs.Shuffle && len(a.history) > 0 {
		p := a.history[len(a.history)-1]
		a.history = a.history[:len(a.history)-1]
		a.play(p)
		return
	}
	if len(a.prefs.Queue) > 0 {
		pos := slices.Index(a.prefs.Queue, a.prefs.Current) - 1
		if pos < 0 {
			pos = len(a.prefs.Queue) - 1
		}
		a.play(a.prefs.Queue[pos])
	}
}

func (a *app) favorite(path string) {
	if i, ok := a.byPath[pathKey(path)]; ok {
		t := &a.prefs.Tracks[i]
		t.Favorite = !t.Favorite
		if t.Favorite {
			a.favoriteCount++
		} else {
			a.favoriteCount--
		}
		a.refreshFilter()
		a.persist()
	}
}

func (a *app) enqueue(path string, next bool) {
	if slices.Contains(a.prefs.Queue, path) {
		a.notice = "This song is already in the queue"
		return
	}
	if next {
		pos := slices.Index(a.prefs.Queue, a.prefs.Current) + 1
		a.prefs.Queue = slices.Insert(a.prefs.Queue, pos, path)
	} else {
		a.prefs.Queue = append(a.prefs.Queue, path)
	}
	a.bag = nil
	a.refreshFilter()
	a.persist()
	a.notice = "Added to queue"
}

func (a *app) savePlaylist() {
	name := strings.TrimSpace(a.playlistName)
	if name == "" || len(a.prefs.Queue) == 0 {
		return
	}
	for _, p := range a.prefs.Playlists {
		if strings.EqualFold(p.Name, name) {
			a.notice = "A playlist with this name already exists"
			return
		}
	}
	a.prefs.Playlists = append(a.prefs.Playlists, playlist{Name: name, Paths: slices.Clone(a.prefs.Queue)})
	a.playlistDialog = false
	a.playlistName = ""
	a.persist()
	a.notice = "Playlist saved"
}

func (a *app) addToPlaylist(name, path string) {
	for i := range a.prefs.Playlists {
		p := &a.prefs.Playlists[i]
		if p.Name == name {
			if !slices.Contains(p.Paths, path) {
				p.Paths = append(p.Paths, path)
				a.refreshFilter()
				a.persist()
				a.notice = "Added to " + name
			}
			return
		}
	}
}

func (a *app) remove(path string, library bool) {
	a.prefs.Queue = slices.DeleteFunc(a.prefs.Queue, func(p string) bool { return p == path })
	if library {
		a.prefs.Tracks = slices.DeleteFunc(a.prefs.Tracks, func(t track) bool { return t.Path == path })
	}
	if a.prefs.Current == path {
		if a.engine != nil {
			a.engine.Stop()
		}
		a.prefs.Current = ""
		a.clearLyrics()
		a.status = audio.Status{}
		a.updatePulse()
	}
	a.bag = nil
	a.history = nil
	a.reindex()
	a.refreshFilter()
	a.persist()
}

func (a *app) setVolume() {
	a.muted = false
	if a.engine != nil {
		a.engine.Volume(a.prefs.Volume / 100)
	}
	// A slider can change every pointer event. Save once it settles, rather
	// than cloning/serializing a large library for every intermediate value.
	if a.saves == nil {
		return
	}
	if a.volumeSave != nil {
		a.volumeSave.Stop()
	}
	a.volumeSave = time.AfterFunc(350*time.Millisecond, func() {
		if !a.closed.Load() {
			a.window.Update(func() {
				if !a.closed.Load() {
					a.persist()
				}
			})
		}
	})
}
func (a *app) mute() {
	a.muted = !a.muted
	if a.engine != nil {
		v := a.prefs.Volume / 100
		if a.muted {
			v = 0
		}
		a.engine.Volume(v)
	}
}
