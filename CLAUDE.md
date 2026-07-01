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

`codeberg.org/elkarrde/lapis` — Go 1.22+. The binding rule is **no run-time dependencies**: a built `lapis` must be a single self-contained executable that needs nothing but itself (no installs, shared libs, or external tools). This is about the *shipped binary*, not the dependency list — lapis now has one `require` (the first-party, pure-Go `codeberg.org/elkarrde/exifscalpel`), which statically links into the same one file, so the run-time goal still holds. (The old "zero `require` entries" proxy no longer applies; judge by the binary, not by `go.mod`.)

**Builds are vendored.** `vendor/` is committed, so `go build` / `go test` resolve offline from the repo alone — no module proxy, no sibling checkout. Verify self-containment with `GOPROXY=off go build ./...`. After bumping the exifscalpel version in `go.mod`, re-run `go mod vendor` and commit the result.

What the rule covers, by phase:

- **Run time (what users get) — the only hard constraint.** The shipped binary stays one file with no external deps, and any code statically linked into it must carry a license that does not restrict a user's use case.
- **Build / development — fair game, and we take the pragmatic path.** Build- and dev-time tooling is fine provided its license doesn't restrict *our* development or distribution. We use what's best for the job rather than re-implementing it (a pure-Go module that statically links into the same one-file binary — `exifscalpel` — preserves the run-time goal even though it adds a `require`). The portability we care about is the *compiled* `.exe`/Linux binary the user runs, not asceticism at build time.
- **Testing / cross-checking — unrestricted.** Comparing lapis output against other tools or libraries (exiftool, goexif, etc.) is fine: we're verifying, not reusing or redistributing their code, so their licenses don't bind us.

