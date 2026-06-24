# Status

*Last updated: 2026-06-19*

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
all APP1–APP15, COM, and vendor segments). Outstanding for a complete **v1**
(see `TODO.md`): IPTC location stripping for `no-gps`, applying timestamps to
surviving EXIF fields, Windows creation-time editing, and output validation.
macOS binary is v1.5. (The `reencoded` level — pixel rework, the `goindigo`
engine — is a v3 placeholder that errors today.)

See [`CHANGELOG.md`](CHANGELOG.md) for the full history of changes.
