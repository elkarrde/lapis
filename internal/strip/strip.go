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
	LevelClean                      // keep only structural segments; excise all metadata
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
			// allow-list: keep only structural segments, drop everything else
			// (APP1–APP15, COM, and any vendor segment that can carry metadata)
			if isStructuralClean(s.Marker) {
				out = append(out, s)
			}
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

// isStructuralClean reports whether a segment marker must survive the clean
// level. The allow-list is APP0 (JFIF density/units), the frame and coding-table
// markers in 0xC0–0xCF (SOF0–SOF15, DHT at 0xC4, DAC at 0xCC; the unused JPG
// extension marker 0xC8 is excluded), DQT (0xDB), DRI (0xDD), and SOS (0xDA).
//
// SOI, EOI and RST markers never reach the segment list — jpeg.Parse emits SOI
// itself and returns the scan data plus EOI as the opaque tail — so they need no
// case here and are always preserved by jpeg.Write.
//
// Everything not on this list is dropped: APP1–APP15 (EXIF, XMP, ICC, IPTC/
// Photoshop, MPF, C2PA, "Ducky", and every other vendor APPn), the COM comment
// segment (0xFE), and any other non-structural marker.
func isStructuralClean(m byte) bool {
	switch {
	case m == 0xE0: // APP0 / JFIF
		return true
	case m >= 0xC0 && m <= 0xCF && m != 0xC8: // SOF0–15, DHT (0xC4), DAC (0xCC)
		return true
	case m == 0xDB: // DQT
		return true
	case m == 0xDD: // DRI
		return true
	case m == 0xDA: // SOS
		return true
	default:
		return false
	}
}
