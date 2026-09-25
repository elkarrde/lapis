# FUTURE — lapis

Exploratory / not-yet-scheduled work. Captured for later; nothing here is committed
to a release. Mirrors the plan written for the sibling tool in
`../tidy-exif/FUTURE.md` (same exifscalpel foundation, same use-case shape), with
the lapis-specific differences called out.

---

## Windows: right-click (Explorer) integration + dual-use exe  *(decided, not scheduled; 2026-09-24)*

**Goal:** let Windows users strip a JPEG by right-clicking it (or a folder, or a
selection) in Explorer, while `lapis.exe` keeps working as the CLI it is today.

### Console vs GUI: the one hard constraint

A PE binary is *either* console or GUI subsystem, never both. Two acceptable
shapes (pick when scheduled):

- **One console exe + launch-context detection.** CLI unchanged (stdout, pipes,
  exit codes). The registry verb / Send to shortcut passes an explicit flag (e.g.
  `--shell`); a double-click is detected with `GetConsoleProcessList` (we are the
  only process on our console). Then hide the console at once
  (`ShowWindow(GetConsoleWindow(), SW_HIDE)`) and report via MessageBox. Cost: a
  split-second console flash. Side fix: `waitIfWindows` then pauses only when we
  own the console — today it also pauses inside `cmd`.
- **Two exes from one source**, like `python.exe`/`pythonw.exe`: `lapis.exe`
  (console) plus `lapisw.exe` (`-H=windowsgui`) for the verbs and Send to. No
  flash, no guessing; one extra build line and a second file in the release.

Rejected: GUI exe + `AttachConsole(ATTACH_PARENT_PROCESS)` — `cmd` returns the
prompt immediately and output lands after it.

### Multi-select findings  *(tested on real Windows, 2026-09-24)*

Tested with contextexif (`MultiSelectModel = Document`):

- **≤ 15 files → one process per file**, each with one path. `Document` does not
  batch the selection into one argv.
- **≥ 16 files → the verb is not shown at all.**
- **Launch order depends on where in the selection you right-clicked.**
- Still to record: `.jpg` + `.jpeg` mix, and whether `MultiSelectModel = Player`
  keeps the verb visible past 15.

**Why this matters more for lapis than for tidy-exif:** `--time shift` picks
**one random offset per batch** so relative ordering survives. Per-file launches
would each pick their own offset — silently breaking that guarantee — and
`--rename`/collision handling would race across N concurrent processes writing
into the same `_lapis/`.

### Batch path: "Send to" *(the preferred shape)*

A shortcut in `shell:sendto` receives the **whole selection in one process**
(one argv; no 15-item cap, only the ~32K command-line limit), which keeps
`--time shift` and the end summary (`Processed: N  Skipped: N  Errors: N`) whole.
lapis already takes files and directories positionally, in any order with flags,
so no CLI change is needed for this.

One shortcut per level reads naturally:

```
Send to ▸ Lapis — remove GPS              lapis.exe --shell --level no-gps      <paths>
Send to ▸ Lapis — remove camera info      lapis.exe --shell --level no-camera   <paths>
Send to ▸ Lapis — remove all metadata     lapis.exe --shell --level clean       <paths>
```

(`reencoded` stays out until it is implemented.) Whether `--rename`/`--time` get
their own shortcuts or a config default is open.

The right-click verb stays for one-offs (and as the "Lapis" item in the planned
shared **EXIF…** cascade — see `../tidy-exif/FUTURE.md` §"Shared "EXIF…" cascade";
Lapis is already one of its named items). A folder verb (`Directory\shell`,
optionally with `--recursive`) is the other reliable batch path.

### Right-click specifics

- **Default output is already right-click-safe:** results go to `_lapis/` beside
  the input and originals are never modified without `--in-place`. Keep that for
  the verbs; do not offer `--in-place` from the menu.
- The **`clean` steganography warning** must reach the MessageBox, not just
  stderr — it is the one message this level must never lose.
