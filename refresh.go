package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"minimuse/internal/audio"
)

// Only a confirmed absence removes a source. Access and transient I/O errors
// leave the saved library intact. This runs on the import worker.
func missingMusic(ctx context.Context, roots []string, tracks []track) (available []string, folders, songs map[string]bool) {
	folders, songs = make(map[string]bool), make(map[string]bool)
	for _, root := range roots {
		if ctx.Err() != nil {
			return
		}
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			folders[pathKey(root)] = true
		} else {
			available = append(available, root)
		}
	}
	for _, tr := range tracks {
		if ctx.Err() != nil {
			return
		}
		for _, root := range roots {
			rel, err := filepath.Rel(pathKey(root), pathKey(tr.Path))
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				continue
			}
			if _, err := os.Stat(tr.Path); errors.Is(err, os.ErrNotExist) {
				songs[pathKey(tr.Path)] = true
			}
			break
		}
	}
	return
}

func (a *app) pruneMissingMusic(folders, songs map[string]bool) {
	a.prefs.Folders = slices.DeleteFunc(a.prefs.Folders, func(p string) bool { return folders[pathKey(p)] })
	a.prefs.Tracks = slices.DeleteFunc(a.prefs.Tracks, func(t track) bool { return songs[pathKey(t.Path)] })
	if songs[pathKey(a.prefs.Current)] {
		if a.engine != nil {
			a.engine.Stop()
		}
		a.prefs.Current = ""
		a.prefs.Position = 0
		a.prefs.Playing = false
		a.status = audio.Status{}
		a.seek = 0
		a.clearLyrics()
		a.updatePulse()
	}
	if len(songs) > 0 {
		a.bag = nil
		a.history = nil
	}
}
