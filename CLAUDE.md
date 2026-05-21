# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`lapis` is a Go CLI tool that strips identifying metadata from JPEG image files. Built for journalists and privacy-sensitive users. The companion `indigo` binary (v2) and `goindigo` (v3) are placeholders.

## Build and test commands

```sh
go build ./...                          # build all binaries
go build -o lapis ./cmd/lapis           # build lapis binary
go test ./...                           # run all tests
go test ./internal/strip/               # run tests for one package
go test -run TestName ./internal/strip/ # run a single test
GOOS=windows GOARCH=amd64 go build -o lapis.exe ./cmd/lapis  # cross-compile
```

## Module

`codeberg.org/elkarrde/lapis` — Go 1.22+. `go.mod` must have **zero `require` entries**. No external dependencies. Pure standard library only.

## Architecture

```
cmd/lapis/main.go       ← CLI entry point, flag parsing, orchestration (flag stdlib only)
cmd/indigo/main.go      ← placeholder only, prints "not yet implemented"
internal/strip/         ← JPEG segment stripping engine (core logic)
  strip.go              ← public Strip() API, JPEG segment parser/writer, level dispatch
  exif.go               ← EXIF IFD parser (resolves all values from offsets) + serializer
internal/rename/        ← filename scrambling (scramble, uuid)
internal/timestamp/     ← filesystem timestamp editing
```

### JPEG segment manipulation

All metadata work operates directly on the binary JPEG segment structure — no EXIF libraries. A JPEG is a sequence of segments, each starting with `0xFF` + type byte. Key segments:

- `APP1` (`0xFFE1`): EXIF data (starts `Exif\0\0`) or XMP (starts `http://ns.adobe.com/xap/1.0/\x00`)
- `APP13` (`0xFFED`): IPTC/Photoshop data
- `APP0` (`0xFFE0`): JFIF marker — always preserve
- `SOI` (`0xFFD8`) / `EOI` (`0xFFD9`): always preserve
- `SOS`, `SOF`, `DHT`, `DQT`: pixel data segments — copy verbatim, never modify

**Do not use `goexif` or any third-party EXIF library** — they are read-only and would add a dependency. Raw byte operations only.

### Stripping levels

| Level | Action |
|-------|--------|
| `scout` | Parse APP1, remove GPS IFD pointer (0x8825) from IFD0. IPTC location fields in APP13 are a pending TODO. |
| `journalist` | Excise APP13 entirely. Rebuild APP1 keeping only shooting data tags. Remove all XMP APP1 segments. IFD1 (thumbnail) is never emitted. |
| `ghost` | Excise all APP1 and APP13 segments. Print steganography warning to stderr. |

Ghost level warning (exact text, printed to stderr):
```
WARNING: Pixel-level steganographic fingerprints are not addressed by --level ghost.
         Camera manufacturers (Canon, Nikon, Fuji) may embed invisible identifying
         patterns in image data. Use goindigo (planned) for full mitigation.
```

**Tags preserved at journalist level:** ExposureTime, FNumber, ISOSpeedRatings, ApertureValue, ExposureBiasValue, MaxApertureValue, Flash, FocalLength, FocalLengthIn35mmFilm, ImageWidth, ImageLength, ColorSpace, DateTimeOriginal.

### EXIF IFD parser and serializer (`internal/strip/exif.go`)

`parseEXIF` reads all IFD entries and resolves their values from TIFF data offsets into `[]byte`. Sub-IFDs (Exif, GPS) are followed and stored separately. IFD1 (thumbnail) is intentionally skipped at all levels — its entries reference raw thumbnail bytes by offset that cannot be safely relocated without specialized handling.

`buildEXIF` serializes IFD0 + optional Exif sub-IFD + optional GPS sub-IFD with freshly computed offsets. Layout: TIFF header | IFD0 | ExifSub | GPSSub | external value data. Sub-IFD pointer entries (0x8769, 0x8825) in IFD0 must be present as placeholder entries; `buildEXIF` patches their values.

Tags 0xA005 (Interoperability IFD) and 0x014A (SubIFDs) are stripped at scout level because their values are TIFF offsets that cannot be rewritten safely during rebuild.

### Output behaviour

- Default: write to `_lapis/` subdirectory alongside input. Never modify originals unless `--in-place` is passed.
- If output file already exists, append `_1`, `_2` etc. — never overwrite.
- If input is a directory: process all `.jpg`/`.jpeg` (case-insensitive). With `--recursive`, mirror directory structure inside `_lapis/`.
- Non-JPEG files: print warning and skip, do not abort batch.
- End summary: `Processed: N  Skipped: N  Errors: N`

### Filename scrambling

- `scramble`: 8-char lowercase hex string (e.g. `a3f9c2d1.jpg`) — `crypto/rand`, 4 bytes formatted as `%08x`
- `uuid`: UUID4 — 16 `crypto/rand` bytes, version bits `0100` in byte 6, variant bits `10` in byte 8. No UUID library.

### Timestamp editing

The `--time` flag takes a value: `random`, `shift`, or `now`.

- `random`: random `time.Time` between configurable range (default 2015-01-01 to 2023-12-31) via `os.Chtimes`
- `shift`: one random offset per batch (±365 days) stored in `Options.Shift *time.Duration`, reused across all files to preserve relative ordering
- `now`: `time.Now()`

Currently applies to filesystem timestamps only (`os.Chtimes`). Applying to EXIF DateTime fields is a pending TODO.

Windows creation time (`syscall.CreateFileW` + `SetFileTime` behind `//go:build windows`) is also a pending TODO.

## CLI flags

```
lapis [options] <file|directory>
  --level             scout | journalist | ghost  (default: journalist)
  --rename            scramble | uuid            (default: none)
  --time              random | shift | now       (default: none)
  --time-range-start  YYYY-MM-DD                 (default: 2015-01-01)
  --time-range-end    YYYY-MM-DD                 (default: 2023-12-31)
  --in-place          modify files directly
  --recursive         process subdirectories
  --verbose           print per-file actions
  --version           print version and exit
```

## Testing

Tests live in `internal/strip/strip_test.go` (white-box, `package strip`) and `internal/rename/rename_test.go` (white-box, `package rename`). Synthetic JPEG fixtures are built from scratch using a `jpegBuilder` helper — no real photos.

Required strip test cases (all passing):
- Valid JPEG with GPS → GPS tags absent after `scout`
- Valid JPEG → APP1 and APP13 absent after `ghost`
- Non-JPEG file → graceful error, no output written
- JPEG with no metadata → passes through without corruption
- JPEG with IFD1 thumbnail → thumbnail absent after `journalist`

## v1 scope boundary

Do not build in v1: mimic filename mode, `--time-set` flag, RAW/PNG/TIFF support, audit/dry-run mode, actual `indigo` logic, `goindigo`, any third-party dependency, GUI/TUI/interactive prompts.
