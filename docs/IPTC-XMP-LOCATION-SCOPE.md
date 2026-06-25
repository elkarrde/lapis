# no-gps location stripping — scope & decisions

*Status: scoped, not yet implemented. Last updated 2026-06-25.*

Closes the last v1 gap: `no-gps` removes the EXIF GPS IFD but still leaves
location data in **IPTC (APP13)** and **XMP (APP1)**. This note records the agreed
scope and the design decisions so the work can resume cold.

## Decisions taken

| Decision | Choice | Why |
|---|---|---|
| Where the IPTC parser lives | **`exifscalpel/iptc` (new package), built now** | exifscalpel HANDOFF §"IPTC (APP13)" already reserves a future `iptc/` package; keeps the family split (exifscalpel = primitives, lapis = policy), same as `jpeg`/`exif`/`xmp`. |
| Also strip XMP location | **Yes** | `no-gps` keeps XMP today (only `no-camera` strips it), so location/coords leak via XMP unless handled. |
| XMP API shape | **New `xmp.CleanLocation` function** (separate from existing `Clean`) | The existing `xmp.Clean` targets only Adobe-signature fields and is consumed by **tidy-exif**; folding location into the default `Fields` would silently change tidy-exif's behavior. Keep it opt-in via a distinct entry point. |
| XMP field scope | **Everything, incl. structured `Iptc4xmpExt`** | Most thorough; accepts the extra `rdf:Bag`-of-structs parsing work and risk. |

## Background facts (verified 2026-06-24/25)

- **lapis today:** `no-camera` excises APP13 wholesale (`strip.go:113`, correct);
  `no-gps` passes APP13 through untouched (`strip.go:137` TODO). No IPTC/IRB parser
  exists in any of the three repos (lapis / exifscalpel / tidy-exif).
- **exifscalpel today:** packages `jpeg`, `exif`, `xmp` only. `xmp.Fields` targets
  **Adobe-signature fields only** (`CreatorTool`, `MetadataDate`, `DocumentID`,
  history `softwareAgent`) — **no location awareness**. `xmp.Clean(payload,
  replacements)` is length-preserving (in-place value rewrite + padding adjust).
  exifscalpel invariants: primitives only; **stdlib only at runtime, forever**;
  differential tests live in the separate `conformance/` module.
- **Cross-consumer constraint:** tidy-exif imports `xmp` for Adobe cleanup — do not
  change the default `Clean` behavior.

## What has to be parsed (IPTC / APP13)

APP13 is a nested container; `no-gps` needs **surgical** editing (keep segment,
drop only location). All big-endian (no byte-order ambiguity).

1. APP13 payload starts with `Photoshop 3.0\0`, then a sequence of **Image
   Resource Blocks (IRBs)**: `8BIM` + resourceID(2) + Pascal name(padded even) +
   size(4) + data(padded even).
2. The **IPTC-IIM** block is the IRB with resource ID **`0x0404`**. Every other
   IRB (thumbnail `0x040C`, ICC, embedded EXIF, …) is preserved verbatim.
3. Inside `0x0404`: IIM datasets — `0x1C` + record(1) + dataset(1) + length(2,
   high-bit ⇒ extended) + data.
4. Drop the record-2 location datasets, re-serialize, recompute `0x0404` size +
   even-padding, reassemble APP13.

### IIM record-2 location datasets to strip

| Dataset | Field |
|---|---|
| 2:26 | Content Location Code (repeatable) |
| 2:27 | Content Location Name (repeatable) |
| 2:90 | City |
| 2:92 | Sub-location |
| 2:95 | Province/State |
| 2:100 | Country/Primary Location Code |
| 2:101 | Country/Primary Location Name |

Everything else (By-line, Caption, Keywords, Object Name, …) is kept.

## XMP location/GPS fields to strip (via new `xmp.CleanLocation`)

- **GPS coordinates (priority — the real leak):** `exif:GPSLatitude`,
  `exif:GPSLongitude`, `exif:GPSAltitude` (+ `GPSAltitudeRef` if present).
- **Flat place-names:** `photoshop:City`, `photoshop:State`, `photoshop:Country`;
  `Iptc4xmpCore:Location`, `Iptc4xmpCore:CountryCode`.
- **Structured (chosen scope):** `Iptc4xmpExt:LocationCreated` and
  `Iptc4xmpExt:LocationShown` — each an `rdf:Bag` of location structures
  (City / ProvinceState / CountryName / CountryCode / Sublocation /
  WorldRegion / lat-long). Requires container-aware removal, not just flat
  value patching — the hardest piece; budget accordingly.

## Edge cases / risks

- Non-Photoshop APP13 (no `Photoshop 3.0\0` prefix) → leave untouched.
- IIM empty after removal → drop the `0x0404` IRB; if APP13 then has no IRBs →
  drop the segment.
- Repeatable datasets (26/27) → remove all occurrences.
- Extended IIM lengths (length high-bit set) → handle or bail safely, never corrupt.
- Multiple / >64KB APP13 segments (Photoshop can split) → v1 handles each segment
  independently; spanning is a known limitation to document.
- lapis output validation re-parses JPEG structure, **not** IPTC/XMP internals — so
  a bad IIM/XMP rebuild won't be caught there. Needs dedicated round-trip tests.
- `Iptc4xmpExt` structured removal must keep the surrounding `rdf:Description`
  well-formed; xmp editing is length-preserving (value blanking + padding), so
  removing whole nested structures needs care to stay valid and re-parseable.

## Planned components & sequence

1. **exifscalpel `iptc` package** — APP13 IRB + IIM parse/strip/rebuild +
   round-trip tests + `conformance/` cross-check. Primitive only, stdlib only.
2. **exifscalpel `xmp` extension** — new opt-in `CleanLocation` (+ a location
   `Fields`/parse), covering the GPS, flat, and `Iptc4xmpExt` structured fields;
   existing `Clean` / tidy-exif behavior unchanged.
3. **Tag exifscalpel v0.2.0.**
4. **lapis:** re-vendor v0.2.0 (`go mod vendor`, commit).
5. **lapis `no-gps` policy:** replace the `0xED` pass-through with an `iptc`-strip
   call; add an XMP branch that runs `xmp.CleanLocation` (today `no-gps` leaves XMP
   untouched).
6. **Tests + docs** (CLAUDE.md no-gps row, TODO, STATUS).

Each exifscalpel piece lands with its tests before lapis consumes it.

## Rough effort

IPTC parser ~200–250 LOC + tests; XMP flat fields modest; **`Iptc4xmpExt`
structured removal is the main unknown**. Plus cross-repo overhead: v0.2.0 bump +
re-vendor. Two repos get commits.
