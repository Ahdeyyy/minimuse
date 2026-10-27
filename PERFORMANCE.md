# Performance review — 27 October 2026

Reviewed playback/device access, decoder reads and seeking, crossfades, command delivery, library/search/album indexing, visible-row rendering, lyrics, artwork, imports, metadata migration, persistence, startup, and shutdown. Changes preserve the 48 kHz float output and resampler quality.

## Findings and changes

| Area | Finding | Change |
| --- | --- | --- |
| MP3 startup and import | The decoder scans compressed frame headers through thousands of tiny file reads. | Added a 32 KiB buffered reader that preserves seek offsets and skips forward within buffered data. |
| Playback resilience | A 125 ms queue leaves limited margin for scheduling and storage delays. Reads return just 512 frames at a time. | Raised the queue target to 250 ms (96,000 bytes); refill batches supply up to 2,048 frames with the existing small scratch array. Actual queued audio can exceed the target by one batch. |
| Scrubbing and volume | Slow operations can leave obsolete slider commands queued, or fill the old 64-entry channel. | Adjacent pending seek, volume, and crossfade commands replace their previous value. Playback actions retain FIFO ordering. Shutdown has a reserved queue slot. |
| Persistence during controls | Every seek notification cloned the library and requested JSON serialization and a synchronized disk write. | Debounce position-only notifications and continuous settings for 350 ms. Play/pause changes flush promptly, including when progress sampled the state first. Shutdown captures and saves the final state. Obsolete timer callbacks are rejected. |
| Lyric timing | A 60 Hz UI sampled position published every 40 ms, producing repeated positions. | Extrapolate between samples for at most 40 ms, bounded by duration. Paused, stopped, failed, and closed playback does not extrapolate. This is display smoothing, not a hardware playback clock. |
| UI scheduling | Progress callbacks could queue repeatedly behind a slow frame. | Allow at most one pending progress callback, sampling the latest engine state when it runs. |
| Lyric/button allocation | Every refresh rebuilt accessibility labels and unused theme-button style callbacks. | Cache lyric labels per document and use MyGo button bases for controls whose appearance is already fully customized. Keyboard/focus behavior remains covered by UI tests. |
| Older libraries | Duration migration posted one UI update per file. | Publish batches of up to 64 results, also flushing after 200 ms of scanning. Wait for each batch to be consumed before posting another; window cancellation releases the wait. |
| End seeking | Source/output sample-rate rounding could leave samples at EOF; MP3's decoder also cannot seek to a nonexistent EOF frame. | Map the output endpoint exactly to source EOF and represent native-decoder EOF without requesting a compressed frame. Seeking back resumes normally. |

## Measurements

Windows amd64, Intel i5-1145G7, eight logical CPUs, Go 1.27.3, MyGo 0.6.4. Compare the same fixture/workload; results are not representative of every library or recording.

| Measurement | Before | After |
| --- | ---: | ---: |
| MP3 open, 128 concatenated copies of the fixture | 20.27 ms | 1.12–1.22 ms |
| Source reads during that MP3 open | 11,359 | 21 |
| Library refresh allocation, 10,000 tracks | about 6,460 bytes / 207 allocations | about 5,120 bytes / 179 allocations |
| Animated word-lyric refresh allocation, 200 lines and 10,000 tracks | 90,528 bytes / 2,759 allocations | 66,815 bytes / 2,022 allocations |
| Mini-player refresh allocation | about 1,090 bytes / 42 allocations | about 705 bytes / 34 allocations |

The MP3 benchmark improves roughly 17 times; lyric refresh allocation falls roughly 26%. UI refresh timing varied substantially between runs, so this review does **not** claim a reliable UI CPU speedup. The UI benchmark advances animation time and explicitly refreshes the view; an iteration may render multiple frames. It uses software rendering at 1080×740, or 420×240 for the mini player, and covers a short passage rather than a full listening session. Before/after UI allocation comparisons used the same benchmark and 600 iterations with the previous source supplied through a Go overlay.

PCM conversion/resampling retains zero allocations per steady-state read. MP3 and FLAC decoders still allocate internally; this change does not eliminate codec allocation or MP3's frame-index scan.

A muted real-device lifecycle test passed play/pause, seek, completion, crossfade, rapid skips, disabling crossfade, restored playback, and decoder/file cleanup. Its additional **30-second** WAV soak reported live Go heap **0.86 → 0.95 MiB** and **0.21 MiB** total allocation. Those numbers exclude GUI/native-driver memory and do not measure audible dropouts. The lifecycle test was rerun after the final code changes.

## Validation and remaining limits

- `CGO_ENABLED=0 go test ./...`, `go vet ./...`, and `go tool mygo vet .` pass.
- New regression tests cover command ordering/coalescing/concurrent shutdown, interpolation bounds, buffered seek offsets, batched MP3 reads, refill size, all-codec EOF seeking, and avoiding library saves during a seek burst.
- UI interaction/layout coverage includes the 10,000-track virtualized library, lyric highlighting/scrolling/seeking, compact layouts, and mini-player controls. Captured song and compact mini-player output was visually inspected.
- `go tool mygo build` produces the Windows executable and installer in `build/windows-amd64/`.
- The race detector could not run: `gcc` is absent. The concurrent regression test passes without race instrumentation.
- There is no measured underrun counter or long-duration mixed-codec/device stress result. A short muted soak cannot prove zero audible stuttering.
- True gapless playback is still unimplemented. Automatic transitions still involve the UI queue and opening the next decoder; a heavily blocked UI or slow storage can delay a transition. Crossfade remains optional.
- Buffer headroom is finite. Storage/device stalls longer than the available decoded audio can still interrupt playback. High-DPI/fullscreen lyric rendering and very large cover images should be profiled on the target setup before choosing software versus GPU rendering.

## Reproduce

```powershell
$env:CGO_ENABLED='0'
go test ./...
go vet ./...
go tool mygo vet .
go test -run '^$' -bench . -benchmem ./internal/audio
go test -run '^$' -bench BenchmarkPlaybackFrame -benchtime=600x -benchmem .
$env:MINIMUSE_AUDIO_INTEGRATION='1'
$env:MINIMUSE_AUDIO_SOAK_SECONDS='30'
go test -run TestRealDeviceLifecycle -count=1 -v ./internal/audio
Remove-Item Env:MINIMUSE_AUDIO_INTEGRATION, Env:MINIMUSE_AUDIO_SOAK_SECONDS
```
