# MiniMuse

A small, native Go music player for long listening sessions. Its Martian Grotesk typography, quiet indigo palette, soft borders, and rounded panels follow BentoMail's design. No browser engine or web frontend runs in the player.

## Run

Download `MiniMuse-v<version>.exe` (portable) or `MiniMuse-setup-v<version>.exe` (installer) from GitHub Releases. For local builds, open `build/windows-amd64/MiniMuse.exe`, or install with `minimuse-setup-0.1.0-amd64.exe` in the same folder. The executable is portable; its fonts and icons are embedded.

Add a music folder, add individual files, or drop files/folders onto the window. Double-click a song or press Enter in the song list to play it. Search matches titles, artists, and albums.

- MP3, FLAC, WAV (integer PCM and floating point), Ogg Vorbis (`.ogg`, `.oga`), and AAC-LC in M4A (`.m4a`). Mono and stereo sources are supported. M4A playback is built in and needs no external FFmpeg installation; HE-AAC, Apple Lossless (ALAC), and DRM-protected M4A are not supported.
- Play/pause, seek, volume/mute, previous/next, shuffle without repeats within a cycle, and repeat off/all/one.
- Reopening restores the current song at its saved playback position. Playing sessions resume automatically; paused sessions stay paused. Opening music files explicitly starts those files instead.
- Favorites, editable queue, and named playlists. Open Queue and use its `+` button to save a mix. A song's menu adds it to the queue, plays it next, or adds it to a saved playlist. Queue menus move songs up/down or remove them. Playlist menus remove songs; right-click a playlist to delete it.
- Albums: open **Albums** in the sidebar and search by album/artist. Open a release for cover artwork that fades into the song list, artist and featured-artist credits from the tags, song count, and total duration. Its search matches only songs and artists within that album; **Play album** queues the whole release. The artwork scrolls away while search remains accessible. Album-artist tags group releases; folders distinguish albums when that tag is absent and keep compilations together. Disc/track tags order newly imported tracks; existing files use their filenames when those tags are unavailable.
- Mini player: click the mini-player button at the right of the playback bar, press **Ctrl+M**, or select **Mini player** from the tray menu. Resize down to 260×180, use a wide playback strip, or enlarge it into an artwork view. Narrow layouts put shuffle/repeat in **Playback options** and volume/mute in a popover. It reuses the same playback engine and window and stays on top. The expand button or **Ctrl+M** restores the library's size and position. Closing follows the same close-to-tray setting.
- Light/dark appearance and remembered library, queue, playlists, favorites, and preferences.
- Custom draggable titlebar with matching window controls, native resize edges, and a transparent vinyl-record app/tray icon. Linux uses a frameless window so the desktop does not add a second titlebar.
- In-app notifications appear as dismissible, auto-expiring overlay toasts in both the library and mini player, without moving the controls or song list.
- Song durations in the library, with readable selection colors in both themes. Existing libraries fill in durations in the background.
- **Playing now** opens a dedicated song screen with album artwork, audio details, and lyrics. Click the current song again in the library, click it in the bottom playback bar, or press **Ctrl+L** to open it without restarting playback; **Back to library** returns to your previous view.
- Artwork comes from embedded MP3/FLAC/Ogg covers, or a song-named image or `cover`, `folder`, `front`, `album`, or `artwork` image beside the music (JPEG, PNG, WebP). Missing artwork uses the record illustration. Visible covers load in a worker; the cache holds at most 48 images resized to 512 pixels.
- **Refresh folders** or **Ctrl+R** scans remembered folders recursively for newly added songs without duplicating tracks. Confirmed missing folders and missing songs under refreshed folders are removed from the library, queue, and playlists; playback stops if the current song was removed. Access errors retain saved entries. Existing songs keep their favorites and playback. Settings groups saved folders in a collapsible, scrollable list. Older libraries seed refresh sources from the folders containing their songs.
- Closing the window keeps playback in the tray by default. Click the tray icon to reopen; right-click for playback controls and **Quit MiniMuse**. Settings can make closing quit instead. If tray creation fails, closing exits normally.

Library removal never deletes music files. Music stays in its original location. MP3 ID3v2.3/v2.4 and FLAC/Ogg text tags are read during import; missing/unsupported tags fall back to the filename and folder.

### Lyrics

The song screen checks locally first, with no automatic network requests. Place `Song.elrc`, `Song.lrc`, or `Song.txt` beside `Song.flac` (or another supported audio file), or inside its folder's `lyrics` subfolder. `Artist - Title` and `Title` filenames also work. UTF-8 and UTF-16 files are supported. Exact song filenames take priority, with enhanced LRC preferred over LRC and plain text.

