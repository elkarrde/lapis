# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`lapis` is a Go CLI tool that strips identifying metadata from JPEG image files. Built for journalists and privacy-sensitive users. The companion `indigo` binary (v2) and `goindigo` (v3) are placeholders.

## Build and test commands

```sh
go build ./...                          # build all binaries
go build -o lapis ./cmd/lapis           # build lapis binary
go test ./...                           # run all tests
go test ./internal/strip/...            # run tests for one package
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
internal/rename/        ← filename scrambling (scramble, uuid)
internal/timestamp/     ← filesystem + EXIF timestamp editing
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
| `scout` | Parse APP1, remove GPS IFD tags only. Remove IPTC location fields from APP13 if present. |
| `journalist` | Excise APP13 entirely. Rebuild APP1 keeping only shooting data tags. Remove MakerNote, serials, owner/artist/copyright, software, ImageUniqueID, IFD1 (embedded thumbnail), all XMP APP1 segments. |
| `ghost` | Excise APP1 and APP13 entirely. Print steganography warning to stderr. |

Ghost level warning (print to stderr exactly):
```
WARNING: Pixel-level steganographic fingerprints are not addressed by --level ghost.
         Camera manufacturers (Canon, Nikon, Fuji) may embed invisible identifying
         patterns in image data. Use goindigo (planned) for full mitigation.
```

**Tags preserved at journalist level:** ExposureTime, FNumber, ISOSpeedRatings, ApertureValue, ExposureBiasValue, MaxApertureValue, Flash, FocalLength, FocalLengthIn35mmFilm, ImageWidth, ImageLength, ColorSpace, DateTimeOriginal.

### XMP padding trick

When modifying XMP content (not excising it), if the cleaned result is shorter than the original, pad with whitespace inside the XML to preserve segment length. This avoids rewriting all subsequent JPEG segment offsets.

### Output behaviour

- Default: write to `_lapis/` subdirectory alongside input. Never modify originals unless `--in-place` is passed.
- If output file already exists, append `_1`, `_2` etc. — never overwrite.
- If input is a directory: process all `.jpg`/`.jpeg` (case-insensitive). With `--recursive`, mirror directory structure inside `_lapis/`.
- Validate output is a parseable JPEG before writing.
- Non-JPEG files: print warning and skip, do not abort batch.
- End summary: `Processed: N  Skipped: N  Errors: N`

### Filename scrambling

- `scramble`: 8-char lowercase hex string (e.g. `a3f9c2d1.jpg`) — use `crypto/rand`
- `uuid`: UUID4 (e.g. `550e8400-e29b-41d4-a716-446655440000.jpg`) — implement directly with `crypto/rand`, 16 random bytes with version bits `0100` in byte 6 and variant bits `10` in byte 8. Do not import a UUID library.

### Timestamp editing

- `--time-random`: random `time.Time` between 2015-01-01 and 2023-12-31 (configurable via `--time-range-start` / `--time-range-end`)
- `--time-shift`: one random offset per batch run (±365 days), same offset applied to all files to preserve relative order
- `--time-now`: `time.Now()`

Apply to both filesystem timestamps (`os.Chtimes` for mtime/atime) and surviving EXIF DateTime fields. Windows creation time: use `syscall.CreateFileW` + `SetFileTime` behind `//go:build windows`. Document honestly that Linux `ctime` cannot be set from userspace.

## CLI flags

```
lapis [options] <file|directory>
  --level       scout | journalist | ghost  (default: journalist)
  --rename      scramble | uuid            (default: none)
  --time        random | shift | now       (default: none)
  --time-range-start  YYYY-MM-DD           (default: 2015-01-01)
  --time-range-end    YYYY-MM-DD           (default: 2023-12-31)
  --in-place    modify files directly
  --recursive   process subdirectories
  --verbose     print per-file actions
  --version     print version and exit
```

## Testing

Tests for `internal/strip` must be table-driven and use synthetic test JPEGs generated in test setup (not real photos). Required cases:
- Valid JPEG with GPS data → GPS tags removed at scout level
- Valid JPEG → APP1 and APP13 fully absent after ghost level
- Non-JPEG file → graceful error, no output written
- JPEG with no metadata → passes through without corruption
- JPEG with embedded thumbnail in IFD1 → thumbnail removed at journalist level

## v1 scope boundary

Do not build in v1: mimic filename mode, `--time-set` flag, RAW/PNG/TIFF support, audit/dry-run mode, actual `indigo` logic, `goindigo`, any third-party dependency, GUI/TUI/interactive prompts.
