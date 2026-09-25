# TODO

## Next release — v0.3.0 (proposed, not yet decided)

Suggestion: ship what is already on `main` as a maintenance release, and leave the v1.5
features for later releases. **v0.3.0** rather than v0.2.1, because the module path change
affects `go install` users and a minor-version bump is the usual pre-1.0 signal.

Already on `main` since v0.2.0:
- Module path moved to `github.com/elkarrde/lapis` (`9a9d067`) — the reason for the release
- Linux `ctime` note when `--time` is used (`54c25e4`)

Release steps:
- [ ] Decide: ship now as v0.3.0, or pull in some v1.5 items first (mimic rename, `--time-set`, macOS binary, Windows `.exe` icon)
- [x] `CHANGELOG.md`: `[Unreleased]` covers the module path move and the ctime note
- [ ] Bump `version` in `cmd/lapis/main.go`; turn `[Unreleased]` into `[0.3.0]` with the date and a link
- [ ] Build `dist/lapis-0.3.0` (Linux) and `dist/lapis-0.3.0.exe` (Windows); `GOPROXY=off go test ./...` first
- [ ] Tag `v0.3.0` and publish a GitHub Release with both binaries. This makes `go install github.com/elkarrde/lapis/cmd/lapis@latest` work (`v0.1.0`/`v0.2.0` declare the old `codeberg.org/elkarrde/lapis` module path)
- [ ] lapis-web: bump `[params].version` in `hugo.toml`, rebuild (`rm -rf public && hugo --minify`), user uploads `public/` to `iso3200.org/lapis/`
- [ ] Update `STATUS.md` (both repos)

Codeberg: left as it is for now (pointer README, last synced at `d1bce71`) until a decision is made — push releases to GitHub only.

---

## v1 — MVP

### Setup
- [x] Initialise `go.mod` (Go 1.22+; now `github.com/elkarrde/lapis`, originally `codeberg.org/elkarrde/lapis`)
- [x] Create directory structure: `cmd/lapis/`, `cmd/indigo/`, `internal/strip/`, `internal/rename/`, `internal/timestamp/`
- [x] Add `cmd/indigo/main.go` placeholder

### `internal/strip` — JPEG segment stripping engine
- [x] JPEG segment parser (walk `0xFF` markers, extract type + length + payload)
- [x] EXIF IFD tag reader (IFD0, GPS sub-IFD, Exif sub-IFD) supporting both byte orders
- [x] Segment parser/writer and IFD engine moved to the first-party `exifscalpel` library (vendored); `internal/strip` keeps only the stripping policy
- [x] `no-gps` level: remove GPS IFD pointer from APP1
- [x] `no-gps` level: remove location from APP13 IPTC **and** XMP. Built `exifscalpel/iptc` package + opt-in `xmp.CleanLocation` (incl. structured `Iptc4xmpExt`, handled by value-blanking at every nesting depth), shipped as exifscalpel v0.2.0, re-vendored, and wired the lapis `no-gps` policy (strip the 7 record-2 location datasets; blank XMP location/GPS, keeping the XMP block). See [`docs/IPTC-XMP-LOCATION-SCOPE.md`](docs/IPTC-XMP-LOCATION-SCOPE.md).
- [x] `no-camera` level: excise APP13 entirely; rebuild APP1 retaining only shooting data tags; remove XMP APP1 segments; remove IFD1 (embedded thumbnail)
- [x] `clean` level: drop ALL non-structural segments (APP1–APP15, COM `0xFFFE`, vendor); keep only APP0, SOF, DHT, DQT, DRI, SOS/SOI/EOI/RST; print steganography warning to stderr
- [x] `reencoded` placeholder: `--level reencoded` / `--reencode` accepted, `Strip` returns `ErrReencodeNotImplemented` (the actual re-encode is v3, see below)
- [x] Output validation: verify result is a parseable JPEG before writing
- [x] Write to `_lapis/` subdirectory by default; `--in-place` flag to modify originals
- [x] Collision avoidance: append `_1`, `_2` etc. if output filename already exists

### `internal/rename` — filename scrambling
- [x] `scramble` mode: 8-char lowercase hex string via `crypto/rand`
- [x] `uuid` mode: UUID4 via `crypto/rand` (implemented directly, no library)

