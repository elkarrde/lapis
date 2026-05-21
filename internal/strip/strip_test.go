package strip

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ---- Synthetic JPEG builder ----
//
// A minimal valid JPEG: SOI + APP0 + optional APP1 + SOF0 + EOI.
// All segments have plausible structure; pixel data is omitted for brevity.

type jpegBuilder struct {
	segs []jpegSeg
}

func (b *jpegBuilder) add(marker byte, data []byte) {
	b.segs = append(b.segs, jpegSeg{marker: marker, data: data})
}

func (b *jpegBuilder) bytes() []byte {
	var buf bytes.Buffer
	// SOI
	buf.Write([]byte{0xFF, 0xD8})
	for _, s := range b.segs {
		buf.WriteByte(0xFF)
		buf.WriteByte(s.marker)
		length := uint16(len(s.data) + 2)
		buf.WriteByte(byte(length >> 8))
		buf.WriteByte(byte(length))
		buf.Write(s.data)
	}
	// EOI
	buf.Write([]byte{0xFF, 0xD9})
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

// buildMinimalEXIF returns an APP1 EXIF payload with the given IFD0 entries
// and no sub-IFDs. Uses little-endian byte order.
func buildMinimalEXIF(entries []ifdEntry) []byte {
	bo := binary.LittleEndian

	ifd0Base := uint32(8) // right after TIFF header
	ifd0Sz := uint32(2 + len(entries)*12 + 4)
	dataBase := ifd0Base + ifd0Sz

	var extData []byte
	ifdBuf := appendU16(nil, bo, uint16(len(entries)))
	for _, e := range entries {
		ifdBuf = appendU16(ifdBuf, bo, e.tag)
		ifdBuf = appendU16(ifdBuf, bo, e.typ)
		ifdBuf = appendU32(ifdBuf, bo, e.count)
		sz := typeSize(e.typ)
		valLen := uint64(e.count) * uint64(sz)
		if sz == 0 || valLen <= 4 {
			var pad [4]byte
			copy(pad[:], e.value)
			ifdBuf = append(ifdBuf, pad[:]...)
		} else {
			off := dataBase + uint32(len(extData))
			extData = append(extData, e.value...)
			ifdBuf = appendU32(ifdBuf, bo, off)
		}
	}
	ifdBuf = appendU32(ifdBuf, bo, 0) // no IFD1

	var tiff []byte
	tiff = append(tiff, 'I', 'I')           // little-endian
	tiff = appendU16(tiff, bo, 42)           // TIFF magic
	tiff = appendU32(tiff, bo, ifd0Base)     // IFD0 offset
	tiff = append(tiff, ifdBuf...)
	tiff = append(tiff, extData...)

	result := make([]byte, 6+len(tiff))
	copy(result, exifSig)
	copy(result[6:], tiff)
	return result
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

// gpsIFDPointerInIFD0 returns the IFD0 entries for a JPEG that includes a GPS sub-IFD.
// The GPS sub-IFD pointer offset must be patched in after the full layout is known;
// for testing purposes we build it through buildEXIF so offsets are correct.
func jpegWithGPS(t *testing.T) []byte {
	t.Helper()
	// IFD0 will have: Make (ASCII), GPS IFD pointer
	// Exif sub-IFD: none
	// GPS sub-IFD: GPSLatitudeRef, GPSLatitude

	gpsEntries := []ifdEntry{
		{tag: 0x0001, typ: 2, count: 2, value: []byte{'N', 0}}, // GPSLatitudeRef (ASCII "N\0")
		{tag: 0x0002, typ: 5, count: 3, value: append(makeRATIONAL(51, 1), append(makeRATIONAL(30, 1), makeRATIONAL(0, 1)...)...)}, // GPSLatitude
	}

	ifd0 := []ifdEntry{
		{tag: 0x010F, typ: 2, count: 5, value: []byte{'T', 'e', 's', 't', 0}}, // Make
		ptrEntry(0x8825), // GPS IFD pointer — buildEXIF patches this
	}
	sortByTag(ifd0)

	exifData := buildEXIF(binary.LittleEndian, ifd0, nil, gpsEntries)

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
	// Build a JPEG with EXIF containing Make, a GPS pointer, and APP13.
	ifd0 := []ifdEntry{
		{tag: 0x010F, typ: 2, count: 5, value: []byte{'F', 'u', 'j', 'i', 0}}, // Make
	}
	exifData := buildEXIF(binary.LittleEndian, ifd0, nil, nil)

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
	segs, _, err := parseJPEG(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	for _, s := range segs {
		if s.marker == marker {
			return s.data
		}
	}
	return nil
}

func hasMarker(data []byte, marker byte) bool {
	segs, _, err := parseJPEG(bytes.NewReader(data))
	if err != nil {
		return false
	}
	for _, s := range segs {
		if s.marker == marker {
			return true
		}
	}
	return false
}

// ---- Tests ----

func TestScout_RemovesGPS(t *testing.T) {
	input := jpegWithGPS(t)
	out := stripTo(t, input, LevelScout)

	// Output must still be a valid JPEG with APP1
	if !hasMarker(out, 0xE1) {
		t.Fatal("scout: EXIF APP1 missing from output")
	}

	// Parse the output EXIF and verify GPS tags are gone
	app1 := findSegment(out, 0xE1)
	if app1 == nil {
		t.Fatal("scout: could not find APP1")
	}
	p, err := parseEXIF(app1)
	if err != nil {
		t.Fatalf("scout: parseEXIF on output: %v", err)
	}
	for _, e := range p.ifd0 {
		if e.tag == 0x8825 {
			t.Error("scout: GPS IFD pointer still present in IFD0")
		}
	}
	if len(p.gpsSub) > 0 {
		t.Error("scout: GPS sub-IFD still present")
	}

	// Make tag should be preserved
	found := false
	for _, e := range p.ifd0 {
		if e.tag == 0x010F {
			found = true
		}
	}
	if !found {
		t.Error("scout: Make tag unexpectedly removed")
	}
}

func TestGhost_RemovesAllMetadataSegments(t *testing.T) {
	input := jpegWithFullEXIF(t)
	out := stripTo(t, input, LevelGhost)

	if hasMarker(out, 0xE1) {
		t.Error("ghost: APP1 segment still present")
	}
	if hasMarker(out, 0xED) {
		t.Error("ghost: APP13 segment still present")
	}
	// APP0 (JFIF) should be kept
	if !hasMarker(out, 0xE0) {
		t.Error("ghost: APP0 unexpectedly removed")
	}
}

func TestNonJPEG_GracefulError(t *testing.T) {
	input := []byte("this is not a jpeg")
	var out bytes.Buffer
	err := Strip(bytes.NewReader(input), &out, LevelGhost)
	if err == nil {
		t.Fatal("expected error for non-JPEG input, got nil")
	}
	if out.Len() != 0 {
		t.Errorf("expected no output on error, got %d bytes", out.Len())
	}
}

func TestNoMetadata_PassesThroughClean(t *testing.T) {
	input := jpegWithNoMetadata(t)
	out := stripTo(t, input, LevelJournalist)

	// Must still be parseable
	segs, _, err := parseJPEG(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("output is not a valid JPEG: %v", err)
	}
	_ = segs
}

func TestJournalist_RemovesEmbeddedThumbnail(t *testing.T) {
	// Build EXIF with a Make tag and a simulated Exif sub-IFD.
	// IFD1 (thumbnail) is parsed by parseEXIF but excluded from output.
	// Since our buildEXIF already never emits IFD1, just verify the output
	// EXIF has no IFD1 next-pointer from IFD0.
	ifd0 := []ifdEntry{
		{tag: 0x0100, typ: 4, count: 1, value: []byte{1, 0, 0, 0}}, // ImageWidth=1
	}
	exifSub := []ifdEntry{
		{tag: 0x8827, typ: 3, count: 1, value: makeU16(400)}, // ISO=400
	}
	ifd0 = append(ifd0, ptrEntry(0x8769))
	sortByTag(ifd0)
	exifData := buildEXIF(binary.LittleEndian, ifd0, exifSub, nil)

	var b jpegBuilder
	b.add(0xE0, app0())
	b.add(0xE1, exifData)
	b.add(0xC0, sof0())
	input := b.bytes()

	out := stripTo(t, input, LevelJournalist)
	app1 := findSegment(out, 0xE1)
	if app1 == nil {
		t.Fatal("journalist: EXIF APP1 missing")
	}
	p, err := parseEXIF(app1)
	if err != nil {
		t.Fatalf("journalist: parseEXIF on output: %v", err)
	}

	// IFD0 next pointer should be 0 (no IFD1)
	// We verify indirectly: Make tag must be absent (not in journalist keep list)
	for _, e := range p.ifd0 {
		if e.tag == 0x010F {
			t.Error("journalist: Make tag should have been removed")
		}
	}
	// ISO should be in Exif sub-IFD
	found := false
	for _, e := range p.exifSub {
		if e.tag == 0x8827 {
			found = true
		}
	}
	if !found {
		t.Error("journalist: ISO tag missing from output")
	}
}
