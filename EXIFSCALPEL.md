# exifscalpel — heads-up for lapis sessions

A shared Go library, **exifscalpel** (`codeberg.org/elkarrde/exifscalpel`, MPL-2.0,
Go 1.22), is being built to hold the JPEG-metadata primitives currently duplicated
between lapis and its sibling **tidy-exif**. Status: repo scaffolded, **no Go code
yet** (pre-code). Full build plan: `../exifscalpel/exifscalpel-HANDOFF.md`.

## Why lapis is involved

lapis's `internal/strip` is the **canonical source** for two of exifscalpel's three
planned packages — lapis *contributes upstream*, it isn't just a consumer:

- `jpeg` — the segment parser/writer (`parseJPEG` / `writeJPEG` / `jpegSeg` /
  `isExifSeg` / `isXMPSeg` in `strip.go`)
- `exif` — the TIFF/IFD engine (`parseEXIF` / `buildEXIF` / `ifdEntry` / `parsedEXIF`
  in `exif.go`)

The third package, `xmp` (field-level surgery), comes from tidy-exif. lapis's policy —
`Level` / `Strip` / `processSegments`, plus `internal/timestamp` and `internal/rename`
— stays in lapis.

## The "zero deps" rule — what it actually protects

lapis's "zero external dependencies / pure stdlib" rule exists so that **running
lapis needs nothing but the single executable** — no installs, no shared libraries,
no external tools. A first-party Go module like exifscalpel does **not** break that:
Go statically links imported packages into the binary, so a pure-Go exifscalpel
compiles into the same single static binary. The distributed artifact stays one file,
and the audit surface stays "stdlib + the author's own code" (exifscalpel is yours).

So Phase 6 preserves the real intent. What changes is narrow:

- `go.mod` gains one `require` entry (and a `go.sum`);
- `go build` must resolve exifscalpel — keep builds self-contained/offline by
  **vendoring** (`go mod vendor`) or a **`replace`** directive to the local sibling.

Net: this is a normal build-setup choice (mostly *vendor vs module fetch*), not a
violation of lapis's single-executable goal. The literal "zero `require` entries"
wording in `CLAUDE.md` is a stricter proxy than the goal needs — revisit that wording
if/when lapis adopts exifscalpel.

## What to do now

**Nothing in the code.** exifscalpel has no Go yet. Do **not** start removing
`internal/strip`. Keep building lapis v1 (ghost level, etc.) as planned. The migration
is decoupled — lapis can adopt exifscalpel later, independently of tidy-exif, and only
after the dependency decision above is made.
