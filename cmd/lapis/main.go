// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"codeberg.org/elkarrde/lapis/internal/rename"
	"codeberg.org/elkarrde/lapis/internal/strip"
	"codeberg.org/elkarrde/lapis/internal/timestamp"
)

const version = "0.2.0"

func main() {
	level := flag.String("level", "no-camera", "stripping level: no-gps | no-camera | clean | reencoded")
	reencode := flag.Bool("reencode", false, "shorthand for --level reencoded (pixel rework; planned)")
	ren := flag.String("rename", "", "filename scrambling: scramble | uuid")
	tim := flag.String("time", "", "timestamp mode: random | shift | now")
	timeStart := flag.String("time-range-start", "2015-01-01", "start of random time range (YYYY-MM-DD)")
	timeEnd := flag.String("time-range-end", "2023-12-31", "end of random time range (YYYY-MM-DD)")
	inPlace := flag.Bool("in-place", false, "modify files directly instead of writing to _lapis/")
	recursive := flag.Bool("recursive", false, "process subdirectories")
	verbose := flag.Bool("verbose", false, "print per-file actions")
	ver := flag.Bool("version", false, "print version and exit")

	// Accept Windows /flag style on all platforms, and allow flags and the
	// file/dir arguments to appear in any order.
	targets, _ := interleave(flag.CommandLine, normaliseArgs(flag.CommandLine, os.Args[1:]))

	if *ver {
		fmt.Println(version)
		die(0)
	}

	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lapis [options] <file|directory>")
		flag.PrintDefaults()
		die(1)
	}

	// --reencode is shorthand for --level reencoded.
	if *reencode {
		*level = "reencoded"
	}

	stripLevel, err := strip.ParseLevel(*level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lapis: %v\n", err)
		die(1)
	}

	// Pixel rework is planned (goindigo); refuse before touching any files.
	if stripLevel == strip.LevelReencoded {
		fmt.Fprintln(os.Stderr, "lapis: --level reencoded not yet implemented")
		die(1)
	}

	var renameMode rename.Mode
	if *ren != "" {
		renameMode, err = rename.ParseMode(*ren)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lapis: %v\n", err)
			die(1)
		}
	}

	tsOpts := timestamp.Options{Mode: 0}
	if *tim != "" {
		tsOpts.Mode, err = timestamp.ParseMode(*tim)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lapis: %v\n", err)
			die(1)
		}
		if tsOpts.Mode == timestamp.ModeRandom {
			tsOpts.RangeStart, err = parseDate(*timeStart)
			if err != nil {
				fmt.Fprintf(os.Stderr, "lapis: --time-range-start: %v\n", err)
				die(1)
			}
			tsOpts.RangeEnd, err = parseDate(*timeEnd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "lapis: --time-range-end: %v\n", err)
				die(1)
			}
		}
		if tsOpts.Mode == timestamp.ModeShift {
			var shiftOnce time.Duration
			tsOpts.Shift = &shiftOnce
		}
	}

	opts := options{
		level:      stripLevel,
		renameMode: renameMode,
		doRename:   *ren != "",
		tsOpts:     &tsOpts,
		doTime:     *tim != "",
		inPlace:    *inPlace,
		recursive:  *recursive,
		verbose:    *verbose,
	}

	var processed, skipped, errors int
	for _, target := range targets {
		p, s, e := run(target, opts)
		processed += p
		skipped += s
		errors += e
	}

	fmt.Printf("Processed: %d  Skipped: %d  Errors: %d\n", processed, skipped, errors)
	if errors > 0 {
		die(1)
	}
	die(0)
}

// normaliseArgs converts Windows-style /flag and /flag=value tokens to --flag so
// both styles are accepted on every platform (a tidy-exif family convention).
// Because lapis takes positional path arguments — unlike tidy-exif — a /token is
// rewritten only when its name matches a flag defined in fs; this leaves real
// paths such as /photos or /usr/local/pic.jpg untouched while accepting
// /level=clean, /in-place, and the rest.
func normaliseArgs(fs *flag.FlagSet, args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = arg
		if !strings.HasPrefix(arg, "/") {
			continue
		}
		name := arg[1:]
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		if fs.Lookup(name) != nil {
			out[i] = "--" + arg[1:]
		}
	}
	return out
}

// interleave parses fs over args while allowing positional arguments (the
// file/dir targets) to appear before, after, or among the flags — Go's flag
// package otherwise stops at the first non-flag token. It returns the collected
// positionals. With an ExitOnError fs a bad flag exits directly, so callers may
// ignore the returned error; tests use ContinueOnError and inspect it.
func interleave(fs *flag.FlagSet, args []string) ([]string, error) {
	var targets []string
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	for fs.NArg() > 0 {
		rest := fs.Args()
		targets = append(targets, rest[0])
		if err := fs.Parse(rest[1:]); err != nil {
			return targets, err
		}
	}
	return targets, nil
}

// die runs the Windows pause (if applicable) and exits with code.
func die(code int) {
	waitIfWindows()
	os.Exit(code)
}