### `internal/timestamp` — timestamp editing
- [x] `--time random`: random `time.Time` in configurable range, applied via `os.Chtimes`
- [x] `--time shift`: single random offset per batch run, applied uniformly to preserve ordering
- [x] `--time now`: set to `time.Now()`
- [x] Apply chosen timestamp to surviving EXIF DateTime fields (DateTimeOriginal etc.)
- [x] Windows creation time: `syscall.CreateFile` + `SetFileTime` behind `//go:build windows`
- [x] Document Linux `ctime` limitation in tool output (`ctimeNote`: printed once to stderr when `--time` is used on Linux)

### `cmd/lapis/main.go` — CLI
- [x] Flag definitions: `--level`, `--rename`, `--time`, `--time-range-start`, `--time-range-end`, `--in-place`, `--recursive`, `--verbose`, `--version`, `--reencode`
- [x] Single file processing path
- [x] Directory batch processing (`.jpg`/`.jpeg`, case-insensitive)
- [x] `--recursive` with mirrored directory structure inside `_lapis/`
- [x] Per-file error handling: warn and skip non-JPEG files, do not abort batch
- [x] End-of-run summary: `Processed: N  Skipped: N  Errors: N`
- [x] Flags and paths in any order (`interleave`)
- [x] Windows `/flag` style accepted on every platform, path-safe (`normaliseArgs`)
- [x] Exit code `1` when any file errored (scriptable)
- [x] Windows double-click: pause for Enter before exiting
- [x] Fix: single-file default output (`lapis photo.jpg` wrote nothing) and `--in-place` actually overwriting (atomic temp-file write) — v0.2.0

### Tests
- [x] Valid JPEG with GPS data → GPS tags absent after `no-gps`
- [x] Valid JPEG → only structural segments remain after `clean` (APP1–APP15, COM all absent)
- [x] Non-JPEG file → graceful error, no output written
- [x] JPEG with no metadata → passes through without corruption
- [x] JPEG with embedded IFD1 thumbnail → thumbnail absent after `no-camera`
- [x] `reencoded` level → `Strip` returns `ErrReencodeNotImplemented`, no output written
- [x] Synthetic test JPEG generator (used by test setup, not real photos)
- [x] `cmd/lapis`: argument handling + end-to-end output path (single file, `--in-place`, `--in-place --rename`)
- [x] `internal/timestamp`: mtime application + mode parsing
- [x] `internal/rename`: scramble / UUID4 format

---

## v1.5

- [ ] `--rename mimic`: camera-style filename schemas (Sony, Canon, Nikon, Fuji, Olympus/OM) with randomised numbers
- [ ] `--mimic-style [brand]`: force a specific brand schema
- [ ] `rotate-file` / `rotate-folder` rotation modes for mimic
- [ ] `--time-set YYYY-MM-DD`: set to a specific date
- [x] Windows binary (shipped since v0.1.0)
- [ ] Windows `.exe` icon + version info: generate `cmd/lapis/rsrc_windows_amd64.syso` (auto-linked by Go, Windows-only; commit it so offline builds keep working) with [go-winres](https://github.com/tc-hib/go-winres) (0BSD, dev-time only: `go run github.com/tc-hib/go-winres@v0.3.3 …`). Icon: rounded-square `logo-bg.svg` from `lapis-web/static/img/` (the plain outline is too faint at 16px), rendered with Inkscape at 16/32/48/256. Regenerate the `.syso` each release so the Properties version matches. Linux ELF has no embedded icon — nothing to do there.
- [ ] macOS binary
- [ ] Windows right-click / Send to integration (dual-use exe) and an in-browser WASM version — planned, not scheduled; see `FUTURE.md`
- [ ] Check what survives in the opaque tail: `jpeg.Parse` stops at the first SOS and `Strip` writes everything after it verbatim at every level, `clean` included — data after EOI (Google Motion Photo MP4, Samsung trailer, …) and marker segments between progressive scans. Test real phone files; if any carry identifying data, drop it (at least at `clean`). See `FUTURE.md` §"Observation to follow up"

---

## v2

- [ ] RAW format support: CR2, ARW, NEF, DNG
- [ ] PNG support
- [ ] TIFF support
- [ ] Audit / dry-run mode: report what would be stripped without writing output
- [ ] `indigo` companion binary: no-options, hardcoded clean + UUID + time-random

---

## v3

- [ ] `goindigo` (`--level reencoded`): full JPEG re-encode to scrub pixel-level steganographic fingerprints
- [ ] Quality option for re-encode
- [ ] Lossless option for re-encode
