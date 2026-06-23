# Project Lapis — Claude Code Handoff

## What you are building

A CLI tool written in Go that strips identifying metadata from JPEG image files. The tool is called `lapis`. It is designed for journalists, documentary scouts, and anyone who needs to share photos without exposing their identity, location, or equipment.

---

## Local repository paths

| Repo             | Path                                   |
| ---------------- | -------------------------------------- |
| Tool             | `\Code\iso3200\lapis`                  |
| Website          | `\Code\iso3200\lapis-web`              |
| Dev Docs (vault) | `\Dropbox\Code\Dev Docs\Project Lapis` |

---

## Prior art — tidy-exif

There is a related earlier project at `\Code\iso3200\tidy-exif` (Codeberg: `codeberg.org/elkarrde/tidy-exif`). It never progressed beyond planning but contains useful research. Key findings to carry forward:

**XMP segment location and padding trick:**
The XMP block in a JPEG is an APP1 segment (`0xFF 0xE1`) identified by the namespace URI `http://ns.adobe.com/xap/1.0/\x00` at its start (as opposed to EXIF APP1 which starts with `Exif\0\0`). When cleaning XMP content and the result is shorter than the original, **pad with whitespace inside the XML** to preserve segment length. This avoids having to rewrite all subsequent JPEG segment offsets, which is error-prone.

**goexif is read-only:**
`github.com/rwcarlsen/goexif` can only read EXIF, not write it. This confirms that Lapis must do all metadata manipulation via raw byte operations on the JPEG segment structure. Do not attempt to use goexif or any similar library for writes.

**Codeberg handle:** `elkarrde` — use this for the module name.

---

## Primary design constraints

- **Single static binary, no runtime dependencies, no external libraries if avoidable**
- **Pure Go standard library preferred throughout**
- **JPEG segment manipulation at the binary level** — do not use third-party EXIF libraries
- **Cross-platform** — Linux first (v1), Windows and macOS in v1.5
- **The code must be readable and auditable** — this is a security tool, clarity matters more than cleverness

---

## Repository structure

```
lapis/
├── cmd/
│   ├── lapis/
│   │   └── main.go        ← CLI entry point, flag parsing, orchestration
│   └── indigo/
│       └── main.go        ← v2, placeholder only for now
├── internal/
│   ├── strip/
│   │   └── strip.go       ← JPEG segment stripping engine
│   ├── rename/
│   │   └── rename.go      ← filename scrambling
│   └── timestamp/
│       └── timestamp.go   ← filesystem + EXIF timestamp editing
├── go.mod
└── README.md
```

---

## v1 scope — build exactly this, nothing more

### 1. JPEG metadata stripping (`internal/strip`)

Operate directly on JPEG binary segment structure. A JPEG file is a sequence of segments, each beginning with a marker (`0xFF` followed by a type byte). Relevant segments to target:

- `APP1` (`0xFFE1`) — contains EXIF data (and sometimes XMP)
- `APP13` (`0xFFED`) — contains IPTC/Photoshop data
- `APP2` (`0xFFE2`) — ICC profile (keep for now, may revisit)
- All other `APPn` segments (`0xFFE0`–`0xFFEF`) except `APP0` (JFIF marker, keep) — strip on clean level

**Three stripping levels:**

`no-gps` — parse APP1, locate and remove GPS-related EXIF tags only. Tags to remove:
- `0x0001` GPSLatitudeRef
- `0x0002` GPSLatitude
- `0x0003` GPSLongitudeRef
- `0x0004` GPSLongitude
- `0x0005` GPSAltitudeRef
- `0x0006` GPSAltitude
- `0x0007` GPSTimeStamp
- `0x0008` GPSSatellites
- `0x000A` GPSMeasureMode
- `0x000B` GPSDOP
- `0x0010` GPSImgDirectionRef
- `0x0011` GPSImgDirection
- `0x0012` GPSMapDatum
- `0x001D` GPSDateStamp
- `0x001F` GPSHPositioningError
- Also remove IPTC location fields if APP13 is present

`no-camera` — excise APP13 entirely. Parse APP1/EXIF and remove the following tag groups, rebuilding the segment with only shooting data remaining:
- All GPS IFD tags (as above)
- `0x013B` Artist
- `0x8298` Copyright
- `0x0131` Software
- `0x013D` Predictor (processing indicator)
- `0xA004` RelatedSoundFile
- `0xA420` ImageUniqueID
- `0x927C` MakerNote (entire block — proprietary per manufacturer, strip entirely)
- `0xC62F` CameraSerialNumber
- `0xA435` LensSerialNumber
- `0xA430` CameraOwnerName
- `0xA431` BodySerialNumber
- IFD1 entirely (embedded thumbnail lives here)
- XMP APP1 segments (identified by `http://ns.adobe.com/xap/1.0/\x00` header instead of `Exif\0\0`) — excise entirely, do not attempt to parse or preserve

Keep: `0x829A` ExposureTime, `0x829D` FNumber, `0x8827` ISOSpeedRatings, `0x9202` ApertureValue, `0x9204` ExposureBiasValue, `0x9205` MaxApertureValue, `0x9209` Flash, `0x920A` FocalLength, `0xA405` FocalLengthIn35mmFilm, `0x0100` ImageWidth, `0x0101` ImageLength, `0xA001` ColorSpace, `0x9003` DateTimeOriginal

