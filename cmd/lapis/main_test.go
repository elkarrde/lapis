// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"reflect"
	"testing"
)

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
