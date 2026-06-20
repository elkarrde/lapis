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
	"strings"
	"time"

	"codeberg.org/elkarrde/lapis/internal/rename"
	"codeberg.org/elkarrde/lapis/internal/strip"
	"codeberg.org/elkarrde/lapis/internal/timestamp"
)

const version = "0.1.0"

func main() {
	level := flag.String("level", "journalist", "stripping level: scout | journalist | ghost")
	ren := flag.String("rename", "", "filename scrambling: scramble | uuid")
	tim := flag.String("time", "", "timestamp mode: random | shift | now")
	timeStart := flag.String("time-range-start", "2015-01-01", "start of random time range (YYYY-MM-DD)")
	timeEnd := flag.String("time-range-end", "2023-12-31", "end of random time range (YYYY-MM-DD)")
	inPlace := flag.Bool("in-place", false, "modify files directly instead of writing to _lapis/")
	recursive := flag.Bool("recursive", false, "process subdirectories")
	verbose := flag.Bool("verbose", false, "print per-file actions")
	ver := flag.Bool("version", false, "print version and exit")

	flag.Parse()

	if *ver {
		fmt.Println(version)
		os.Exit(0)
	}

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: lapis [options] <file|directory>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	stripLevel, err := strip.ParseLevel(*level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lapis: %v\n", err)
		os.Exit(1)
	}

	var renameMode rename.Mode
	if *ren != "" {
		renameMode, err = rename.ParseMode(*ren)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lapis: %v\n", err)
			os.Exit(1)
		}
	}

	tsOpts := timestamp.Options{Mode: 0}
	if *tim != "" {
		tsOpts.Mode, err = timestamp.ParseMode(*tim)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lapis: %v\n", err)
			os.Exit(1)
		}
		if tsOpts.Mode == timestamp.ModeRandom {
			tsOpts.RangeStart, err = parseDate(*timeStart)
			if err != nil {
				fmt.Fprintf(os.Stderr, "lapis: --time-range-start: %v\n", err)
				os.Exit(1)
			}
			tsOpts.RangeEnd, err = parseDate(*timeEnd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "lapis: --time-range-end: %v\n", err)
				os.Exit(1)
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
	for _, target := range flag.Args() {
		p, s, e := run(target, opts)
		processed += p
		skipped += s
		errors += e
	}

	fmt.Printf("Processed: %d  Skipped: %d  Errors: %d\n", processed, skipped, errors)
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
	ok, err2 := processFile(target, target, opts)
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

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}

	var buf bytes.Buffer
	stripErr := strip.Strip(f, &buf, opts.level)
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

	outPath := collisionSafe(filepath.Join(outDir, outName))

	// Determine timestamp before writing.
	var ts time.Time
	if opts.doTime {
		ts, err = timestamp.Pick(opts.tsOpts, info.ModTime())
		if err != nil {
			return false, fmt.Errorf("timestamp: %w", err)
		}
	}

	if opts.inPlace {
		if err := os.WriteFile(outPath, buf.Bytes(), info.Mode()); err != nil {
			return false, err
		}
	} else {
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