`clean` — excise APP1 and APP13 segments entirely. Rebuild the JPEG by copying all non-APP1/APP13 segments verbatim. Print a warning to stderr:
```
WARNING: Pixel-level steganographic fingerprints are not addressed by --level clean.
         Camera manufacturers (Canon, Nikon, Fuji) may embed invisible identifying
         patterns in image data. Use goindigo (planned) for full mitigation.
```

**All levels must:**
- Operate on a copy by default (never modify in place unless `--in-place` flag is passed)
- Preserve the JPEG SOI (`0xFFD8`) and EOI (`0xFFD9`) markers
- Preserve image pixel data segments (`SOS`, `SOF`, `DHT`, `DQT`) exactly
- Validate that output is a parseable JPEG before writing

### 2. Filename scrambling (`internal/rename`)

Two modes for v1:

`scramble` — generate a random 8-character lowercase hex string. Output: `a3f9c2d1.jpg`. Use `crypto/rand`, not `math/rand`.

`uuid` — generate a UUID4. Output: `550e8400-e29b-41d4-a716-446655440000.jpg`. Implement UUID4 generation using `crypto/rand` directly — do not import a UUID library. It is 16 random bytes with version bits `0100` set in byte 6 and variant bits `10` set in byte 8. Trivial to implement cleanly.

### 3. Timestamp editing (`internal/timestamp`)

**Filesystem timestamps:**
Use `os.Chtimes(path, atime, mtime)` to set access time and modification time.
On Windows, additionally use `syscall` to set creation time (`CreateFileW` + `SetFileTime`). Wrap in a build tag `//go:build windows` so it compiles cleanly on Linux without stubs.
Document in output that `ctime` on Linux cannot be set via userspace and will reflect the time of processing — do not attempt to hide this.

**Modes:**
- `--time-random` — pick a random `time.Time` between `2015-01-01` and `2023-12-31` (configurable via `--time-range-start` and `--time-range-end`, both accept `YYYY-MM-DD`)
- `--time-shift` — generate one random offset duration (between -365 days and +365 days) per batch run; apply the same offset to all files so relative ordering is preserved
- `--time-now` — use `time.Now()`

Apply chosen timestamp to both filesystem mtime/atime AND any EXIF DateTime fields that survive stripping (i.e. on no-gps/no-camera levels where DateTime tags are kept, update them to match).

---

## CLI interface (`cmd/lapis/main.go`)

Use Go's `flag` standard library. No third-party CLI frameworks.

```
lapis [options] <file|directory>

Options:
  --level       no-gps | no-camera | clean  (default: no-camera)
  --rename      scramble | uuid             (default: none, keep original)
  --time        random | shift | now        (default: none, keep original)
  --time-range-start  YYYY-MM-DD            (default: 2015-01-01)
  --time-range-end    YYYY-MM-DD            (default: 2023-12-31)
  --in-place    modify files directly instead of writing to _lapis/ subdirectory
  --recursive   process subdirectories
  --verbose     print per-file actions
  --version     print version and exit
```

**Default output behaviour (without --in-place):**
Create a `_lapis/` subdirectory in the same directory as the input. Write processed files there. Never modify the originals unless `--in-place` is explicitly passed.

**Batch processing:**
If input is a directory, process all `.jpg` and `.jpeg` files (case-insensitive). With `--recursive`, descend into subdirectories and mirror the directory structure inside `_lapis/`.

**Error handling:**
- If a file is not a valid JPEG, print a warning and skip it — do not abort the batch
- If output file already exists, do not overwrite — append `_1`, `_2` etc.
- Print a summary at the end: `Processed: 12  Skipped: 1  Errors: 0`

---

## `cmd/indigo/main.go` — placeholder only for v1

Create the file and directory. Contents:

```go
package main

// indigo is planned for v2.
// It will be a no-options wrapper around the lapis engine with hardcoded
// clean stripping, uuid renaming, and random timestamps.
// No flags. No configuration. Maximum paranoia, minimum ceremony.

func main() {
    println("indigo is not yet implemented. Use lapis --level clean for now.")
}
```

---

## What NOT to build in v1

- No mimic filename mode (v1.5)
- No `--time-set` flag (v1.5)
- No RAW, PNG, or TIFF support (v2)
- No audit/dry-run mode (v2)
- No actual indigo binary logic (v2)
- No goindigo / re-encoding (v3)
- No third-party dependencies whatsoever
- No GUI, no TUI, no interactive prompts

---

## Testing

Write table-driven tests for the strip engine (`internal/strip`). Test cases must include:
- A valid JPEG with GPS data — verify GPS tags removed at no-gps level
- A valid JPEG — verify APP1 and APP13 fully absent after clean level
- A file that is not a JPEG — verify graceful error, no output written
- A JPEG with no metadata — verify it passes through cleanly without corruption
- A JPEG with an embedded thumbnail in IFD1 — verify thumbnail removed at no-camera level

Use small synthetic test JPEGs generated in the test setup, not real photos.

---

## Go version and module

Use Go 1.22 or later. Module name: `codeberg.org/elkarrde/lapis`. `go.mod` must have zero `require` entries. If any are added, that is a mistake.

---

## Definition of done for v1

- `lapis --level clean --rename uuid --time random ./testfolder` processes a folder of JPEGs, writes sanitised copies to `./testfolder/_lapis/`, all with UUID filenames and randomised timestamps
- `lapis --level no-gps image.jpg` removes only GPS data, leaves all other metadata intact
- `lapis --level no-camera image.jpg` produces a file with no identifying tags but intact shooting data
- Binary compiles and runs on Linux with no installed dependencies
- All tests pass
- Clean level prints the steganography warning to stderr

*Handoff prepared: May 2026*
