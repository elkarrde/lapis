# Changelog

All notable changes to **lapis** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- On Linux, `--time` now prints a one-time note to stderr: the inode change
  time (`ctime`) cannot be set from userspace and will show when lapis ran.

### Changed
- Project moved to GitHub: the module path is now `github.com/elkarrde/lapis`
  (was `codeberg.org/elkarrde/lapis`). The `exifscalpel` dependency stays on
  Codeberg at `codeberg.org/elkarrde/exifscalpel`.

## [0.2.0] - 2026-07-02

### Added
- `no-gps` now strips location beyond the EXIF GPS IFD: IPTC-IIM location
  datasets in APP13 (Content Location Code/Name, City, Sub-location,
  Province/State, Country code/name) and location/GPS fields in XMP
  (`exif:GPS*`, `photoshop:City/State/Country`, `Iptc4xmpCore:*`, and the
  structured `Iptc4xmpExt:LocationCreated`/`LocationShown`). The XMP block is
  kept and only its location values are blanked; an APP13 that carries only
  location data is dropped entirely.

### Changed
- Metadata engine now comes from the first-party, pure-Go library **exifscalpel**
  (`codeberg.org/elkarrde/exifscalpel` v0.2.0, MPL-2.0): its `jpeg`/`exif`
  packages hold the segment parser/writer and TIFF/IFD engine, and its `iptc`
  package + `xmp.CleanLocation` power the new `no-gps` location stripping. lapis
  keeps only the stripping policy. The library statically links into the same
  one-file binary and is **vendored** (`vendor/` committed), so builds stay
  offline and the shipped executable stays self-contained — the "no run-time
  dependencies" rule is now judged by that binary, not by a zero-`require`
  `go.mod`.

### Fixed
- `lapis <file>` (a single file, default output) no longer fails trying to
  create `<file>/_lapis/`; the `_lapis/` directory is now created beside the
  file. Broken since the first release.
- `--in-place` now overwrites the original instead of writing a `<name>_1` copy
  beside it. The write is atomic (temp file + rename); with `--rename` it is a
  move. Broken since the first release.

## [0.1.0] - 2026-05-22

### Added
- `internal/strip` JPEG segment stripping engine: raw-byte segment parser/writer,
  EXIF IFD parser/serializer, and the stripping levels (`no-gps`, `no-camera`, `clean`).
- New `reencoded` stripping level (and `--reencode` shorthand) reserved for future
  pixel-level rework (the `goindigo` engine). Not yet implemented: `Strip` returns
  `ErrReencodeNotImplemented` and the CLI exits non-zero before writing any file.
- `internal/rename` filename scrambling (`scramble`, `uuid`) using `crypto/rand`.
- `internal/timestamp` filesystem timestamp editing (`random`, `shift`, `now`) via `os.Chtimes`.
- `cmd/lapis` CLI entry point with flag parsing and batch/recursive orchestration.
- `cmd/indigo` placeholder binary (prints "not yet implemented").
- White-box test suites for `strip` and `rename` using synthetic JPEG fixtures.
- Project logo (`lapis-logo.svg`) and `dist/` packaging folder.
- Sublime Text project configuration.

### Changed
- Renamed the stripping levels to be less role-specific: `scout` → `no-gps`,
  `journalist` → `no-camera` (still the default), `ghost` → `clean`. The old names
  are no longer accepted by `--level`.
- Relicensed the project to **MPL-2.0**; added SPDX headers across sources.
- Updated motto and project documentation (README, CLAUDE.md, STATUS.md, EXIFSCALPEL.md)
  to reflect the implemented v1 core.

### Notes
- First public release (2026-05-22): Linux and Windows binaries on Codeberg.
- The shipped binary is a single self-contained executable with no run-time
  dependencies; at this release `go.mod` had zero `require` entries.

[0.2.0]: https://github.com/elkarrde/lapis/releases/tag/v0.2.0
[0.1.0]: https://github.com/elkarrde/lapis/releases/tag/v0.1.0
