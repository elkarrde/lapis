// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package strip

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"codeberg.org/elkarrde/exifscalpel/iptc"
	"codeberg.org/elkarrde/exifscalpel/jpeg"
	"codeberg.org/elkarrde/exifscalpel/xmp"
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
// If exifTime is non-nil, any EXIF DateTime field that survives the level's
// filtering is rewritten to that time (DateTimeOriginal at no-camera; also
// DateTime and DateTimeDigitized at no-gps). It is the caller's job to pass the
// same value it applies to the filesystem timestamp. Pass nil to leave EXIF
// times untouched.
//
// The byte-level segment parse/write and EXIF/XMP identification come from
// codeberg.org/elkarrde/exifscalpel; this package keeps the stripping policy
// (levels and the per-level segment processing in processSegments).
func Strip(r io.Reader, w io.Writer, level Level, exifTime *time.Time) error {
	segs, tail, err := jpeg.Parse(r)
	if err != nil {
		return err
	}
	out, err := processSegments(segs, level, exifTime)
	if err != nil {
		return err
	}

	// Serialize into a buffer and re-parse it before writing a single byte to w.
	// We never emit output we can't read back as a JPEG: this catches a rebuilt
	// segment that overflowed its 16-bit length, and it matters most for the
	// CLI's --in-place mode, where a corrupt result would overwrite the original.
	var buf bytes.Buffer
	if err := jpeg.Write(&buf, out, tail); err != nil {
		return err
	}
	if _, _, err := jpeg.Parse(bytes.NewReader(buf.Bytes())); err != nil {
		return fmt.Errorf("output validation failed: stripped result is not a valid JPEG: %w", err)
	}
	_, err = w.Write(buf.Bytes())
	return err
}

func processSegments(segs []jpeg.Segment, level Level, exifTime *time.Time) ([]jpeg.Segment, error) {
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
				newData, err := processNoCameraEXIF(s.Data, exifTime)
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
			switch {
			case jpeg.IsEXIF(s):
				newData, err := processNoGPSEXIF(s.Data, exifTime)
				if err != nil {
					out = append(out, s) // best-effort: keep original on parse failure
				} else {
					out = append(out, jpeg.Segment{Marker: 0xE1, Data: newData})
				}
			case jpeg.IsXMP(s): // XMP APP1 — blank location values, keep the rest
				out = appendNoGPSXMP(out, s)
			case s.Marker == 0xED: // APP13 / IPTC — strip location datasets only
				out = appendNoGPSIPTC(out, s)
			default:
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// noGPSLocationDatasets are the IPTC-IIM record-2 datasets that carry location
// data, removed at the no-gps level. Everything else in APP13 — descriptive
// datasets (By-line, Caption, Keywords, Object Name, ...) and every non-IPTC
// Photoshop resource block (thumbnail, ICC, embedded EXIF) — is preserved.
var noGPSLocationDatasets = []struct{ record, number uint8 }{
	{2, 26},  // Content Location Code (repeatable)
	{2, 27},  // Content Location Name (repeatable)
	{2, 90},  // City
	{2, 92},  // Sub-location
	{2, 95},  // Province/State
	{2, 100}, // Country/Primary Location Code
	{2, 101}, // Country/Primary Location Name
}

// appendNoGPSIPTC strips the IPTC location datasets from an APP13 segment and
// appends the result to out. A non-Photoshop or otherwise unparseable APP13 is
// passed through unchanged (best-effort — never emit corrupt IPTC). If stripping
// leaves the Photoshop resource set empty, the whole segment is dropped.
func appendNoGPSIPTC(out []jpeg.Segment, s jpeg.Segment) []jpeg.Segment {
	d, err := iptc.Parse(s.Data)
	if err != nil {
		return append(out, s) // not a Photoshop APP13 we understand; leave it
	}
	removed := 0
	for _, ds := range noGPSLocationDatasets {
		removed += d.Remove(ds.record, ds.number)
	}
	if removed == 0 {
		return append(out, s) // no location data present; keep original bytes
	}
	if d.Empty() {
		return out // all resource blocks gone; drop the segment entirely
	}
	newData, err := d.Build()
	if err != nil {
		return append(out, s) // best-effort: keep original on rebuild failure
	}
	return append(out, jpeg.Segment{Marker: 0xED, Data: newData})
}

// appendNoGPSXMP strips location and GPS fields from an XMP APP1 segment and
// appends the result to out. Unlike no-camera (which excises XMP entirely),
// no-gps keeps the XMP block and only blanks its location values. A parse
// failure or a block carrying no location data is passed through unchanged.
func appendNoGPSXMP(out []jpeg.Segment, s jpeg.Segment) []jpeg.Segment {
	newData, changed, err := xmp.CleanLocation(s.Data)
	if err != nil || !changed {
		return append(out, s)
	}
	return append(out, jpeg.Segment{Marker: 0xE1, Data: newData})
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
