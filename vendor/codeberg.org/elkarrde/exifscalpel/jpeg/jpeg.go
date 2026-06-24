// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package jpeg parses and writes JPEG marker segments and identifies the
// EXIF and XMP APP1 segments. It depends only on the standard library.
//
// Parsing reads every marker segment up to and including SOS (start of scan)
// and returns the compressed image data and EOI as an opaque tail. Writing
// reassembles a byte-faithful JPEG from a segment list and that tail.
//
// The package deals in segment structure only; interpreting an EXIF or XMP
// payload is the job of the exif and xmp packages. Lifted from the segment
// layer shared by codeberg.org/elkarrde/lapis and tidy-exif.
package jpeg

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Segment is a single JPEG marker segment: the marker byte (the value after
// the 0xFF prefix) and the payload that follows its two-byte length field.
// The length field itself is not stored; Write recomputes it.
type Segment struct {
	Marker byte
	Data   []byte
}

var (
	// exifSig is the prefix that identifies an EXIF APP1 segment payload.
	exifSig = []byte{'E', 'x', 'i', 'f', 0, 0}
	// xmpSig is the Adobe xap namespace URI that identifies an XMP APP1 payload.
	xmpSig = []byte("http://ns.adobe.com/xap/1.0/\x00")
)

// IsEXIF reports whether s is an EXIF APP1 segment (marker 0xE1 with the
// "Exif\x00\x00" prefix).
func IsEXIF(s Segment) bool {
	return s.Marker == 0xE1 && bytes.HasPrefix(s.Data, exifSig)
}

// IsXMP reports whether s is an XMP APP1 segment (marker 0xE1 with the Adobe
// xap namespace prefix).
func IsXMP(s Segment) bool {
	return s.Marker == 0xE1 && bytes.HasPrefix(s.Data, xmpSig)
}

// Parse reads a complete JPEG from r and returns its marker segments plus the
// raw tail starting at the SOS payload (the compressed image data through
// EOI). Parsing stops at SOS; the tail is returned verbatim so it can be
// written back unchanged.
//
// It returns an error if r is not a JPEG (missing SOI) or is malformed
// (unexpected byte where a marker was expected, truncated or invalid segment
// length).
func Parse(r io.Reader) (segs []Segment, tail []byte, err error) {
	all, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	if len(all) < 2 || all[0] != 0xFF || all[1] != 0xD8 {
		return nil, nil, fmt.Errorf("not a JPEG file")
	}

	i := 2 // skip SOI
	for i < len(all) {
		if all[i] != 0xFF {
			return nil, nil, fmt.Errorf("JPEG parse error at offset %d: expected 0xFF", i)
		}
		// legal JPEG padding: skip consecutive 0xFF bytes before a marker
		for i < len(all) && all[i] == 0xFF {
			i++
		}
		if i >= len(all) {
			break
		}
		m := all[i]
		i++

		// standalone markers (no payload): SOI, EOI, RST0-RST7
		if m == 0xD8 || m == 0xD9 || (m >= 0xD0 && m <= 0xD7) {
			if m == 0xD9 {
				return segs, []byte{0xFF, 0xD9}, nil
			}
			continue
		}

		if i+2 > len(all) {
			return nil, nil, fmt.Errorf("truncated JPEG segment at marker 0xFF%02X", m)
		}
		length := int(binary.BigEndian.Uint16(all[i:]))
		if length < 2 || i+length > len(all) {
			return nil, nil, fmt.Errorf("invalid segment length at 0xFF%02X: %d", m, length)
		}
		payload := make([]byte, length-2)
		copy(payload, all[i+2:i+length])

		if m == 0xDA { // SOS: image data follows the header
			segs = append(segs, Segment{Marker: m, Data: payload})
			return segs, all[i+length:], nil
		}

		segs = append(segs, Segment{Marker: m, Data: payload})
		i += length
	}
	return segs, nil, nil
}

// Write emits a JPEG to w: the SOI marker, each segment (marker, recomputed
// two-byte length, payload), then the raw tail (compressed image data + EOI)
// exactly as returned by Parse.
func Write(w io.Writer, segs []Segment, tail []byte) error {
	if _, err := w.Write([]byte{0xFF, 0xD8}); err != nil {
		return err
	}
	hdr := make([]byte, 4)
	for _, s := range segs {
		hdr[0] = 0xFF
		hdr[1] = s.Marker
		binary.BigEndian.PutUint16(hdr[2:], uint16(len(s.Data)+2))
		if _, err := w.Write(hdr); err != nil {
			return err
		}
		if _, err := w.Write(s.Data); err != nil {
			return err
		}
	}
	_, err := w.Write(tail)
	return err
}