- The summary MessageBox reports `Processed / Skipped / Errors`, and names the
  `_lapis/` folder (or offers to open it).
- `-install` / `-uninstall` ported from contextexif (`internal/setup` +
  `internal/shellmenu`): per-user copy under `%LOCALAPPDATA%\Programs`, HKCU
  verbs for `.jpg`/`.jpeg`, the Send to shortcuts, Settings → Apps entry. Pairs
  with the pending `.exe` icon item in `TODO.md` (verbs and shortcuts show it).
- **Run-time-deps rule holds:** `golang.org/x/sys/windows` (registry, as
  contextexif uses) is pure Go and links into the same one file. Alternatively
  bind the few Win32 calls via stdlib `syscall.NewLazyDLL` and add no `require`.

---

## Browser version (WASM) — strip in the browser, no upload  *(decided, not scheduled; 2026-09-24)*

**Goal:** a page on lapis-web where someone drops JPEGs and gets stripped copies
back, processed entirely in the browser — the file never reaches a server.

### Feasibility (probed 2026-09-24)

`internal/strip` compiles to `GOOS=js GOARCH=wasm` **unchanged** and offline
(`GOPROXY=off`, vendored): a ~20-line `syscall/js` wrapper around `strip.Strip`
built with `-ldflags="-s -w"` is **2.9 MB raw / 0.83 MB gzipped** — fine
lazy-loaded on first drop. TinyGo untried.

### Design notes

- **Header-only processing works here too, even though lapis changes lengths.**
  `jpeg.Parse` hands back the scan data + EOI (+ anything after) as an opaque
  tail that `Strip` writes verbatim, so the output is exactly
  `strippedHeader + file.slice(sosOffset)`. Slice the first ~128 KB, strip that in
  WASM, and splice. Needs a small header-only entry point (return the stripped
  header plus the SOS offset) — `Strip` itself expects the whole file and
  re-parses its output for validation.
- **What maps and what does not:**
  - `--level` → a level picker; `--rename` → the download/written filename.
  - `_lapis/` → Chromium's `showDirectoryPicker` can write into a real `_lapis/`
    subfolder; elsewhere, a single download or a zip for a batch.
  - `--time`: the **EXIF DateTime rewrite works** (it is inside `Strip`), but
    **filesystem timestamps cannot be set from a browser** — a download or an FS
    Access write gets "now". Say so on the page rather than offer it silently.
  - `--in-place` → FS Access overwrite on Chromium only; do not offer elsewhere.

### No logging — the tidy-exif plan's debug log does not carry over

lapis is a privacy tool for journalists; the page has to be trustworthy *and
verifiably so*:

- **No debug log, no analytics, no beacon.** Even "which fields were found" says
  something about a source's camera. If diagnostics are ever wanted, opt-in only,
  version + outcome, never on by default.
- **Make "no upload" checkable, not just claimed:** a strict
  `Content-Security-Policy` (`connect-src 'none'`, scripts/wasm from self only)
  so the browser itself forbids any request after load; a note telling users they
  can disconnect from the network before dropping files.
- **No third-party requests at all on that page** — self-host fonts rather than
  the Google Fonts CDN lapis-web uses, since every CDN hit reports the visitor's
  IP to a third party.
- Consider a service worker so the page works fully offline once loaded (and can
  be installed as a PWA).

### Next step when scheduled

Prototype: the header-only strip entry point + a bare drop page with the CSP in
place, to prove the splice on a real file before any design work.

---

## Observation to follow up (not part of either plan)

Noted while checking the WASM splice: exifscalpel's `jpeg.Parse` stops at the
**first** SOS and returns everything after it as the opaque tail, which `Strip`
writes **verbatim at every level, including `clean`**. That covers data after
EOI (e.g. a Google Motion Photo's appended MP4, a Samsung trailer) and any marker
segment that sits between the scans of a progressive JPEG. Worth checking against
real phone files whether such data carries identifying metadata lapis should
drop.
