// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package rename

import (
	"crypto/rand"
	"fmt"
	"io"
)

// Mode controls how output filenames are generated.
type Mode int

const (
	ModeScramble Mode = iota + 1 // random 8-char lowercase hex string
	ModeUUID                     // UUID4
)

// ParseMode converts a string flag value to a Mode.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "scramble":
		return ModeScramble, nil
	case "uuid":
		return ModeUUID, nil
	default:
		return 0, fmt.Errorf("unknown rename mode %q: must be scramble or uuid", s)
	}
}

// NewName generates a new filename stem (without extension) using the given mode.
func NewName(mode Mode) (string, error) {
	switch mode {
	case ModeScramble:
		return scramble()
	case ModeUUID:
		return uuid4()
	default:
		return "", fmt.Errorf("unknown rename mode %d", mode)
	}
}

func scramble() (string, error) {
	var b [4]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x", b), nil
}

func uuid4() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
