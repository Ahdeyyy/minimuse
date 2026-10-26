The three half-second codec fixtures are from `github.com/gopxl/beep/v2`
v2.1.1, `internal/testdata` (MIT licensed), generated for decoder testing:

- `tone.mp3`: `valid_44100hz_x_padded_samples.mp3`
- `tone.flac`: `valid_44100hz_22050_samples_ffmpeg.flac`
- `tone.ogg`: `valid_44100hz_22050_samples.ogg`

Source: https://github.com/gopxl/beep/tree/v2.1.1/internal/testdata

WAV fixtures are synthesized by the tests so 24-bit precision can be checked.
