# TODO

## v1 — MVP

### Setup
- [ ] Initialise `go.mod` (`codeberg.org/elkarrde/lapis`, Go 1.22+)
- [ ] Create directory structure: `cmd/lapis/`, `cmd/indigo/`, `internal/strip/`, `internal/rename/`, `internal/timestamp/`
- [ ] Add `cmd/indigo/main.go` placeholder

### `internal/strip` — JPEG segment stripping engine
- [ ] JPEG segment parser (walk `0xFF` markers, extract type + length + payload)
- [ ] EXIF IFD tag reader (IFD0, IFD1, GPS sub-IFD, Exif sub-IFD) supporting both byte orders
- [ ] `scout` level: remove GPS IFD tags from APP1, remove IPTC location fields from APP13
- [ ] `journalist` level: excise APP13 entirely; rebuild APP1 retaining only shooting data tags; remove XMP APP1 segments; remove IFD1 (embedded thumbnail)
- [ ] `ghost` level: excise all APP1 and APP13 segments; print steganography warning to stderr
- [ ] Output validation: verify result is a parseable JPEG before writing
- [ ] Write to `_lapis/` subdirectory by default; `--in-place` flag to modify originals
- [ ] Collision avoidance: append `_1`, `_2` etc. if output filename already exists

### `internal/rename` — filename scrambling
- [ ] `scramble` mode: 8-char lowercase hex string via `crypto/rand`
- [ ] `uuid` mode: UUID4 via `crypto/rand` (implement directly, no library)

### `internal/timestamp` — timestamp editing
- [ ] `--time random`: random `time.Time` in configurable range, applied via `os.Chtimes`
- [ ] `--time shift`: single random offset per batch run, applied uniformly to preserve ordering
- [ ] `--time now`: set to `time.Now()`
- [ ] Apply chosen timestamp to surviving EXIF DateTime fields (DateTimeOriginal etc.) as well as filesystem timestamps
- [ ] Windows creation time: `syscall.CreateFileW` + `SetFileTime` behind `//go:build windows`
- [ ] Document Linux `ctime` limitation in tool output

### `cmd/lapis/main.go` — CLI
- [ ] Flag definitions: `--level`, `--rename`, `--time`, `--time-range-start`, `--time-range-end`, `--in-place`, `--recursive`, `--verbose`, `--version`
- [ ] Single file processing path
- [ ] Directory batch processing (`.jpg`/`.jpeg`, case-insensitive)
- [ ] `--recursive` with mirrored directory structure inside `_lapis/`
- [ ] Per-file error handling: warn and skip non-JPEG files, do not abort batch
- [ ] End-of-run summary: `Processed: N  Skipped: N  Errors: N`

### Tests (`internal/strip`)
- [ ] Valid JPEG with GPS data → GPS tags absent after `scout`
- [ ] Valid JPEG → APP1 and APP13 fully absent after `ghost`
- [ ] Non-JPEG file → graceful error, no output written
- [ ] JPEG with no metadata → passes through without corruption
- [ ] JPEG with embedded IFD1 thumbnail → thumbnail absent after `journalist`
- [ ] Synthetic test JPEG generator (used by test setup, not real photos)

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
