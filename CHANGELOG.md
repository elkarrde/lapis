# Changelog

All notable changes to **lapis** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- No public release has been tagged yet; the binary reports version `0.1.0`.
- Binding rule: the shipped binary stays a single self-contained executable with
  no run-time dependencies (`go.mod` has zero `require` entries).

[Unreleased]: https://codeberg.org/elkarrde/lapis/commits/branch/main
