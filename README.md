<p align="center">
  <img src="lapis-logo.svg" width="64" height="64" alt="lapis logo">
</p>

# Lapis

CLI tool for stripping identifying metadata from photos and image files.

Built for people who need to share images without exposing their location, equipment, or identity — journalists working in hostile environments, documentary scouts, fixers, activists. Most tools strip everything or nothing. Lapis lets you choose your paranoia level.

---

## The problem

Photos carry more identifying information than most people realise:

- **GPS coordinates** — where you were when you took the shot
- **Device identifiers** — camera serial numbers, lens serial numbers
- **Timestamps** — exact date and time, sometimes timezone
- **Software fingerprints** — what app processed or edited the image
- **Embedded thumbnails** — a separate JPEG inside the file with its own EXIF
- **XMP/IPTC metadata** — additional layers beyond standard EXIF

Sharing a photo without stripping this data can expose your location, your equipment, your identity, and your timeline.

---

## Stripping levels

**`scout`** — Strip GPS and location data only. Keep everything else: camera model, lens, shooting data.

**`journalist`** — Strip GPS, device identifiers, serial numbers, owner/artist/copyright fields, software fingerprints, unique image IDs, embedded thumbnail, XMP and IPTC blocks. Keep shooting data: shutter speed, aperture, ISO, focal length, exposure compensation, capture time, dimensions, colour space.

**`ghost`** — Strip everything. Excise EXIF and IPTC segments entirely, rebuild a clean file structure without re-encoding pixel data. Warns about steganographic fingerprints (see below).

---

## Usage

```
lapis [options] <file|directory>

Options:
  --level       scout | journalist | ghost   (default: journalist)
  --rename      scramble | uuid             (default: none, keep original filename)
  --time        random | shift | now        (default: none, keep original timestamps)
  --time-range-start  YYYY-MM-DD            (default: 2015-01-01)
  --time-range-end    YYYY-MM-DD            (default: 2023-12-31)
  --in-place    modify files directly instead of writing to _lapis/ subdirectory
  --recursive   process subdirectories
  --verbose     print per-file actions
  --version     print version and exit
```

By default, processed files are written to a `_lapis/` subdirectory alongside the input. Originals are never modified unless `--in-place` is explicitly passed.

### Examples

```sh
# Strip GPS only, keep original filename
lapis --level scout image.jpg

# Full journalist strip on a folder
lapis --level journalist ./photos

# Maximum anonymisation: ghost strip, UUID filename, randomised timestamps
lapis --level ghost --rename uuid --time random ./photos

# Process subdirectories, mirror structure in _lapis/
lapis --level journalist --recursive ./archive
```

---

## Filename scrambling

`--rename scramble` — Random 8-character hex string: `a3f9c2d1.jpg`

`--rename uuid` — Full UUID4: `550e8400-e29b-41d4-a716-446655440000.jpg`

---

## Timestamp editing

`--time random` — Random date within a configurable range (default 2015–2023). Applied to both filesystem timestamps and any EXIF DateTime fields that survive stripping.

`--time shift` — Random offset applied uniformly across a batch, preserving relative ordering between files.

`--time now` — Set to current time.

**Note on Linux:** `ctime` (inode change time) cannot be set from userspace and will reflect the time of processing. This is documented in tool output and is a kernel limitation, not a bug.

---

## A note on steganographic fingerprints

`--level ghost` addresses all addressable metadata. Some camera manufacturers (Canon, Nikon, Fuji) embed near-invisible identifying patterns directly in pixel data. These survive metadata stripping and cannot be removed without re-encoding the image.

`goindigo` (planned, v3) will perform a full JPEG re-encode to mitigate pixel-level fingerprints.

---

## Website

[lapis.elkarrde.codeberg.page](https://lapis.elkarrde.codeberg.page) — source at [codeberg.org/elkarrde/lapis-web](https://codeberg.org/elkarrde/lapis-web) (Hugo)

---

## Companion tools

**`indigo`** *(planned, v2)* — No-options binary. Hardcoded to ghost stripping + UUID filename + random timestamps. One command, no decisions. For field use when options are a liability.

**`goindigo`** *(planned, v3)* — Full JPEG re-encode to scrub pixel-level steganographic fingerprints that survive metadata stripping.

---

## Build

```sh
go build -o lapis ./cmd/lapis
```

Requires Go 1.22+. No external dependencies.

Cross-compilation:
```sh
GOOS=windows GOARCH=amd64 go build -o lapis.exe ./cmd/lapis
GOOS=darwin  GOARCH=arm64 go build -o lapis-mac ./cmd/lapis
```

---

## Roadmap

| Version | Scope |
|---------|-------|
| **v1** | JPEG stripping levels 1–3, filename scramble + UUID, timestamp random/shift/now, batch directory processing, Linux binary |
| **v1.5** | Filename mimic mode (Sony/Canon/Nikon/Fuji/Olympus schemas), `--time-set`, Windows + macOS binaries |
| **v2** | RAW support (CR2/ARW/NEF/DNG), PNG + TIFF, audit/dry-run mode, `indigo` companion binary |
| **v3** | `goindigo` — full JPEG re-encode, steganography mitigation |

---

## License

See [LICENSE](LICENSE).
