# MiniMuse

[![Listed on Awesome MyGo](https://awesomemygo.dev/badge.svg)](https://awesomemygo.dev/apps/minimuse)

MiniMuse is a small, native music player for your local library, written in Go. It is built for long listening sessions: no browser engine or web frontend, low memory and CPU use, and a quiet interface that stays out of the way.

Point it at your music folders and it builds a searchable library of songs, albums, and playlists, with artwork, synced lyrics, and a mini player that sits on top of other windows.

## Features

- **Formats:** MP3, FLAC, WAV, Ogg Vorbis, and AAC-LC in M4A. No FFmpeg needed.
- **Library:** add folders or files (or drag them in), search by title, artist, or album, and refresh folders to pick up new songs. Music files are never moved or deleted.
- **Albums:** browse releases with cover art, credits, song count, and total duration.
- **Playback:** play/pause, seek, volume, previous/next, shuffle, repeat, and optional crossfade.
- **Queue, favorites, and playlists:** edit the queue and save it as a named playlist.
- **Lyrics:** reads local `.elrc`, `.lrc`, and `.txt` files with static, line-synced, and word-by-word highlighting. Click a line to seek. **Get lyrics** fetches missing lyrics from LRCLIB on request.
- **Now playing screen:** album artwork, audio details, and lyrics.
- **Mini player:** a resizable, always-on-top player (**Ctrl+M**).
- **Tray:** keeps playing in the system tray after you close the window.
- **Resume:** reopens at the song and position you left off.
- **Themes:** light and dark.
- **Automatic updates:** signed updates from GitHub Releases.
- **Cross-platform:** Windows, Linux (x64/ARM64), and macOS (Intel and Apple Silicon).

### Keyboard shortcuts

| Shortcut | Action |
| --- | --- |
| Space / Ctrl+P | Play / pause |
| Ctrl+Left / Ctrl+Right | Previous / next song |
| Ctrl+M | Toggle mini player |
| Ctrl+L | Open the now playing screen |
| Ctrl+R | Refresh music folders |
| Ctrl+O | Add songs |
| Ctrl+F | Search |
| Ctrl+Q | Quit |

## Install

Download the latest build for your platform from [GitHub Releases](https://github.com/Ahdeyyy/minimuse/releases):

- **Windows:** `MiniMuse-setup-v<version>.exe` (installer) or `MiniMuse-v<version>.exe` (portable)
- **Linux:** `.deb` package or `.tar.gz` archive
- **macOS:** `.dmg` (ad hoc signed, not notarized)

## Build from source

Requires Go 1.27.3. MyGo is pinned as a Go tool in `go.mod`.

```sh
CGO_ENABLED=0 go test ./...
go tool mygo build
```

Release builds for every platform can be made with `./scripts/build-release.ps1`. See [PERFORMANCE.md](PERFORMANCE.md) for resource measurements.

## Built with

| Library | Purpose | License |
| --- | --- | --- |
| [MyGo](https://github.com/egoist/mygo) | Native UI, windowing, tray, packaging, and updates | MIT |
| [Beep](https://github.com/gopxl/beep) | Audio decoding, mixing, and resampling | MIT |
| [Oto](https://github.com/ebitengine/oto) | Native audio output | Apache-2.0 |
| [go-mp3](https://github.com/hajimehoshi/go-mp3) | MP3 decoding | Apache-2.0 |
| [mewkiz/flac](https://github.com/mewkiz/flac) | FLAC decoding | Unlicense |
| [oggvorbis](https://github.com/jfreymuth/oggvorbis) | Ogg Vorbis decoding | MIT |
| [go-m4a](https://github.com/tphakala/go-m4a) | M4A container parsing | MIT |
| [go-aac](https://github.com/tphakala/go-aac) | AAC decoding | LGPL-2.1-or-later (with BSD portions) |
| [simd](https://github.com/tphakala/simd) | SIMD routines for the AAC decoder | MIT |
| [go-text/typesetting](https://github.com/go-text/typesetting) | Text shaping | Unlicense / BSD-3-Clause |
| [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) | Image decoding and scaling | BSD-3-Clause |

Exact versions are pinned in `go.mod` and `go.sum`.

## Attribution

- **Martian Grotesk** typeface by [Evil Martians](https://github.com/evilmartians/grotesk), licensed under the SIL Open Font License 1.1 (`resources/fonts/OFL.txt`).
- **Lyrics** lookups are provided by [LRCLIB](https://lrclib.net), a free, community-run lyrics database.
- **Test fixtures** for MP3, FLAC, and Ogg come from [Beep](https://github.com/gopxl/beep/tree/v2.1.1/internal/testdata) (MIT); see `internal/audio/testdata/`.
- **go-aac** is used unmodified under the LGPL. Its license texts, along with those for go-m4a and simd, ship in `resources/licenses/`. To rebuild with a modified copy, run `go mod edit -replace github.com/tphakala/go-aac=<path>` and build again.

## License

MiniMuse is released under the [MIT License](LICENSE). Third-party components keep their own licenses, as listed above.
