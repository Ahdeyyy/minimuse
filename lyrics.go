package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"minimuse/internal/audio"
)

type lyricWord struct {
	At   time.Duration
	Text string
}
type lyricLine struct {
	At    time.Duration
	Text  string
	Words []lyricWord
}
type lyricDocument struct {
	Lines            []lyricLine
	Timed, WordTimed bool
	Source           string
}

var lyricStamp = regexp.MustCompile(`\[(\d+:\d{2}(?:[.:]\d{1,3})?)\]`)
var wordStamp = regexp.MustCompile(`<(\d+:\d{2}(?:[.:]\d{1,3})?)>`)
var lyricMeta = regexp.MustCompile(`^\[([a-zA-Z]+):(.*)\]$`)

// Enhanced LRC voice markers identify singers; they are not sung text.
var lyricVoice = regexp.MustCompile(`(?i)^((?:<\d+:\d{2}(?:[.:]\d{1,3})?>\s*)*)(?:v[1-9]\d*:|\[v[1-9]\d*:?\]|<v[1-9]\d*:?>)\s*`)

func parseLyricTime(s string) (time.Duration, bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	m, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	sc, err := strconv.ParseFloat(strings.ReplaceAll(parts[1], ":", "."), 64)
	if err != nil || sc < 0 || sc >= 60 {
		return 0, false
	}
	return time.Duration((float64(m)*60 + sc) * float64(time.Second)), true
}

func parseLyrics(text string) lyricDocument {
	text = strings.TrimPrefix(text, "\ufeff")
	var d lyricDocument
	var plain []lyricLine
	var offset time.Duration
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		raw = strings.TrimSpace(raw)
		if meta := lyricMeta.FindStringSubmatch(raw); meta != nil {
			if strings.EqualFold(meta[1], "offset") {
				if n, err := strconv.Atoi(strings.TrimSpace(meta[2])); err == nil {
					offset = time.Duration(n) * time.Millisecond
				}
			}
			continue
		}
		stamps := lyricStamp.FindAllStringSubmatchIndex(raw, -1)
		if len(stamps) == 0 || stamps[0][0] != 0 {
			plain = append(plain, lyricLine{Text: raw})
			continue
		}
		end := 0
		var times []time.Duration
		for _, match := range stamps {
			if match[0] != end {
				break
			}
			at, ok := parseLyricTime(raw[match[2]:match[3]])
			if !ok {
				break
			}
			times = append(times, at)
			end = match[1]
		}
		if len(times) == 0 {
			continue
		}
		body := lyricVoice.ReplaceAllString(strings.TrimSpace(raw[end:]), "${1}")
		var words []lyricWord
		matches := wordStamp.FindAllStringSubmatchIndex(body, -1)
		if len(matches) > 0 {
			if matches[0][0] > 0 {
				words = append(words, lyricWord{At: times[0], Text: body[:matches[0][0]]})
			}
			for j, m := range matches {
				at, ok := parseLyricTime(body[m[2]:m[3]])
				if !ok {
					continue
				}
				stop := len(body)
				if j+1 < len(matches) {
					stop = matches[j+1][0]
				}
				words = append(words, lyricWord{At: at, Text: body[m[1]:stop]})
			}
			body = wordStamp.ReplaceAllString(body, "")
		}
		for _, at := range times {
			line := lyricLine{At: at, Text: body}
			for _, w := range words {
				w.At += at - times[0]
				line.Words = append(line.Words, w)
			}
			d.Lines = append(d.Lines, line)
		}
	}
	d.Timed = len(d.Lines) > 0
	if !d.Timed {
		d.Lines = plain
	}
	for i := range d.Lines {
		line := &d.Lines[i]
		line.At = max(0, line.At+offset)
		for j := range line.Words {
			line.Words[j].At = max(0, line.Words[j].At+offset)
		}
		if len(line.Words) > 0 {
			d.WordTimed = true
		}
	}
	if d.Timed {
		slices.SortStableFunc(d.Lines, func(a, b lyricLine) int {
			if a.At < b.At {
				return -1
			}
			if a.At > b.At {
				return 1
			}
			return 0
		})
	}
	for len(d.Lines) > 0 && d.Lines[len(d.Lines)-1].Text == "" && !d.Timed {
		d.Lines = d.Lines[:len(d.Lines)-1]
	}
	return d
}

