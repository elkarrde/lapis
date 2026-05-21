# Project Lapis

## What is it?

A cross-platform CLI tool for stripping identifying metadata from photos and image files. Built for people who need to stay unidentified — journalists working in hostile environments, documentary scouts, fixers, activists — but useful for anyone who values their privacy when sharing images.

Most tools strip everything or nothing. Project Lapis is adjustable: you choose your paranoia level depending on the situation and what you need to preserve.

---

## The problem it solves

Photos carry far more identifying information than most people realise:

- **GPS coordinates** — where you were when you took the shot
- **Device identifiers** — camera serial numbers, lens serial numbers
- **Timestamps** — exact date and time, sometimes timezone
- **Software fingerprints** — what app processed or edited the image
- **Embedded thumbnails** — a separate JPEG inside the file with its own EXIF
- **XMP/IPTC metadata** — additional layers beyond standard EXIF
- **Steganographic patterns** — some manufacturers embed near-invisible fingerprints in the pixel data itself (not addressable by metadata stripping alone — see `goindigo`, planned v3)

Sharing a photo taken on a DSLR, drone, or phone without stripping this data can expose your location, your equipment, your identity, and your timeline.

---

## Adjustable paranoia levels

**Level 1 — `scout`**
Strip GPS and location data only. Keep everything else including camera model, lens, shooting data.

**Level 2 — `journalist`**
Strip GPS, device identifiers, serial numbers, owner/artist/copyright fields, software fingerprints, unique image IDs, embedded thumbnail, XMP and IPTC blocks entirely.
Keep: shutter speed, aperture, ISO, focal length, exposure compensation, original capture time, dimensions, color space.

**Level 3 — `ghost`**
Strip everything. Excise APP1/APP13 JPEG segments entirely, rebuild clean file structure. Does not re-encode pixel data. Warns explicitly about steganographic fingerprints and directs to `goindigo` for full mitigation.

---

## Filename scrambling levels

**Level 1 — `mimic`** *(v1.5)*
Replace with a believable camera-style filename using a built-in schema library:
- Sony: `DSC00000.JPG`
- Canon: `IMG_0000.JPG`
- Nikon: `DSC_0000.JPG`
- Fuji: `DSCF0000.JPG`
- Olympus/OM: `P0000000.JPG`

Numbers are randomized within realistic ranges, not sequential.
Rotation modes: `--mimic-style [brand]` to force one style, `rotate-file` for per-file variation, `rotate-folder` for consistent style per folder.

**Level 2 — `scramble`**
Random alphanumeric string. `a3f9c2d1.jpg`. Unguessable, obviously not a camera filename.

**Level 3 — `uuid`**
Full UUID4. Maximum entropy. Suitable when filename is never exposed to an adversary.

---

## Timestamp editing

Two separate layers:

**EXIF timestamps** (inside the file)
- `DateTimeOriginal`, `DateTimeDigitized`, `DateTime`
- Ghost level: stripped entirely
- Other levels: optionally randomized or shifted

**Filesystem timestamps** (OS level)
- Windows NTFS: creation time, modification time, access time — all three addressable
- Linux: `mtime` and `atime` via `os.Chtimes()`; `ctime` cannot be set directly (documented honestly in tool output)

**Options:**
- `--time-random` — random date within configurable range (default 2015–2023)
- `--time-shift` — random offset preserving relative order within a batch
- `--time-now` — set to current time
- `--time-set [date]` *(v1.5)* — set to a specific date

---

## The companion tools

**`indigo`** *(v2)*
No-options binary. Maximum paranoia, no flags accepted. Hardcoded to: ghost metadata stripping + uuid filename + time-random. One command, done. For journalists in the field who should not have to think about options.

Named after indigo dye — historically the cheaper, darker, plant-based substitute for lapis lazuli pigment. Same logic: blunter, no ceremony.

**`goindigo`** *(v3)*
Full JPEG re-encode to scrub pixel-level steganographic fingerprints that survive metadata stripping. Quality and lossless options. The tool that goes all the way down.

The name: go-indi-go. The Go language pun is accidental and absolutely intentional.

---

## Release roadmap

| Version | Scope |
|---------|-------|
| **v1 — MVP** | JPEG stripping levels 1–3, filename scramble + uuid, timestamp random/shift/now, batch directory processing, Linux binary |
| **v1.5** | Filename mimic mode + camera schema library + rotation, `--time-set`, Windows + macOS binaries |
| **v2** | RAW support (CR2/ARW/NEF/DNG), PNG + TIFF support, audit/dry-run mode, `indigo` companion binary |
| **v3** | `goindigo` — full JPEG re-encode, steganography mitigation |

---

## Technology

- **Language:** Go
- **Dependencies:** none (pure Go, standard library only where possible)
- **Distribution:** single static binary per platform
- **Metadata approach:** direct JPEG segment manipulation (locate and excise APP1/APP13 segments at the binary level) — no external libraries, fully auditable
- **Cross-compilation:** `GOOS=windows GOARCH=amd64 go build` etc., handled natively by Go toolchain

---

## Project structure

```
lapis/
├── cmd/
│   ├── lapis/        ← main CLI tool
│   └── indigo/       ← no-options binary (v2), wraps lapis engine
├── internal/
│   ├── strip/        ← metadata stripping engine
│   ├── rename/       ← filename scrambling
│   └── timestamp/    ← file timestamp spoofing
└── README.md
```

---

## Inspiration

Inspired by [Koloristika](https://koloristika.app) (iOS), which calls a similar feature "stealth" and uses a B-21 bomber silhouette as its icon. Koloristika is iPhone-only and limited to photos taken in the moment. Lapis works on any file from any source, on any platform, offline, with no app store relationship.

---

## The name

**Lapis** comes from lapis lazuli — a deep blue semi-precious stone mined primarily in Afghanistan since antiquity. Ground into pigment, it produced **ultramarine**, the most expensive colour in Renaissance painting. Painters reserved it for the most sacred subjects; Michelangelo left figures unpainted rather than use a cheaper substitute.

The word traces through Medieval Latin *lapis* (stone) and Persian *lāzhward* — meaning **heaven**, or **sky**.

A tool that makes you invisible to the sky. The name arrived accidentally and turned out to be exactly right.

The companion tools `indigo` and `goindigo` continue the pigment lineage — indigo was the plant-based alternative to lapis ultramarine, cheaper and more accessible, darker and more intense.

---

## Status

Planning complete. Handoff to Claude Code prepared. Ready to build.

*Created: May 2026*