// waitIfWindows pauses for Enter on Windows so the console window stays open when
// lapis is launched by double-clicking the .exe (a tidy-exif family convention).
func waitIfWindows() {
	if runtime.GOOS == "windows" {
		fmt.Println("\nPress <Enter> to close.")
		fmt.Scanln()
	}
}

type options struct {
	level      strip.Level
	renameMode rename.Mode
	doRename   bool
	tsOpts     *timestamp.Options
	doTime     bool
	inPlace    bool
	recursive  bool
	verbose    bool
}

func run(target string, opts options) (processed, skipped, errors int) {
	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lapis: %v\n", err)
		errors++
		return
	}

	if info.IsDir() {
		return runDir(target, target, opts)
	}
	// baseDir is the root the _lapis/ mirror hangs off; for a single file that is
	// the file's directory, not the file itself (else the output path would
	// resolve under the file, e.g. photo.jpg/_lapis).
	ok, err2 := processFile(target, filepath.Dir(target), opts)
	if err2 != nil {
		fmt.Fprintf(os.Stderr, "lapis: %s: %v\n", target, err2)
		errors++
	} else if ok {
		processed++
	} else {
		skipped++
	}
	return
}

// runDir processes all JPEG files in dir. baseDir is the top-level directory
// argument used to compute relative paths for mirroring into _lapis/.
func runDir(dir, baseDir string, opts options) (processed, skipped, errors int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lapis: %v\n", err)
		errors++
		return
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if opts.recursive {
				p, s, e := runDir(path, baseDir, opts)
				processed += p
				skipped += s
				errors += e
			}
			continue
		}
		if !isJPEG(entry.Name()) {
			continue
		}
		ok, err := processFile(path, baseDir, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lapis: %s: %v\n", path, err)
			errors++
		} else if ok {
			processed++
		} else {
			skipped++
		}
	}
	return
}

// processFile strips a single JPEG. baseDir is the root input directory
// (used to compute the _lapis/ mirror path). Returns true if processed.
func processFile(path, baseDir string, opts options) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	// Pick the timestamp up front: the same value is rewritten into surviving
	// EXIF DateTime fields during Strip and applied to the filesystem after the
	// write, so the two never disagree.
	var ts time.Time
	var exifTime *time.Time
	if opts.doTime {
		ts, err = timestamp.Pick(opts.tsOpts, info.ModTime())
		if err != nil {
			return false, fmt.Errorf("timestamp: %w", err)
		}
		exifTime = &ts
	}

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}

	var buf bytes.Buffer
	stripErr := strip.Strip(f, &buf, opts.level, exifTime)
	f.Close()
	if stripErr != nil {
		return false, stripErr
	}

	// Determine output filename stem and extension.
	ext := strings.ToLower(filepath.Ext(path))
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if opts.doRename {
		stem, err = rename.NewName(opts.renameMode)
		if err != nil {
			return false, fmt.Errorf("rename: %w", err)
		}
	}
	outName := stem + ext

	// Determine output directory.
	var outDir string
	if opts.inPlace {
		outDir = filepath.Dir(path)
	} else {
		rel, err := filepath.Rel(baseDir, filepath.Dir(path))
		if err != nil {
			rel = "."
		}
		outDir = filepath.Join(baseDir, "_lapis", rel)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return false, err
		}
	}

	var outPath string
	if opts.inPlace {
		// Modify the file directly: the destination is the file itself (or, with
		// --rename, a new name in the same directory — a move). Write atomically
		// via a temp file so a failure never leaves a half-written image, and do
		// NOT collision-suffix — the existing original is the intended target,
		// not a collision.
		outPath = filepath.Join(outDir, outName)
		if err := writeAtomic(outPath, buf.Bytes(), info.Mode()); err != nil {
			return false, err
		}
		if outPath != path {
			if err := os.Remove(path); err != nil { // --rename: drop the original
				return false, err
			}
		}
	} else {
		outPath = collisionSafe(filepath.Join(outDir, outName))
		if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
			return false, err
		}
	}

	if opts.doTime {
		if err := timestamp.Apply(outPath, ts); err != nil {
			fmt.Fprintf(os.Stderr, "lapis: warning: could not set timestamp on %s: %v\n", outPath, err)
		}
	}

	if opts.verbose {
		fmt.Printf("%s -> %s\n", path, outPath)
	}
	return true, nil
}

// writeAtomic writes data to a temp file in dest's directory, gives it mode,
// then renames it over dest. The rename is atomic on the same filesystem, so an
// interrupted or failed write never clobbers an existing file with partial
// output — the reason --in-place can overwrite an original safely.
func writeAtomic(dest string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".lapis-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// collisionSafe returns path if it does not exist, otherwise path_1, path_2, etc.
func collisionSafe(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func isJPEG(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".jpg" || ext == ".jpeg"
}

func parseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: expected YYYY-MM-DD", s)
	}
	return t, nil
}