func (d lyricDocument) active(position time.Duration) int {
	if !d.Timed {
		return -1
	}
	i, _ := slices.BinarySearchFunc(d.Lines, position, func(line lyricLine, at time.Duration) int {
		if line.At <= at {
			return -1
		}
		return 1
	})
	return i - 1
}

func decodeLyrics(b []byte) string {
	if len(b) >= 2 && (b[0] == 255 && b[1] == 254 || b[0] == 254 && b[1] == 255) {
		little := b[0] == 255
		b = b[2:]
		var units []uint16
		for len(b) >= 2 {
			n := uint16(b[0])<<8 | uint16(b[1])
			if little {
				n = uint16(b[1])<<8 | uint16(b[0])
			}
			units = append(units, n)
			b = b[2:]
		}
		return string(utf16.Decode(units))
	}
	return string(b)
}

func localLyrics(t track) (lyricDocument, error) {
	base := strings.TrimSuffix(filepath.Base(t.Path), filepath.Ext(t.Path))
	dir := filepath.Dir(t.Path)
	for _, folder := range []string{dir, filepath.Join(dir, "lyrics")} {
		entries, err := os.ReadDir(folder)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return lyricDocument{}, err
		}
		for _, name := range []string{base, t.Artist + " - " + t.Title, t.Title} {
			for _, ext := range []string{".elrc", ".lrc", ".txt"} {
				for _, entry := range entries {
					if !entry.IsDir() && strings.EqualFold(entry.Name(), name+ext) {
						path := filepath.Join(folder, entry.Name())
						b, err := readSmallFile(path, 2<<20)
						if err != nil {
							return lyricDocument{}, err
						}
						d := parseLyrics(decodeLyrics(b))
						if len(d.Lines) == 0 {
							continue
						}
						d.Source = path
						return d, nil
					}
				}
			}
		}
	}
	return lyricDocument{}, nil
}

