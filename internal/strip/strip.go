// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package strip

import (
	"errors"
	"fmt"
	"io"
	"os"

	"codeberg.org/elkarrde/exifscalpel/jpeg"
)

// Level controls how aggressively metadata is stripped.
type Level int

const (
	LevelNoGPS     Level = iota + 1 // GPS and location data only
	LevelNoCamera                   // all identifying metadata; keep shooting data
	LevelClean                      // excise all EXIF and IPTC segments entirely
	LevelReencoded                  // clean + pixel rework (planned; not yet implemented)
)

// ErrReencodeNotImplemented is returned when the reencoded level is requested.
// Pixel-level rework is planned future work (goindigo) and is not yet available.
var ErrReencodeNotImplemented = errors.New("reencoded level not yet implemented: pixel rework is planned (goindigo)")

const cleanWarning = `WARNING: Pixel-level steganographic fingerprints are not addressed by --level clean.
         Camera manufacturers (Canon, Nikon, Fuji) may embed invisible identifying
         patterns in image data. Use --level reencoded (planned) for full mitigation.`

// ParseLevel converts a string flag value to a Level.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "no-gps":
		return LevelNoGPS, nil
	case "no-camera":
		return LevelNoCamera, nil
	case "clean":
		return LevelClean, nil
	case "reencoded":
		return LevelReencoded, nil
	default:
		return 0, fmt.Errorf("unknown level %q: must be no-gps, no-camera, clean, or reencoded", s)
	}
}

// Strip reads a JPEG from r, applies the given stripping level, and writes
// the result to w. Returns an error if r is not a valid JPEG.
//
// The byte-level segment parse/write and EXIF/XMP identification come from
// codeberg.org/elkarrde/exifscalpel; this package keeps the stripping policy
// (levels and the per-level segment processing in processSegments).
func Strip(r io.Reader, w io.Writer, level Level) error {
	segs, tail, err := jpeg.Parse(r)
	if err != nil {
		return err
	}
	out, err := processSegments(segs, level)
	if err != nil {
		return err
	}
	return jpeg.Write(w, out, tail)
}

func processSegments(segs []jpeg.Segment, level Level) ([]jpeg.Segment, error) {
	var out []jpeg.Segment
	switch level {

	case LevelReencoded:
		// Pixel rework is planned future work (goindigo); refuse rather than
		// emit a file that misleadingly implies pixels were touched.
		return nil, ErrReencodeNotImplemented

	case LevelClean:
		fmt.Fprintln(os.Stderr, cleanWarning)
		for _, s := range segs {
			// drop APP1 (EXIF and XMP) and APP13 (IPTC)
			if s.Marker == 0xE1 || s.Marker == 0xED {
				continue
			}
			out = append(out, s)
		}

	case LevelNoCamera:
		for _, s := range segs {
			switch {
			case s.Marker == 0xED: // APP13 / IPTC — excise entirely
			case jpeg.IsXMP(s): // XMP APP1 — excise
			case jpeg.IsEXIF(s):
				newData, err := processNoCameraEXIF(s.Data)
				if err == nil {
					out = append(out, jpeg.Segment{Marker: 0xE1, Data: newData})
				}
				// on parse failure, drop segment rather than expose raw data
			default:
				out = append(out, s)
			}
		}

	case LevelNoGPS:
		for _, s := range segs {
			if jpeg.IsEXIF(s) {
				newData, err := processNoGPSEXIF(s.Data)
				if err != nil {
					out = append(out, s) // best-effort: keep original on parse failure
				} else {
					out = append(out, jpeg.Segment{Marker: 0xE1, Data: newData})
				}
				continue
			}
			// TODO: strip IPTC location fields from APP13 (requires IPTC segment parser)
			out = append(out, s)
		}
	}
	return out, nil
}
