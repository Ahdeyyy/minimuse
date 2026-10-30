The three half-second codec fixtures are from `github.com/gopxl/beep/v2`
v2.1.1, `internal/testdata` (MIT licensed), generated for decoder testing:

- `tone.mp3`: `valid_44100hz_x_padded_samples.mp3`
- `tone.flac`: `valid_44100hz_22050_samples_ffmpeg.flac`
- `tone.ogg`: `valid_44100hz_22050_samples.ogg`

`tone.m4a` is an original one-second stereo 440 Hz AAC-LC tone at 44.1 kHz,
generated locally with FFmpeg (not copied from the Beep fixtures). It carries
iTunes text tags and an edit list that trims encoder priming and final padding:

```sh
ffmpeg -f lavfi -i 'sine=frequency=440:sample_rate=44100:duration=1' -ac 2 -c:a aac -aac_pns 0 -metadata title='M4A Tone' -metadata artist='MiniMuse' -metadata album='Codec tests' -metadata album_artist='MiniMuse ensemble' -metadata track='2/4' -metadata disc='1/2' tone.m4a
```

Noise substitution is disabled so seek output can be compared with sequential
decoding sample for sample. Tests require no FFmpeg installation.

Source: https://github.com/gopxl/beep/tree/v2.1.1/internal/testdata

WAV fixtures are synthesized by the tests so 24-bit precision can be checked.
