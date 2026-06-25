# Status

*Last updated: 2026-06-25*

| Field | Value |
|:--|:--|
| Phase | building (MVP released as v0.1.0; v1 incomplete) |
| Version | v0.1.0 |
| Build | passing |
| Tests | passing (`internal/strip`, `internal/rename`) |
| Deployed | v0.1.0 binaries built in `dist/` (Linux + Windows) |
| Blocker | confirm v0.1.0 Codeberg release is published with binaries attached |

## Notes

`no-gps`, `no-camera`, and `clean` strip levels are implemented and tested
(`clean` keeps only structural segments — APP0/SOF/DHT/DQT/DRI/SOS — and drops
all APP1–APP15, COM, and vendor segments). Output is validated before
writing (`Strip` re-parses its result; a failed strip writes nothing). The
`--time` value is applied to surviving EXIF DateTime fields as well as the
filesystem, including the Windows creation time. The CLI accepts flags and paths
in any order, takes both `--flag` and Windows `/flag` styles, exits non-zero when
any file errors, and pauses on Windows for double-click users. The only item
outstanding for a complete **v1** is location stripping for `no-gps` from APP13
IPTC and XMP — now **scoped** (see `docs/IPTC-XMP-LOCATION-SCOPE.md`): build a new
`exifscalpel/iptc` package + an opt-in `xmp.CleanLocation`, ship as exifscalpel
v0.2.0, re-vendor, then add the lapis `no-gps` policy. macOS binary is v1.5. (The `reencoded` level — pixel rework, the `goindigo`
engine — is a v3 placeholder that errors today.)

See [`CHANGELOG.md`](CHANGELOG.md) for the full history of changes.