type lrclibRecord struct {
	PlainLyrics  string `json:"plainLyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
	Instrumental bool   `json:"instrumental"`
}

func downloadLyrics(ctx context.Context, client *http.Client, endpoint string, t track) (lyricDocument, error) {
	values := url.Values{"track_name": {t.Title}, "artist_name": {t.Artist}, "album_name": {t.Album}, "duration": {strconv.FormatFloat(t.Duration.Seconds(), 'f', 3, 64)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/get?"+values.Encode(), nil)
	if err != nil {
		return lyricDocument{}, err
	}
	req.Header.Set("User-Agent", "MiniMuse/0.1.0")
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return lyricDocument{}, fmt.Errorf("LRCLIB: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return lyricDocument{}, errors.New("No matching lyrics on LRCLIB. Check the song's title and artist tags")
	}
	if res.StatusCode != http.StatusOK {
		return lyricDocument{}, fmt.Errorf("LRCLIB returned %s. Try again later", res.Status)
	}
	var record lrclibRecord
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&record); err != nil {
		return lyricDocument{}, fmt.Errorf("Could not read LRCLIB response: %w", err)
	}
	if record.Instrumental {
		return lyricDocument{}, errors.New("LRCLIB identifies this song as instrumental")
	}
	body, ext := record.SyncedLyrics, ".lrc"
	if strings.TrimSpace(body) == "" {
		body, ext = record.PlainLyrics, ".txt"
	}
	if strings.TrimSpace(body) == "" {
		return lyricDocument{}, errors.New("No lyrics available on LRCLIB")
	}
	d := parseLyrics(body)
	if len(d.Lines) == 0 {
		return lyricDocument{}, errors.New("LRCLIB returned empty lyrics")
	}
	if ctx.Err() != nil {
		return lyricDocument{}, ctx.Err()
	}
	path := strings.TrimSuffix(t.Path, filepath.Ext(t.Path)) + ext
	// Exclusive creation preserves a user's sidecar even if it appeared during
	// the request. An existing local lyric file always takes priority.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return localLyrics(t)
	}
	if err != nil {
		return lyricDocument{}, fmt.Errorf("Could not save lyrics beside the song: %w", err)
	}
	_, writeErr := io.WriteString(f, body+"\n")
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(path)
		return lyricDocument{}, fmt.Errorf("Could not save lyrics: %w", errors.Join(writeErr, closeErr))
	}
	d.Source = path
	return d, nil
}

func (a *app) clearLyrics() {
	if a.lyricsCancel != nil {
		a.lyricsCancel()
		a.lyricsCancel = nil
	}
	a.lyricsGeneration++
	a.lyrics = lyricDocument{}
	a.lyricsPath = ""
	a.lyricsMessage = ""
	a.lyricsBusy = false
	a.lyricActive = -1
	a.lyricList = ui.ListState{}
	a.lyricTimingOpen = false
}

// Positive offsets delay the display; negative offsets bring lyrics forward.
func (a *app) lyricOffset() time.Duration {
	if tr := a.current(); tr != nil {
		return time.Duration(tr.LyricOffsetMS * float64(time.Millisecond))
	}
	return 0
}

func (a *app) lyricPosition() time.Duration { return a.status.Position - a.lyricOffset() }

func (a *app) lyricSeekTime(at time.Duration) time.Duration {
	at = max(0, at+a.lyricOffset())
	if a.status.Duration > 0 {
		at = min(at, a.status.Duration)
	}
	return at
}

func (a *app) lyricTimingChanged() {
	if tr := a.current(); tr != nil {
		tr.LyricOffsetMS = max(-300000, min(300000, tr.LyricOffsetMS))
	}
	a.lyricActive = -1
	a.persist()
}

func (a *app) loadLyrics(online bool) {
	t := a.current()
	if t == nil || a.window == nil {
		return
	}
	a.clearLyrics()
	a.lyricsPath = t.Path
	a.lyricsBusy = true
	a.lyricsMessage = "Looking for local lyrics…"
	if online {
		a.lyricsMessage = "Checking local lyrics, then LRCLIB…"
	}
	a.lyricMode = ""
	a.followLyrics = true
	ctx, cancel := context.WithCancel(a.ctx)
	a.lyricsCancel = cancel
	generation := a.lyricsGeneration
	song := *t
	go func() {
		defer cancel()
		d, err := localLyrics(song)
		if err == nil && len(d.Lines) == 0 && online {
			if song.Duration == 0 {
				if decoder, format, e := audio.Decode(song.Path); e == nil {
					song.Duration = format.SampleRate.D(decoder.Len())
					decoder.Close()
				}
			}
			d, err = downloadLyrics(ctx, &http.Client{Timeout: 20 * time.Second}, "https://lrclib.net", song)
		}
		if ctx.Err() != nil || a.closed.Load() {
			return
		}
		a.window.Update(func() {
			if a.closed.Load() || generation != a.lyricsGeneration || song.Path != a.prefs.Current {
				return
			}
			a.lyricsBusy = false
			a.lyrics = d
			a.lyricsMessage = ""
			if err != nil {
				a.lyricsMessage = err.Error()
			} else if len(d.Lines) == 0 {
				a.lyricsMessage = "No local lyrics found"
			}
			if d.WordTimed {
				a.lyricMode = "Words"
			} else if d.Timed {
				a.lyricMode = "Timed"
			} else {
				a.lyricMode = "Static"
			}
			a.updatePulse()
		})
	}()
}
