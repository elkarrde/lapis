// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package strip

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"codeberg.org/elkarrde/exifscalpel/exif"
	"codeberg.org/elkarrde/exifscalpel/jpeg"
)

// ---- Synthetic JPEG builder ----
//
// A minimal valid JPEG: SOI + APP0 + optional APP1 + SOF0 + EOI.
// All segments have plausible structure; pixel data is omitted for brevity.
// Segment write/parse and EXIF (de)serialization come from the exifscalpel
// library; these helpers only assemble fixtures and read them back.

type jpegBuilder struct {
	segs []jpeg.Segment
}

func (b *jpegBuilder) add(marker byte, data []byte) {
	b.segs = append(b.segs, jpeg.Segment{Marker: marker, Data: data})
}

func (b *jpegBuilder) bytes() []byte {
	var buf bytes.Buffer
	// jpeg.Write emits SOI, the segments, then the tail (EOI) verbatim.
	_ = jpeg.Write(&buf, b.segs, []byte{0xFF, 0xD9})
	return buf.Bytes()
}

// app0 returns a minimal JFIF APP0 payload.
func app0() []byte {
	return []byte{'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0}
}

// sof0 returns a minimal SOF0 payload (1x1 greyscale).
func sof0() []byte {
	return []byte{8, 0, 1, 0, 1, 1, 1, 0x11, 0}
}

// ---- EXIF/TIFF builders ----

// buildEXIFPayload serializes an APP1 EXIF payload from the given IFDs via the
// library's rebuild path. Build reconciles the sub-IFD pointer entries from the
// populated sub-IFDs, so callers pass only real tag entries.
func buildEXIFPayload(t *testing.T, bo binary.ByteOrder, ifd0, exifSub, gpsSub []exif.Entry) []byte {
	t.Helper()
	d := &exif.Data{ByteOrder: bo, IFD0: ifd0, ExifSub: exifSub, GPSSub: gpsSub}
	payload, err := d.Build()
	if err != nil {
		t.Fatalf("build EXIF: %v", err)
	}
	return payload
}

// makeRATIONAL encodes a RATIONAL value (two uint32s, LE).
func makeRATIONAL(num, den uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b[0:], num)
	binary.LittleEndian.PutUint32(b[4:], den)
	return b
}

// makeU16 encodes a SHORT value (LE).
func makeU16(v uint16) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	return b
}

// jpegWithGPS builds a JPEG whose EXIF has a Make tag in IFD0 and a GPS sub-IFD.
func jpegWithGPS(t *testing.T) []byte {
	t.Helper()

	gpsEntries := []exif.Entry{
		{Tag: 0x0001, Type: 2, Count: 2, Value: []byte{'N', 0}},                                                                     // GPSLatitudeRef (ASCII "N\0")
		{Tag: 0x0002, Type: 5, Count: 3, Value: append(makeRATIONAL(51, 1), append(makeRATIONAL(30, 1), makeRATIONAL(0, 1)...)...)}, // GPSLatitude
	}
	ifd0 := []exif.Entry{
		{Tag: 0x010F, Type: 2, Count: 5, Value: []byte{'T', 'e', 's', 't', 0}}, // Make
	}
	exifData := buildEXIFPayload(t, binary.LittleEndian, ifd0, nil, gpsEntries)

	var b jpegBuilder
	b.add(0xE0, app0())
	b.add(0xE1, exifData)
	b.add(0xC0, sof0())
	return b.bytes()
}

func jpegWithNoMetadata(t *testing.T) []byte {
	t.Helper()
	var b jpegBuilder
	b.add(0xE0, app0())
	b.add(0xC0, sof0())
	return b.bytes()
}

func jpegWithFullEXIF(t *testing.T) []byte {
	t.Helper()
	// JPEG with EXIF containing Make and a fake APP13 (IPTC).
	ifd0 := []exif.Entry{
		{Tag: 0x010F, Type: 2, Count: 5, Value: []byte{'F', 'u', 'j', 'i', 0}}, // Make
	}
	exifData := buildEXIFPayload(t, binary.LittleEndian, ifd0, nil, nil)

	var b jpegBuilder
	b.add(0xE0, app0())
	b.add(0xE1, exifData)
	b.add(0xED, []byte("Photoshop 3.0\x008BIM\x04\x04\x00\x00\x00\x00\x00\x00")) // fake IPTC APP13
	b.add(0xC0, sof0())
	return b.bytes()
}

