# exifscalpel — heads-up for lapis sessions

A shared Go library, **exifscalpel** (`codeberg.org/elkarrde/exifscalpel`, MPL-2.0,
Go 1.22), holds the JPEG-metadata primitives that used to be duplicated between lapis
and its sibling **tidy-exif**. Status: **adopted in lapis as of v0.1.0** (commit
`4cdb506`). lapis imports `exifscalpel/jpeg` and `exifscalpel/exif`; the dependency is
**vendored** (`vendor/` committed) so lapis builds offline from the repo alone. Full
build plan: `../exifscalpel/exifscalpel-HANDOFF.md`.

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

## What's done / what's next

**Migration is done.** The `jpeg` and `exif` packages moved upstream; `internal/strip`
was trimmed to the policy layer and now imports them. The dependency decision above is
settled: **vendored**, builds verified offline (`GOPROXY=off go build ./...`).

Remaining lapis v1 work is unaffected by exifscalpel — finish the `clean` level
(tighten to structural-only segments), output validation, etc. (see `TODO.md`). When
exifscalpel publishes a new version: bump `go.mod`, then `go mod vendor` and commit.
