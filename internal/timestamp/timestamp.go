// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package timestamp

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

// Mode controls how timestamps are applied to processed files.
type Mode int

const (
	ModeRandom Mode = iota + 1 // random time within a configurable range
	ModeShift                  // uniform random offset applied across a batch run
	ModeNow                    // time.Now()
)

// Options configures timestamp behaviour for a batch run.
// Shift is a shared pointer: nil on first call triggers generation of an offset
// that is reused for all subsequent files in the batch.
type Options struct {
	Mode       Mode
	RangeStart time.Time
	RangeEnd   time.Time
	Shift      *time.Duration // caller allocates; nil value triggers generation
}

// ParseMode converts a string flag value to a Mode.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "random":
		return ModeRandom, nil
	case "shift":
		return ModeShift, nil
	case "now":
		return ModeNow, nil
	default:
		return 0, fmt.Errorf("unknown time mode %q: must be random, shift, or now", s)
	}
}

// Pick returns the timestamp to apply for a given file.
// For ModeShift, originalMtime is the file's current modification time;
// the same offset is generated once and reused via opts.Shift.
func Pick(opts *Options, originalMtime time.Time) (time.Time, error) {
	switch opts.Mode {
	case ModeNow:
		return time.Now(), nil

	case ModeRandom:
		delta := opts.RangeEnd.Sub(opts.RangeStart)
		if delta <= 0 {
			return time.Time{}, fmt.Errorf("time-range-end must be after time-range-start")
		}
		n, err := randInt63(int64(delta))
		if err != nil {
			return time.Time{}, err
		}
		return opts.RangeStart.Add(time.Duration(n)), nil

	case ModeShift:
		if *opts.Shift == 0 {
			// generate once per batch; zero is a valid shift but treated as unset sentinel
			// use a non-zero guaranteed: generate in [1, 730days] and randomly negate
			days365 := int64(365 * 24 * time.Hour)
			n, err := randInt63(days365 * 2)
			if err != nil {
				return time.Time{}, err
			}
			d := time.Duration(n - days365)
			if d == 0 {
				d = time.Hour // avoid zero sentinel
			}
			*opts.Shift = d
		}
		return originalMtime.Add(*opts.Shift), nil

	default:
		return time.Time{}, fmt.Errorf("unknown mode %d", opts.Mode)
	}
}

// Apply sets the filesystem mtime and atime of path to t.
// On Linux, ctime cannot be set from userspace and will reflect the time of
// processing — this is a kernel limitation, not a bug.
func Apply(path string, t time.Time) error {
	return os.Chtimes(path, t, t)
}

// randInt63 returns a cryptographically random int64 in [0, max).
func randInt63(max int64) (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	v := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	return v % max, nil
}