> **Heads-up (see [`EXIFSCALPEL.md`](EXIFSCALPEL.md)):** the shared first-party library `exifscalpel` (`codeberg.org/elkarrde/exifscalpel`, MPL-2.0, pure Go) is now **adopted** — its `jpeg` and `exif` packages hold the segment parser/writer and TIFF/IFD engine that used to live in `internal/strip`. lapis keeps only the stripping *policy* (`Level` / `Strip` / `processSegments`) on top. It statically links into the same one-file binary, so the **single self-contained executable** goal is intact; the dependency is **vendored** (`vendor/` committed) so builds stay offline and self-contained.
>
> **License:** lapis is **MPL-2.0** (see [`LICENSE`](LICENSE); every `.go` file carries the `SPDX-License-Identifier: MPL-2.0` + Exhibit A header), and so is exifscalpel — so the whole metadata engine sits under one consistent license. MPL is *file-level* copyleft: its reciprocity covers each MPL source file (modifications must stay open under MPL), not a user's photos, their use of the tool, or proprietary files someone might add alongside. This satisfies the run-time rule above (a user's use case is never restricted) and the build/development rule (MPL doesn't restrict our development or distribution). Because lapis and exifscalpel share the license, the packages lapis contributed upstream (`jpeg`, `exif`) moved between repos without any relicensing friction.

## Architecture

```
cmd/lapis/main.go       ← CLI entry point, flag parsing, orchestration (flag stdlib only)
cmd/indigo/main.go      ← placeholder only, prints "not yet implemented"
internal/strip/         ← JPEG stripping policy (segment/EXIF engine lives in exifscalpel)
  strip.go              ← public Strip() API, level dispatch, per-level segment policy
                          (incl. no-gps IPTC/XMP location stripping via exifscalpel/iptc + xmp.CleanLocation)
  exif.go               ← per-level EXIF filtering (parse via exifscalpel/exif, drop tags, rebuild)
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

**Do not link `goexif` or any third-party EXIF library into the binary** — they are read-only and would add a run-time dependency. The shipped engine uses raw byte operations only. (Using such a library in *tests* to cross-check output is fine — see the Module section.)

### Stripping levels

| Level | Constant | Action |
|-------|----------|--------|
| `no-gps` | `LevelNoGPS` | Parse APP1, remove GPS IFD pointer (0x8825) from IFD0. Strip IPTC location datasets from APP13 (via `exifscalpel/iptc`) and blank location/GPS fields in XMP (via `xmp.CleanLocation`); XMP is kept, only its location values are cleared. |
| `no-camera` | `LevelNoCamera` | Excise APP13 entirely. Rebuild APP1 keeping only shooting data tags. Remove all XMP APP1 segments. IFD1 (thumbnail) is never emitted. |
| `clean` | `LevelClean` | Drop all non-structural segments: keep only APP0 (`0xFFE0`), SOF (`0xFFC0`–`0xFFCF`, excluding `0xFFC4`/`0xFFC8`), DHT (`0xFFC4`), DQT (`0xFFDB`), DRI (`0xFFDD`), SOS/SOI/EOI/RST. Everything else — APP1–APP15, COM (`0xFFFE`), vendor segments — is excised. Print steganography warning to stderr. |
| `reencoded` | `LevelReencoded` | **Planned, not yet implemented.** Intended to do everything `clean` does plus rework pixel data to defeat pixel-level fingerprints (future `goindigo`). Today `Strip` returns `ErrReencodeNotImplemented` and the CLI exits non-zero before touching any file. Selectable via `--level reencoded` or the `--reencode` shorthand. |

Note: `no-camera` (formerly `journalist`) is the **default** level.

Clean level warning (exact text, printed to stderr):
```
WARNING: Pixel-level steganographic fingerprints are not addressed by --level clean.
         Camera manufacturers (Canon, Nikon, Fuji) may embed invisible identifying
         patterns in image data. Use --level reencoded (planned) for full mitigation.
```

**Tags preserved at no-camera level:** ExposureTime, FNumber, ISOSpeedRatings, ApertureValue, ExposureBiasValue, MaxApertureValue, Flash, FocalLength, FocalLengthIn35mmFilm, ImageWidth, ImageLength, ColorSpace, DateTimeOriginal.

### EXIF IFD parser and serializer (`internal/strip/exif.go`)

`parseEXIF` reads all IFD entries and resolves their values from TIFF data offsets into `[]byte`. Sub-IFDs (Exif, GPS) are followed and stored separately. IFD1 (thumbnail) is intentionally skipped at all levels — its entries reference raw thumbnail bytes by offset that cannot be safely relocated without specialized handling.

`buildEXIF` serializes IFD0 + optional Exif sub-IFD + optional GPS sub-IFD with freshly computed offsets. Layout: TIFF header | IFD0 | ExifSub | GPSSub | external value data. Sub-IFD pointer entries (0x8769, 0x8825) in IFD0 must be present as placeholder entries; `buildEXIF` patches their values.

Tags 0xA005 (Interoperability IFD) and 0x014A (SubIFDs) are stripped at no-gps level because their values are TIFF offsets that cannot be rewritten safely during rebuild.

### Output behaviour

- Default: write to `_lapis/` subdirectory alongside input. Never modify originals unless `--in-place` is passed.
- Output validation: `Strip` re-parses its own serialized output before returning a single byte; if the result is not a parseable JPEG it returns an error and writes nothing. The CLI then counts the file as an error and moves on — so a failed strip never produces a corrupt file, and `--in-place` never overwrites an original with bad output.
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

The chosen time is applied to **both** the filesystem (`os.Chtimes`) and any surviving EXIF DateTime field. The CLI picks the time once (`timestamp.Pick`) before stripping and passes it to `Strip` as `exifTime *time.Time`; `setEXIFDateTimes` (in `internal/strip/exif.go`) overwrites only the DateTime fields the level kept — DateTime (`0x0132`), DateTimeOriginal (`0x9003`), DateTimeDigitized (`0x9004`) — and never adds one the source lacked or a level just stripped (so at `no-camera` only DateTimeOriginal is rewritten; at `clean` there is no EXIF to touch). The format is EXIF's ASCII `YYYY:MM:DD HH:MM:SS` + NUL.

Windows creation time is set via `syscall.CreateFile` + `SetFileTime` behind `//go:build windows` (`creationtime_windows.go`); `creationtime_other.go` (`//go:build !windows`) is a no-op so `Apply` is one cross-platform call. Standard `syscall` only — no run-time dependency. Verify the Windows path compiles with `GOOS=windows go build ./...` / `go vet`.

## CLI flags

```
lapis [options] <file|directory>
  --level             no-gps | no-camera | clean | reencoded  (default: no-camera)
  --reencode          shorthand for --level reencoded (planned; not yet implemented)
  --rename            scramble | uuid            (default: none)
  --time              random | shift | now       (default: none)
  --time-range-start  YYYY-MM-DD                 (default: 2015-01-01)
  --time-range-end    YYYY-MM-DD                 (default: 2023-12-31)
  --in-place          modify files directly
  --recursive         process subdirectories
  --verbose           print per-file actions
  --version           print version and exit
```

### Argument conventions (`cmd/lapis/main.go`)

- **Flag styles:** `--flag`, `-flag`, `--flag=value`, `-flag value` all work (Go `flag`). Windows `/flag` and `/flag=value` are also accepted on every platform — `normaliseArgs` rewrites them to `--flag`, but only when the name matches a defined flag, so positional paths like `/photos` or `/usr/local/pic.jpg` are left alone (a tidy-exif family convention, made path-safe because lapis takes positional paths).
- **Order-independent:** flags and the file/dir targets may appear in any order (`lapis photo.jpg --level clean` works). `interleave` drives this — Go's `flag` otherwise stops at the first non-flag token.
- **Exit codes:** `0` on success, `1` when any file errored (so the tool is scriptable), `2` for an unknown/malformed flag (from `flag`).
- **Windows double-click:** every exit goes through `die`, which calls `waitIfWindows` to pause for Enter on Windows so a double-clicked `.exe` doesn't vanish before its output is read.

## Testing

Tests live in `internal/strip/strip_test.go` (white-box, `package strip`), `internal/rename/rename_test.go` (white-box, `package rename`), and `internal/timestamp/timestamp_test.go` (white-box, `package timestamp`). Synthetic JPEG fixtures are built from scratch using a `jpegBuilder` helper — no real photos.

Required strip test cases (all passing):
- Valid JPEG with GPS → GPS tags absent after `no-gps`
- Valid JPEG → APP1 and APP13 absent after `clean`
- Non-JPEG file → graceful error, no output written
- JPEG with no metadata → passes through without corruption
- JPEG with IFD1 thumbnail → thumbnail absent after `no-camera`
- `reencoded` level → `Strip` returns `ErrReencodeNotImplemented`, no output written

## v1 scope boundary

Do not build in v1: mimic filename mode, `--time-set` flag, RAW/PNG/TIFF support, audit/dry-run mode, actual `indigo` logic, `goindigo`, any third-party run-time dependency, GUI/TUI/interactive prompts.
