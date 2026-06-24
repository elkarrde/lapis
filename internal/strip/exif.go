// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package strip

import "codeberg.org/elkarrde/exifscalpel/exif"

// This file holds lapis's EXIF stripping *policy* — which tags each level keeps
// or removes. The TIFF/IFD parse and rebuild engine lives in
// codeberg.org/elkarrde/exifscalpel/exif; we parse into exif.Data, filter the
// IFDs per the level, and let exif.Data.Build reserialize (it reconciles the
// sub-IFD pointer entries from the populated sub-IFDs).

// filterKeep returns only the entries whose tag is in keep.
func filterKeep(entries []exif.Entry, keep map[uint16]bool) []exif.Entry {
	out := entries[:0:0]
	for _, e := range entries {
		if keep[e.Tag] {
			out = append(out, e)
		}
	}
	return out
}

// filterRemove returns the entries whose tag is not in remove.
func filterRemove(entries []exif.Entry, remove map[uint16]bool) []exif.Entry {
	out := entries[:0:0]
	for _, e := range entries {
		if !remove[e.Tag] {
			out = append(out, e)
		}
	}
	return out
}

// ---- Level-specific EXIF processors ----

// no-camera: keep only shooting data, nothing identifying.
var noCameraIFD0Keep = map[uint16]bool{
	0x0100: true, // ImageWidth
	0x0101: true, // ImageLength
	// ExifIFD pointer (0x8769) is reconciled by Build when the sub-IFD survives.
}

var noCameraExifSubKeep = map[uint16]bool{
	0x829A: true, // ExposureTime
	0x829D: true, // FNumber
	0x8827: true, // ISOSpeedRatings
	0x9003: true, // DateTimeOriginal
	0x9202: true, // ApertureValue
	0x9204: true, // ExposureBiasValue
	0x9205: true, // MaxApertureValue
	0x9209: true, // Flash
	0x920A: true, // FocalLength
	0xA001: true, // ColorSpace
	0xA405: true, // FocalLengthIn35mmFilm
}

func processNoCameraEXIF(data []byte) ([]byte, error) {
	d, err := exif.Parse(data)
	if err != nil {
		return nil, err
	}

	d.IFD0 = filterKeep(d.IFD0, noCameraIFD0Keep)
	d.ExifSub = filterKeep(d.ExifSub, noCameraExifSubKeep)
	d.GPSSub = nil // drop all GPS data

	return d.Build()
}

// no-gps: remove GPS sub-IFD and any other unresolvable pointer sub-IFDs.
// All other IFD0 and Exif sub-IFD data is preserved.
var noGPSIFD0Remove = map[uint16]bool{
	0x8825: true, // GPSIFD pointer — drops all GPS data
	0xA005: true, // Interoperability IFD — pointer cannot be safely rewritten
	0x014A: true, // SubIFDs — same
}

func processNoGPSEXIF(data []byte) ([]byte, error) {
	d, err := exif.Parse(data)
	if err != nil {
		return nil, err
	}

	d.IFD0 = filterRemove(d.IFD0, noGPSIFD0Remove)
	d.GPSSub = nil // ensure Build does not re-add the GPS pointer
	// ExifSub is preserved as-is.

	return d.Build()
}