**Static** displays the text; **Timed** follows standard `[00:12.34]` line timestamps; **Words** highlights enhanced LRC timestamps such as `[00:12.34]<00:12.34>Hello <00:12.80>world`. Multiple timestamps per line and LRC offsets are supported. Timed lines can be clicked to seek. Turn off **Follow playback** to browse manually. The target icon, **Return to current lyric**, centers the current line without changing playback or your follow setting. Modes requiring timestamps are disabled when the file lacks them; word timings are never guessed.

Timed lyrics glide between lines using non-bouncing springs, with smooth changes in emphasis and gentler scaling and fading around the current line. Wrapped lines are centered using their measured height. Words progressively fills each word using its timestamps. Currently sung words use the accent color with a soft glyph glow and a gentle visual enlargement and lift; completed words keep a quieter fill, and upcoming words stay muted. Lead and backing vocals highlight independently, and a delayed frame still fills every completed word. Seeking reconstructs the fill at the new position. Opening lyrics and distant seeks place the new passage immediately. These animations respect the desktop's reduced-motion preference, and turning off **Follow playback** stops the glide for manual browsing. Follow playback sits at the far right of the lyric controls (an icon button in compact windows); a **Local** badge beside Lyrics identifies a loaded lyric file.

**Settings → Crossfade between tracks** enables an overlap for automatic transitions and manual song changes. Set **Fade duration** from 1–12 seconds; shorter values switch faster. Crossfade starts disabled with a remembered four-second duration. It follows queue, shuffle, and repeat-all order; repeat-one restarts normally. Short tracks limit the overlap to half their duration. Pause freezes both tracks, seeking ends the overlap, and disabling crossfade immediately returns the current track to full playback volume.

