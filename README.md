<p align="center">
  <img src="lapis-logo.svg" width="64" height="64" alt="lapis logo">
</p>

> [!IMPORTANT]
> **lapis has moved to GitHub: <https://github.com/elkarrde/lapis>**
>
> This Codeberg repository is no longer updated. New code, issues and releases
> are on GitHub. The module path is now `github.com/elkarrde/lapis`.

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

**`no-gps`** — Strip GPS and location data only. Keep everything else: camera model, lens, shooting data.

**`no-camera`** *(default)* — Strip GPS, device identifiers, serial numbers, owner/artist/copyright fields, software fingerprints, unique image IDs, embedded thumbnail, XMP and IPTC blocks. Keep shooting data: shutter speed, aperture, ISO, focal length, exposure compensation, capture time, dimensions, colour space.

**`clean`** — Strip everything addressable. Drop all non-structural segments: APP1–APP15, COM, and any vendor-specific blocks. Keep only APP0 (JFIF), SOF, DHT, DQT, DRI, and pixel data markers. Rebuilds a clean file structure without re-encoding pixel data. Warns about steganographic fingerprints (see below).

**`reencoded`** *(planned)* — Everything `clean` does, plus reworking the pixel data itself to defeat pixel-level fingerprints (the future `goindigo` engine). Not yet implemented: selecting it (via `--level reencoded` or the `--reencode` shorthand) exits with an error rather than writing a file.

---

## Usage

```
lapis [options] <file|directory>

Options:
  --level       no-gps | no-camera | clean | reencoded   (default: no-camera)
  --reencode    shorthand for --level reencoded (planned; not yet implemented)
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
lapis --level no-gps image.jpg

# Full no-camera strip on a folder
lapis --level no-camera ./photos

# Maximum anonymisation: clean strip, UUID filename, randomised timestamps
lapis --level clean --rename uuid --time random ./photos

# Process subdirectories, mirror structure in _lapis/
lapis --level no-camera --recursive ./archive
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

`--level clean` addresses all addressable metadata. Some camera manufacturers (Canon, Nikon, Fuji) embed near-invisible identifying patterns directly in pixel data. These survive metadata stripping and cannot be removed without re-encoding the image.

`--level reencoded` (planned, the `goindigo` engine, v3) will perform a full JPEG re-encode to mitigate pixel-level fingerprints.

---

## Website

[lapis.elkarrde.codeberg.page](https://lapis.elkarrde.codeberg.page) — source at [codeberg.org/elkarrde/lapis-web](https://codeberg.org/elkarrde/lapis-web) (Hugo)

---

## Companion tools

**`indigo`** *(planned, v2)* — No-options binary. Hardcoded to clean stripping + UUID filename + random timestamps. One command, no decisions. For field use when options are a liability.

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
| **v1** | JPEG stripping levels (`no-gps`, `no-camera`, `clean`), filename scramble + UUID, timestamp random/shift/now, batch directory processing, Linux binary |
| **v1.5** | Filename mimic mode (Sony/Canon/Nikon/Fuji/Olympus schemas), `--time-set`, Windows + macOS binaries |
| **v2** | RAW support (CR2/ARW/NEF/DNG), PNG + TIFF, audit/dry-run mode, `indigo` companion binary |
| **v3** | `--level reencoded` (`goindigo` engine) — full JPEG re-encode, steganography mitigation |

---

## License

lapis is licensed under the **Mozilla Public License 2.0 (MPL-2.0)**. See [LICENSE](LICENSE).

This places no restriction on using the tool: clean as many photos as you like, for any purpose, including commercial. MPL's file-level copyleft only applies if you modify and redistribute lapis's own source files — those modifications must stay open under MPL.
