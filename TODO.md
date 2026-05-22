# TODO

## v1 — MVP

### Setup
- [x] Initialise `go.mod` (`codeberg.org/elkarrde/lapis`, Go 1.22+)
- [x] Create directory structure: `cmd/lapis/`, `cmd/indigo/`, `internal/strip/`, `internal/rename/`, `internal/timestamp/`
- [x] Add `cmd/indigo/main.go` placeholder

### `internal/strip` — JPEG segment stripping engine
- [x] JPEG segment parser (walk `0xFF` markers, extract type + length + payload)
- [x] EXIF IFD tag reader (IFD0, GPS sub-IFD, Exif sub-IFD) supporting both byte orders
- [x] `scout` level: remove GPS IFD pointer from APP1
- [ ] `scout` level: remove IPTC location fields from APP13 (requires IPTC parser — deferred)
- [x] `journalist` level: excise APP13 entirely; rebuild APP1 retaining only shooting data tags; remove XMP APP1 segments; remove IFD1 (embedded thumbnail)
- [ ] `ghost` level: drop ALL non-structural segments (APP1–APP15, COM `0xFFFE`, vendor); keep only APP0, SOF, DHT, DQT, DRI, SOS/SOI/EOI/RST; print steganography warning to stderr
- [ ] Output validation: verify result is a parseable JPEG before writing
- [x] Write to `_lapis/` subdirectory by default; `--in-place` flag to modify originals
- [x] Collision avoidance: append `_1`, `_2` etc. if output filename already exists

### `internal/rename` — filename scrambling
- [x] `scramble` mode: 8-char lowercase hex string via `crypto/rand`
- [x] `uuid` mode: UUID4 via `crypto/rand` (implemented directly, no library)

### `internal/timestamp` — timestamp editing
- [x] `--time random`: random `time.Time` in configurable range, applied via `os.Chtimes`
- [x] `--time shift`: single random offset per batch run, applied uniformly to preserve ordering
- [x] `--time now`: set to `time.Now()`
- [ ] Apply chosen timestamp to surviving EXIF DateTime fields (DateTimeOriginal etc.)
- [ ] Windows creation time: `syscall.CreateFileW` + `SetFileTime` behind `//go:build windows`
- [x] Document Linux `ctime` limitation in tool output

### `cmd/lapis/main.go` — CLI
- [x] Flag definitions: `--level`, `--rename`, `--time`, `--time-range-start`, `--time-range-end`, `--in-place`, `--recursive`, `--verbose`, `--version`
- [x] Single file processing path
- [x] Directory batch processing (`.jpg`/`.jpeg`, case-insensitive)
- [x] `--recursive` with mirrored directory structure inside `_lapis/`
- [x] Per-file error handling: warn and skip non-JPEG files, do not abort batch
- [x] End-of-run summary: `Processed: N  Skipped: N  Errors: N`

### Tests (`internal/strip`)
- [x] Valid JPEG with GPS data → GPS tags absent after `scout`
- [ ] Valid JPEG → only structural segments remain after `ghost` (APP1–APP15, COM all absent)
- [x] Non-JPEG file → graceful error, no output written
- [x] JPEG with no metadata → passes through without corruption
- [x] JPEG with embedded IFD1 thumbnail → thumbnail absent after `journalist`
- [x] Synthetic test JPEG generator (used by test setup, not real photos)

---

## v1.5

- [ ] `--rename mimic`: camera-style filename schemas (Sony, Canon, Nikon, Fuji, Olympus/OM) with randomised numbers
- [ ] `--mimic-style [brand]`: force a specific brand schema
- [ ] `rotate-file` / `rotate-folder` rotation modes for mimic
- [ ] `--time-set YYYY-MM-DD`: set to a specific date
- [ ] Windows binary
- [ ] macOS binary

---

## v2

- [ ] RAW format support: CR2, ARW, NEF, DNG
- [ ] PNG support
- [ ] TIFF support
- [ ] Audit / dry-run mode: report what would be stripped without writing output
- [ ] `indigo` companion binary: no-options, hardcoded ghost + UUID + time-random

---

## v3

- [ ] `goindigo`: full JPEG re-encode to scrub pixel-level steganographic fingerprints
- [ ] Quality option for re-encode
- [ ] Lossless option for re-encode
