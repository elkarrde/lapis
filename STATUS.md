# Status

*Last updated: 2026-09-20*

| Field | Value |
|:--|:--|
| Phase | released (v0.2.0 — no-gps location stripping complete; two CLI output bugs fixed) |
| Version | v0.2.0 |
| Build | passing (`GOPROXY=off go build ./...`; Windows cross-compile OK) |
| Tests | passing (`cmd/lapis`, `internal/strip`, `internal/rename`, `internal/timestamp`) |
| Deployed | v0.2.0 published on [GitHub Releases](https://github.com/elkarrde/lapis/releases/tag/v0.2.0) (originally on Codeberg) (2026-07-02) with `lapis-0.2.0` (Linux) and `lapis-0.2.0.exe` (Windows) attached; v0.1.0 likewise (2026-05-22) |
| Blocker | none for the tool. `lapis-web` is still not deployed, so nothing public points at these downloads |

## Notes

`no-gps`, `no-camera`, and `clean` strip levels are implemented and tested
(`clean` keeps only structural segments — APP0/SOF/DHT/DQT/DRI/SOS — and drops
all APP1–APP15, COM, and vendor segments). Output is validated before
writing (`Strip` re-parses its result; a failed strip writes nothing). The
`--time` value is applied to surviving EXIF DateTime fields as well as the
filesystem, including the Windows creation time. The CLI accepts flags and paths
in any order, takes both `--flag` and Windows `/flag` styles, exits non-zero when
any file errors, and pauses on Windows for double-click users.

The last outstanding v1 stripping item — location stripping for `no-gps` from
APP13 IPTC and XMP — is now **done** (see `docs/IPTC-XMP-LOCATION-SCOPE.md`):
built `exifscalpel/iptc` + opt-in `xmp.CleanLocation`, shipped as exifscalpel
**v0.2.0** (tagged + pushed), re-vendored, and wired the lapis `no-gps` policy —
strip the 7 record-2 location datasets from APP13 (drop the segment if it
empties); blank location/GPS values in XMP while keeping the block. Verified
end-to-end against the built binary. macOS binary is v1.5. (The `reencoded`
level — pixel rework, the `goindigo` engine — is a v3 placeholder that errors
today.)

## Fixed bugs (were shipped in v0.1.0)

Both surfaced verifying no-gps; both predated this work (first commit `817e1d9`,
in released v0.1.0) and lived in `cmd/lapis/main.go`, untested there
(`main_test.go` only covered argument parsing). Now fixed, with regression tests
that drive the real output path (`TestRun_SingleFileDefaultOutput`,
`TestRun_InPlaceOverwrites`, `TestRun_InPlaceRenameMovesOriginal`):

1. **Single-file default output failed.** `run()` passed the file path itself as
   `baseDir`, so the `_lapis/` output path resolved under the file
   (`MkdirAll("photo.jpg/_lapis")` → "not a directory") — `lapis photo.jpg`, the
   primary usage, errored and wrote nothing. Fix: pass `filepath.Dir(target)` as
   `baseDir` for the single-file case.
2. **`--in-place` never overwrote.** `collisionSafe` ran in in-place mode too, so
   it wrote `photo_1.jpg` beside the original instead of modifying it. Fix: in
   in-place mode skip the collision suffix and write atomically via a temp file +
   rename (`writeAtomic`); with `--rename` it's a move (original removed).

See [`CHANGELOG.md`](CHANGELOG.md) for the full history of changes.
