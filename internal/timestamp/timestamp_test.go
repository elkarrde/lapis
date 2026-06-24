// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package timestamp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestApply_SetsModTime checks the cross-platform path: Apply sets mtime via
// os.Chtimes and the platform setCreationTime hook returns without error (a
// no-op off Windows). On Windows it additionally exercises the SetFileTime call.
func TestApply_SetsModTime(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.jpg")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	want := time.Date(2018, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := Apply(p, want); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(want) {
		t.Errorf("mtime = %v, want %v", info.ModTime(), want)
	}
}

func TestParseMode(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Mode
	}{
		{"random", ModeRandom},
		{"shift", ModeShift},
		{"now", ModeNow},
	} {
		got, err := ParseMode(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseMode(%q) = %v, %v; want %v, nil", tc.in, got, err, tc.want)
		}
	}
	if _, err := ParseMode("bogus"); err == nil {
		t.Error("ParseMode(\"bogus\"): expected error, got nil")
	}
}
