// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"codeberg.org/elkarrde/exifscalpel/jpeg"

	"codeberg.org/elkarrde/lapis/internal/rename"
	"codeberg.org/elkarrde/lapis/internal/strip"
)

// writeTestJPEG writes a minimal valid JPEG to path: SOI + APP0 + optional COM
// + SOF0 + EOI. The COM segment gives clean/no-camera something to strip so the
// output bytes differ from the input.
func writeTestJPEG(t *testing.T, path string, withCOM bool) {
	t.Helper()
	segs := []jpeg.Segment{
		{Marker: 0xE0, Data: []byte{'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0}},
	}
	if withCOM {
		segs = append(segs, jpeg.Segment{Marker: 0xFE, Data: []byte("secret comment")})
	}
	segs = append(segs, jpeg.Segment{Marker: 0xC0, Data: []byte{8, 0, 1, 0, 1, 1, 1, 0x11, 0}})
	var buf bytes.Buffer
	if err := jpeg.Write(&buf, segs, []byte{0xFF, 0xD9}); err != nil {
		t.Fatalf("build test JPEG: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write test JPEG: %v", err)
	}
}

// TestRun_SingleFileDefaultOutput is the regression test for the single-file
// default-output path: `lapis photo.jpg` must write to _lapis/photo.jpg and
// leave the original untouched (previously it failed with mkdir photo.jpg/_lapis).
func TestRun_SingleFileDefaultOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	writeTestJPEG(t, in, false)

	p, s, e := run(in, options{level: strip.LevelNoCamera})
	if p != 1 || s != 0 || e != 0 {
		t.Fatalf("run: processed=%d skipped=%d errors=%d, want 1/0/0", p, s, e)
	}
	if _, err := os.Stat(filepath.Join(dir, "_lapis", "photo.jpg")); err != nil {
		t.Fatalf("expected default output at _lapis/photo.jpg: %v", err)
	}
	if _, err := os.Stat(in); err != nil {
		t.Errorf("original should be left in place: %v", err)
	}
}

// TestRun_InPlaceOverwrites is the regression test for --in-place: it must
// overwrite the original, not write a photo_1.jpg beside it, and must not create
// a _lapis/ directory.
func TestRun_InPlaceOverwrites(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	writeTestJPEG(t, in, true) // COM present so clean changes the bytes
	before, _ := os.ReadFile(in)

	p, s, e := run(in, options{level: strip.LevelClean, inPlace: true})
	if p != 1 || s != 0 || e != 0 {
		t.Fatalf("run: processed=%d skipped=%d errors=%d, want 1/0/0", p, s, e)
	}
	if _, err := os.Stat(filepath.Join(dir, "photo_1.jpg")); err == nil {
		t.Error("--in-place created photo_1.jpg instead of overwriting the original")
	}
	if _, err := os.Stat(filepath.Join(dir, "_lapis")); err == nil {
		t.Error("--in-place should not create a _lapis/ directory")
	}
	after, err := os.ReadFile(in)
	if err != nil {
		t.Fatalf("original missing after --in-place: %v", err)
	}
	if bytes.Equal(before, after) {
		t.Error("--in-place did not modify the file (COM should have been stripped)")
	}
	if bytes.Contains(after, []byte("secret comment")) {
		t.Error("COM segment survived clean --in-place")
	}
	if _, _, err := jpeg.Parse(bytes.NewReader(after)); err != nil {
		t.Errorf("--in-place output is not a valid JPEG: %v", err)
	}
}

// TestRun_InPlaceRenameMovesOriginal covers --in-place combined with --rename:
// the result is a move — the new (scrambled) name is written and the original
// path is removed, leaving exactly one JPEG.
func TestRun_InPlaceRenameMovesOriginal(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	writeTestJPEG(t, in, false)

	p, _, e := run(in, options{
		level:      strip.LevelNoCamera,
		inPlace:    true,
		doRename:   true,
		renameMode: rename.ModeScramble,
	})
	if p != 1 || e != 0 {
		t.Fatalf("run: processed=%d errors=%d, want 1/0", p, e)
	}
	if _, err := os.Stat(in); !os.IsNotExist(err) {
		t.Error("--in-place --rename should remove the original path")
	}
	entries, _ := os.ReadDir(dir)
	jpgs := 0
	for _, en := range entries {
		if strings.HasSuffix(en.Name(), ".jpg") {
			jpgs++
		}
	}
	if jpgs != 1 {
		t.Errorf("expected exactly one .jpg after in-place rename, got %d", jpgs)
	}
}

func TestNormaliseArgs(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("level", "", "")
	fs.Bool("in-place", false, "")

	got := normaliseArgs(fs, []string{
		"/level=clean",       // defined flag with value -> convert
		"/in-place",          // defined bool flag -> convert
		"/photos",            // single-segment path, not a flag -> keep
		"/usr/local/pic.jpg", // multi-segment path -> keep
		"--time=now",         // already --flag -> keep
		"a.jpg",              // relative path -> keep
	})
	want := []string{
		"--level=clean", "--in-place", "/photos", "/usr/local/pic.jpg", "--time=now", "a.jpg",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normaliseArgs:\n got %v\nwant %v", got, want)
	}
}

func TestInterleave(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		want      []string
		wantLevel string
	}{
		{"flags before paths", []string{"--level", "clean", "a.jpg", "b.jpg"}, []string{"a.jpg", "b.jpg"}, "clean"},
		{"flags after path", []string{"a.jpg", "--level", "clean"}, []string{"a.jpg"}, "clean"},
		{"interleaved", []string{"a.jpg", "--level", "clean", "b.jpg"}, []string{"a.jpg", "b.jpg"}, "clean"},
		{"equals form after path", []string{"a.jpg", "--level=clean"}, []string{"a.jpg"}, "clean"},
		{"no flags", []string{"a.jpg", "b.jpg"}, []string{"a.jpg", "b.jpg"}, "no-camera"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			level := fs.String("level", "no-camera", "")
			got, err := interleave(fs, tc.args)
			if err != nil {
				t.Fatalf("interleave: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("targets = %v, want %v", got, tc.want)
			}
			if *level != tc.wantLevel {
				t.Errorf("level = %q, want %q", *level, tc.wantLevel)
			}
		})
	}
}