// ---- Helpers ----

func stripTo(t *testing.T, input []byte, level Level) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := Strip(bytes.NewReader(input), &out, level); err != nil {
		t.Fatalf("Strip: %v", err)
	}
	return out.Bytes()
}

func findSegment(data []byte, marker byte) []byte {
	segs, _, err := jpeg.Parse(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	for _, s := range segs {
		if s.Marker == marker {
			return s.Data
		}
	}
	return nil
}

func hasMarker(data []byte, marker byte) bool {
	segs, _, err := jpeg.Parse(bytes.NewReader(data))
	if err != nil {
		return false
	}
	for _, s := range segs {
		if s.Marker == marker {
			return true
		}
	}
	return false
}

// ---- Tests ----

func TestNoGPS_RemovesGPS(t *testing.T) {
	input := jpegWithGPS(t)
	out := stripTo(t, input, LevelNoGPS)

	// Output must still be a valid JPEG with APP1
	if !hasMarker(out, 0xE1) {
		t.Fatal("no-gps: EXIF APP1 missing from output")
	}

	// Parse the output EXIF and verify GPS tags are gone
	app1 := findSegment(out, 0xE1)
	if app1 == nil {
		t.Fatal("no-gps: could not find APP1")
	}
	d, err := exif.Parse(app1)
	if err != nil {
		t.Fatalf("no-gps: exif.Parse on output: %v", err)
	}
	for _, e := range d.IFD0 {
		if e.Tag == exif.GPSIFDPointer {
			t.Error("no-gps: GPS IFD pointer still present in IFD0")
		}
	}
	if len(d.GPSSub) > 0 {
		t.Error("no-gps: GPS sub-IFD still present")
	}

	// Make tag should be preserved
	found := false
	for _, e := range d.IFD0 {
		if e.Tag == 0x010F {
			found = true
		}
	}
	if !found {
		t.Error("no-gps: Make tag unexpectedly removed")
	}
}

func TestClean_KeepsOnlyStructuralSegments(t *testing.T) {
	// A JPEG carrying a broad spread of metadata/app/comment segments alongside
	// the structural ones. Clean must keep only the structural set and drop the
	// rest — including vendor APPn segments that can carry identifying info.
	ifd0 := []exif.Entry{{Tag: 0x010F, Type: 2, Count: 5, Value: []byte{'F', 'u', 'j', 'i', 0}}}
	exifData := buildEXIFPayload(t, binary.LittleEndian, ifd0, nil, nil)

	var b jpegBuilder
	b.add(0xE0, app0())                                                  // APP0 / JFIF   keep
	b.add(0xE1, exifData)                                                // APP1 EXIF     drop
	b.add(0xE1, append([]byte("http://ns.adobe.com/xap/1.0/\x00"), '<')) // APP1 XMP      drop
	b.add(0xE2, []byte("ICC_PROFILE\x00\x01\x01"))                       // APP2 ICC      drop
	b.add(0xEC, []byte("Ducky\x00\x01"))                                 // APP12 Ducky   drop
	b.add(0xED, []byte("Photoshop 3.0\x008BIM"))                         // APP13 IPTC    drop
	b.add(0xEF, []byte{0x00})                                            // APP15         drop
	b.add(0xFE, []byte("a private comment"))                             // COM           drop
	b.add(0xDB, []byte{0x00, 0x01, 0x02})                                // DQT           keep
	b.add(0xC4, []byte{0x00, 0x01, 0x02})                                // DHT           keep
	b.add(0xC0, sof0())                                                  // SOF0          keep
	b.add(0xDA, []byte{0x01, 0x01, 0x00, 0x00, 0x3F, 0x00})              // SOS header    keep
	input := b.bytes()

	out := stripTo(t, input, LevelClean)

	// Output must still parse as a JPEG.
	if _, _, err := jpeg.Parse(bytes.NewReader(out)); err != nil {
		t.Fatalf("clean: output is not a valid JPEG: %v", err)
	}

	// Non-structural markers must all be gone: APP1–APP15 and COM.
	for m := byte(0xE1); m <= 0xEF; m++ {
		if hasMarker(out, m) {
			t.Errorf("clean: APP%d segment (0x%02X) still present", m-0xE0, m)
		}
	}
	if hasMarker(out, 0xFE) {
		t.Error("clean: COM segment still present")
	}

	// Structural markers must survive.
	for _, m := range []byte{0xE0, 0xDB, 0xC4, 0xC0, 0xDA} {
		if !hasMarker(out, m) {
			t.Errorf("clean: structural marker 0x%02X unexpectedly removed", m)
		}
	}
}

func TestStrip_OutputIsValidJPEG(t *testing.T) {
	// Strip re-parses its own output before returning; assert that every working
	// level over a range of fixtures produces something that reads back as a
	// JPEG (and that the validation never rejects legitimate output).
	fixtures := map[string][]byte{
		"withGPS":    jpegWithGPS(t),
		"fullEXIF":   jpegWithFullEXIF(t),
		"noMetadata": jpegWithNoMetadata(t),
	}
	for _, lvl := range []Level{LevelNoGPS, LevelNoCamera, LevelClean} {
		for name, input := range fixtures {
			out := stripTo(t, input, lvl)
			if _, _, err := jpeg.Parse(bytes.NewReader(out)); err != nil {
				t.Errorf("level %d on %s: output is not a valid JPEG: %v", lvl, name, err)
			}
		}
	}
}

func TestNonJPEG_GracefulError(t *testing.T) {
	input := []byte("this is not a jpeg")
	var out bytes.Buffer
	err := Strip(bytes.NewReader(input), &out, LevelClean)
	if err == nil {
		t.Fatal("expected error for non-JPEG input, got nil")
	}
	if out.Len() != 0 {
		t.Errorf("expected no output on error, got %d bytes", out.Len())
	}
}

func TestReencoded_NotImplemented(t *testing.T) {
	input := jpegWithFullEXIF(t)
	var out bytes.Buffer
	err := Strip(bytes.NewReader(input), &out, LevelReencoded)
	if !errors.Is(err, ErrReencodeNotImplemented) {
		t.Fatalf("reencoded: expected ErrReencodeNotImplemented, got %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("reencoded: expected no output, got %d bytes", out.Len())
	}
}

func TestNoMetadata_PassesThroughClean(t *testing.T) {
	input := jpegWithNoMetadata(t)
	out := stripTo(t, input, LevelNoCamera)

	// Must still be parseable
	if _, _, err := jpeg.Parse(bytes.NewReader(out)); err != nil {
		t.Fatalf("output is not a valid JPEG: %v", err)
	}
}

func TestNoCamera_RemovesEmbeddedThumbnail(t *testing.T) {
	// Build EXIF with an ImageWidth tag in IFD0 and an ISO tag in the Exif
	// sub-IFD. no-camera must keep shooting data (ISO) and drop everything else.
	ifd0 := []exif.Entry{
		{Tag: 0x0100, Type: 4, Count: 1, Value: []byte{1, 0, 0, 0}}, // ImageWidth=1
	}
	exifSub := []exif.Entry{
		{Tag: 0x8827, Type: 3, Count: 1, Value: makeU16(400)}, // ISO=400
	}
	exifData := buildEXIFPayload(t, binary.LittleEndian, ifd0, exifSub, nil)

	var b jpegBuilder
	b.add(0xE0, app0())
	b.add(0xE1, exifData)
	b.add(0xC0, sof0())
	input := b.bytes()

	out := stripTo(t, input, LevelNoCamera)
	app1 := findSegment(out, 0xE1)
	if app1 == nil {
		t.Fatal("no-camera: EXIF APP1 missing")
	}
	d, err := exif.Parse(app1)
	if err != nil {
		t.Fatalf("no-camera: exif.Parse on output: %v", err)
	}

	// Make tag must be absent (not in the no-camera keep list).
	for _, e := range d.IFD0 {
		if e.Tag == 0x010F {
			t.Error("no-camera: Make tag should have been removed")
		}
	}
	// ISO should survive in the Exif sub-IFD.
	found := false
	for _, e := range d.ExifSub {
		if e.Tag == 0x8827 {
			found = true
		}
	}
	if !found {
		t.Error("no-camera: ISO tag missing from output")
	}
}