Active lines use larger, bolder text; surrounding lines are smaller and muted. Overlapping vocals highlight independently, including timed `[bg:…]` backing vocals and words sharing a timestamp. Trailing word timestamps end a vocal without cutting off another line that is still being sung; follow playback stays with the lead vocal. Word highlighting uses text color, and enhanced LRC singer markers such as `v1:` are hidden. **Timing** offers a per-song offset from −300,000 to +300,000 milliseconds, **Earlier**/**Later** buttons in 100 ms steps, and **Reset**. Positive values delay lyrics; negative values show them earlier. The saved adjustment applies to line/word highlighting and click-to-seek in addition to any offset in the lyric file, without rewriting it.

**Get lyrics** explicitly looks up the song through [LRCLIB](https://lrclib.net/docs), using title, artist, album, and duration. A successful result is saved beside the audio as `.lrc` (synced) or `.txt` (plain) and works offline afterward. Existing local lyrics take priority and are never overwritten. **Reload local lyrics** rereads edited or newly added sidecars. Network failures, missing matches, instrumentals, and folders that cannot be written show a message in the lyrics panel. LRCLIB's available timing determines whether timed or word modes can be used.

Keyboard: **Space** or **Ctrl+P** plays/pauses; **Ctrl+Left/Right** changes songs; **Ctrl+M** switches mini player mode; **Ctrl+L** opens the song screen; **Ctrl+R** refreshes music folders; **Ctrl+O** adds songs; **Ctrl+F** searches the library; **Ctrl+Q** quits. Space remains available for text entry in search/dialogs.

Command-line options:

```powershell
.\MiniMuse.exe -tray -play               # start in tray and play the saved song
.\MiniMuse.exe -data .\my-library        # choose a settings directory
.\MiniMuse.exe 'C:\Music\song.flac'      # import and play a file
```

Settings normally live in `%APPDATA%\MiniMuse\library.json`. Writes use a synchronized temporary file followed by atomic replacement. A malformed settings file is backed up before starting a fresh library. Audio never starts automatically unless requested with a file or `-play`.

## Resource use and audio

Only the current track is decoded for playback, with one additional stream during a crossfade. Imports and a one-time upgrade of older library entries open decoders to read durations, then close them. Each playback stream targets a 250 ms PCM queue (96,000 bytes), plus at most one 2,048-frame refill batch, decoder, resampler, device, and small scratch buffers. MP3 frame scanning uses a 32 KiB file buffer. Tracks are never decoded into a full-song cache. FLAC seeking searches compressed frame offsets and checks frame CRCs, avoiding a full-song decode on the first seek, including files without seek tables. Other compressed formats may retain small seek tables proportional to file duration. The saved library keeps text metadata, durations, folder sources, paths, and the last playback position and playing state; visible artwork uses a bounded in-memory cache.

Playback work, imports, artwork/lyrics loading, downloads, file dialogs, and persistence run off the UI thread. The song and lyric lists virtualize visible rows; searches run only when the query or library changes. Playback progress refreshes once a second while visible, or up to 60 times a second while synchronized lyrics are displayed, with interpolation between device samples. Progress updates cannot accumulate a UI callback backlog. They stop when hidden/minimized and sleep when paused. Pausing suspends the output device. Consecutive pending seek/volume commands coalesce; seek, volume, lyric-offset, and crossfade-setting saves wait 350 ms for controls to settle. No periodic disk writes occur while listening to one song. Older-library metadata upgrades publish bounded batches instead of redrawing for every track.

Software rendering is the default to avoid the graphics driver's persistent memory cost for this mostly static interface. Set `MYGO_GPU=1` before launching to opt into GPU rendering; it may suit very large/high-DPI windows better. Close all existing instances for the change to take effect.

Audio uses a 32-bit floating-point stereo path at 48 kHz through Oto's native output (WASAPI on Windows, with a WinMM fallback). 48 kHz inputs bypass application resampling; other rates use Beep's quality-4 real-time resampler. The WAV/MP3 decoders reuse scratch buffers. 24-bit lossless input precision survives the float path. Volume is limited to attenuation, with no EQ, normalization, boost, or artificial enhancement. This is shared-mode playback, not exclusive/bit-perfect output: system processing, the source recording, and the output device still affect the sound. Lossy formats cannot regain discarded detail.

Measurements on this Windows machine (Intel i5-1145G7, 8 logical CPUs), using a muted stereo 48 kHz WAV and a one-song library:

- Revised software-rendered tray playback: **65.2 MiB working set**, **51.9 MiB private bytes**, **0.23% total-machine CPU** (1.82% of one core), over 12 seconds.
- Earlier GPU build: about 88 MiB working set / 116 MiB private bytes during tray playback. This was before the WAV buffer optimization, so its CPU result is not a renderer-only comparison.
- Separate audio-engine device soak: live Go heap **1.69 → 1.77 MiB** across 20 seconds, with 0.23 MiB total allocated during that interval. This excludes the GUI and native driver memory.
- PCM conversion and resampling benchmarks: zero allocations per steady-state read.

These are short, local measurements, not a claim of an hours-long soak or fixed memory use for every codec/library/device. Use `scripts/measure-process.ps1 -ProcessId <id> -Seconds 30 -Phase tray` to measure your own library. For CPU profiling, start with `-cpuprofile <path>` and quit normally to finish the profile; Windows profiles can attribute blocked native waits to syscall stacks, so use OS CPU counters for actual utilization.

The October 2026 review, measured improvements, and remaining playback limits are recorded in [PERFORMANCE.md](PERFORMANCE.md). Its MP3-opening benchmark improved about 17 times, and animated-lyric refresh allocation fell about 26%. These are fixture measurements, not a guarantee for every file or device.

## Development and checks

Go 1.27.3 and MyGo 0.7.1 are pinned in `go.mod`. Windows is the verified target.

```powershell
$env:CGO_ENABLED='0'
go test ./...
go vet ./...
go tool mygo vet .
go tool mygo build
```

The build produces the Windows executable and installer under `build/windows-amd64/`. `go run ./cmd/brand-assets` regenerates the app/tray PNGs from `resources/logo.svg`.

### Automatic releases

Settings includes automatic update checks, optional automatic installation, and a manual **Check for updates** button. Checks run shortly after launch and daily. The native update window shows release notes, download progress, errors, and the option to restart. Preferences are retained by MyGo in its user data directory. Development builds, unsigned builds, and installs without write access show an unavailable message; package manager installs should use their package manager.

To enable signed updates for releases, run `go tool mygo keygen`, keep the generated private key safe outside this repository, and set the GitHub Actions repository variable `MYGO_UPDATER_PUBLIC_KEY` to the public key and repository secret `MYGO_UPDATER_PRIVATE_KEY` to the private key. The release script embeds the public key and the `Ahdeyyy/minimuse` feed and publishes MyGo's signed archives and `update-<platform>.json` manifests using their original filenames. Only push builds receive the private key; PR builds never sign. Existing unsigned installations need a one-time manual upgrade to a build with the public key. Local signed builds use the same environment variables. Without these keys the workflow continues producing ordinary downloads with automatic updates unavailable.

`.github/workflows/release.yml` tests, vets, and builds Windows x64, Linux x64/ARM64, and macOS universal (Intel and Apple Silicon) on pull requests targeting `master` and on every push to `master`. Windows, Linux, and macOS use native OS runners; Linux ARM64 is cross-compiled on Linux x64 and macOS packages both architectures. Only successful pushes publish a GitHub release after all builds pass, including pushes caused by merging a PR. The tag targets the exact tested commit. Publishing uses the built-in `GITHUB_TOKEN` with `contents: write`; no personal access token is needed.

The release version uses the major/minor version from `mygo.json` and adds the workflow run number to its patch number. With the configured `0.1.0`, run 1 produces `v0.1.1`, run 2 produces `v0.1.2`, and so on. PR checks also consume run numbers, so release numbers can have gaps. Reruns keep their version; publishing an existing release fails rather than replacing its files. Change the major/minor in `mygo.json` when starting a new release series; keep this workflow's filename stable so its run counter continues.

Each release includes:

- `MiniMuse-v0.1.1.exe`: portable executable.
- `MiniMuse-setup-v0.1.1.exe`: installer.
- `MiniMuse-v0.1.1-linux-amd64.tar.gz` and `MiniMuse-v0.1.1-linux-arm64.tar.gz`: Linux executables, resources, and desktop integration files. Extract and run `./minimuse` from the extracted directory.
- `MiniMuse-v0.1.1-linux-amd64.deb` and `MiniMuse-v0.1.1-linux-arm64.deb`: Debian/Ubuntu/Pop!_OS packages.
- `MiniMuse-v0.1.1-darwin-universal.dmg`: macOS app for Intel and Apple Silicon. The bundle uses ad hoc signing; Developer ID signing and notarization are not configured.
- `MiniMuse-v0.1.1-<platform>.sha256`: SHA-256 checksums for each platform's downloads.

To build the same release files locally with PowerShell, run `./scripts/build-release.ps1` (Windows x64, configured version), or `./scripts/build-release.ps1 -BuildNumber 1` (the configured patch plus 1). Pass `-Platform linux/amd64`, `-Platform linux/arm64`, or `-Platform darwin/universal` for the other targets. Files go to `build/releases/v<version>/<platform>/`; the script restores `mygo.json` after building. MyGo downloads NSIS on Windows if needed. Building the macOS disk image requires macOS.

Optional hardware and visual checks:

```powershell
$env:MINIMUSE_AUDIO_INTEGRATION='1'
go test -run TestRealDeviceLifecycle -count=1 -v ./internal/audio
$env:MINIMUSE_AUDIO_SOAK_SECONDS='20'
go test -run TestRealDeviceLifecycle -count=1 -v ./internal/audio
Remove-Item Env:MINIMUSE_AUDIO_INTEGRATION, Env:MINIMUSE_AUDIO_SOAK_SECONDS
$env:MINIMUSE_CAPTURE_DIR="$PWD/build/preview"
go test -run TestCaptureUI -count=1 .
go test -run '^$' -bench . -benchmem ./internal/audio
```

Tests cover decoder/seek behavior for all supported formats, retained 24-bit lossless precision, M4A presentation trimming and metadata, file handle cleanup, queue boundaries, stale playback events, shuffle cycles, persistence replacement/corruption, metadata import, playlists, keyboard actions, and a 10,000-song virtualized library. The opt-in sound-device test is muted and exercises play, pause, seek, resume, completion, and optional sustained playback. Native close-to-tray and restore were also checked on Windows. The race detector could not run here because no C compiler is installed.

HE-AAC, ALAC, raw AAC (`.aac`), WMA, Opus, network streams, exclusive output, gapless transitions, and OS media-key integration are not implemented. The M4A decoder produces 16-bit PCM before conversion to the float output path; imports include iTunes title, artist, album, album artist, track/disc numbers, and embedded cover art.

M4A uses [go-m4a](https://github.com/tphakala/go-m4a) v0.8.0 (MIT), [go-aac](https://github.com/tphakala/go-aac) v0.7.1 (LGPL-2.1-or-later, with BSD portions), and simd v1.14.0 (MIT). License texts ship in `resources/licenses/`. These libraries are unmodified; their exact versions and checksums are pinned in `go.mod` and `go.sum`. To rebuild with a modified codec, replace its module with a local checkout using `go mod edit -replace github.com/tphakala/go-aac=<path>`, then run the development build commands above.

Martian Grotesk ships under its OFL license in `resources/fonts/OFL.txt`. Codec test fixtures and their MIT license are attributed in `internal/audio/testdata/`.
