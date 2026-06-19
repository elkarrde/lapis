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

`scout` and `journalist` strip levels are implemented and tested. Outstanding for
a complete **v1** (see `TODO.md`): the `ghost` level, IPTC location stripping for
`scout`, applying timestamps to surviving EXIF fields, Windows creation-time
editing, and the `ghost`/output-validation tests. Shippable as an MVP today;
finish `ghost` before claiming v1. macOS binary is v1.5.
