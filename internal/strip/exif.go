// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package strip

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// typeSizes maps TIFF type codes (1-12) to bytes-per-value.
var typeSizes = [13]uint32{0, 1, 1, 2, 4, 8, 1, 1, 2, 4, 8, 4, 8}

func typeSize(t uint16) uint32 {
	if int(t) < len(typeSizes) {
		return typeSizes[t]
	}
	return 0
}

// ifdEntry is an EXIF IFD entry with its value fully resolved (not an offset).
type ifdEntry struct {
	tag   uint16
	typ   uint16
	count uint32
	value []byte // always the actual value bytes, never an offset
}

type parsedEXIF struct {
	bo      binary.ByteOrder
	ifd0    []ifdEntry
	exifSub []ifdEntry
	gpsSub  []ifdEntry
}

// parseEXIF parses EXIF APP1 payload (starting with "Exif\0\0").
// It resolves all IFD entry values from their offsets.
func parseEXIF(data []byte) (*parsedEXIF, error) {
	if !bytes.HasPrefix(data, exifSig) {
		return nil, fmt.Errorf("missing Exif header")
	}
	tiff := data[6:]
	if len(tiff) < 8 {
		return nil, fmt.Errorf("EXIF too short")
	}

	var bo binary.ByteOrder
	switch {
	case tiff[0] == 'I' && tiff[1] == 'I':
		bo = binary.LittleEndian
	case tiff[0] == 'M' && tiff[1] == 'M':
		bo = binary.BigEndian
	default:
		return nil, fmt.Errorf("invalid TIFF byte order")
	}
	if bo.Uint16(tiff[2:]) != 42 {
		return nil, fmt.Errorf("invalid TIFF magic")
	}

	ifd0Off := bo.Uint32(tiff[4:])

	readIFD := func(offset uint32) ([]ifdEntry, uint32, error) {
		if uint64(offset)+2 > uint64(len(tiff)) {
			return nil, 0, fmt.Errorf("IFD offset %d out of range", offset)
		}
		n := int(bo.Uint16(tiff[offset:]))
		base := int(offset) + 2
		entries := make([]ifdEntry, 0, n)
		for i := 0; i < n; i++ {
			pos := base + i*12
			if pos+12 > len(tiff) {
				return nil, 0, fmt.Errorf("IFD entry %d out of range", i)
			}
			e := tiff[pos : pos+12]
			tag := bo.Uint16(e[0:])
			typ := bo.Uint16(e[2:])
			cnt := bo.Uint32(e[4:])
			sz := typeSize(typ)

			var val []byte
			valLen := uint64(cnt) * uint64(sz)
			if sz == 0 || valLen <= 4 {
				cp := valLen
				if sz == 0 {
					cp = 4 // unknown type: copy 4 raw bytes
				}
				val = make([]byte, cp)
				copy(val, e[8:])
			} else {
				off := uint64(bo.Uint32(e[8:]))
				end := off + valLen
				if end > uint64(len(tiff)) {
					end = uint64(len(tiff))
				}
				if off > uint64(len(tiff)) {
					val = nil
				} else {
					val = make([]byte, end-off)
					copy(val, tiff[off:])
				}
			}
			entries = append(entries, ifdEntry{tag: tag, typ: typ, count: cnt, value: val})
		}
		nextOff := uint32(0)
		tail := base + n*12
		if tail+4 <= len(tiff) {
			nextOff = bo.Uint32(tiff[tail:])
		}
		return entries, nextOff, nil
	}

	ifd0, ifd1Off, err := readIFD(ifd0Off)
	if err != nil {
		return nil, fmt.Errorf("IFD0: %w", err)
	}

	p := &parsedEXIF{bo: bo, ifd0: ifd0}

	for _, e := range ifd0 {
		if len(e.value) < 4 {
			continue
		}
		switch e.tag {
		case 0x8769: // ExifIFD pointer
			if sub, _, err := readIFD(bo.Uint32(e.value)); err == nil {
				p.exifSub = sub
			}
		case 0x8825: // GPSIFD pointer
			if sub, _, err := readIFD(bo.Uint32(e.value)); err == nil {
				p.gpsSub = sub
			}
		}
	}

	// IFD1 (thumbnail) is intentionally skipped: its entries reference raw thumbnail
	// bytes by offset, which cannot be safely relocated without specialized handling.
	// IFD1 is excluded from all output levels.
	_ = ifd1Off

	return p, nil
}

// ---- EXIF rebuilding ----

// ptrEntry creates a LONG IFD entry for a sub-IFD pointer.
// buildEXIF will overwrite its value with the correct offset.
func ptrEntry(tag uint16) ifdEntry {
	return ifdEntry{tag: tag, typ: 4, count: 1, value: make([]byte, 4)}
}

func sortByTag(entries []ifdEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].tag < entries[j].tag })
}

func filterKeep(entries []ifdEntry, keep map[uint16]bool) []ifdEntry {
	out := entries[:0:0]
	for _, e := range entries {
		if keep[e.tag] {
			out = append(out, e)
		}
	}
	return out
}

func filterRemove(entries []ifdEntry, remove map[uint16]bool) []ifdEntry {
	out := entries[:0:0]
	for _, e := range entries {
		if !remove[e.tag] {
			out = append(out, e)
		}
	}
	return out
}

// appendU16 / appendU32 append a value in the given byte order.
func appendU16(b []byte, bo binary.ByteOrder, v uint16) []byte {
	var tmp [2]byte
	putU16(tmp[:], bo, v)
	return append(b, tmp[:]...)
}

func appendU32(b []byte, bo binary.ByteOrder, v uint32) []byte {
	var tmp [4]byte
	putU32(tmp[:], bo, v)
	return append(b, tmp[:]...)
}

func putU16(b []byte, bo binary.ByteOrder, v uint16) {
	if bo == binary.LittleEndian {
		binary.LittleEndian.PutUint16(b, v)
	} else {
		binary.BigEndian.PutUint16(b, v)
	}
}

func putU32(b []byte, bo binary.ByteOrder, v uint32) {
	if bo == binary.LittleEndian {
		binary.LittleEndian.PutUint32(b, v)
	} else {
		binary.BigEndian.PutUint32(b, v)
	}
}

// buildEXIF serializes EXIF APP1 payload from the given IFD data.
// ifd0 must include pointer entries (0x8769, 0x8825) if the corresponding
// sub-IFDs are non-empty; buildEXIF will fill in the correct offsets.
//
// Layout: TIFF header | IFD0 | ExifSub | GPSSub | external value data
func buildEXIF(bo binary.ByteOrder, ifd0, exifSub, gpsSub []ifdEntry) []byte {
	const headerSize = 8

	ifd0Base := uint32(headerSize)
	ifd0Sz := uint32(2 + len(ifd0)*12 + 4)

	exifBase := uint32(0)
	exifSz := uint32(0)
	if len(exifSub) > 0 {
		exifBase = ifd0Base + ifd0Sz
		exifSz = uint32(2 + len(exifSub)*12 + 4)
	}

	gpsBase := uint32(0)
	gpsSz := uint32(0)
	if len(gpsSub) > 0 {
		gpsBase = ifd0Base + ifd0Sz + exifSz
		gpsSz = uint32(2 + len(gpsSub)*12 + 4)
	}
	_ = gpsSz

	dataBase := ifd0Base + ifd0Sz + exifSz + gpsSz

	// Patch sub-IFD pointer values in ifd0 now that offsets are known.
	for i := range ifd0 {
		switch ifd0[i].tag {
		case 0x8769:
			if exifBase > 0 {
				putU32(ifd0[i].value, bo, exifBase)
			}
		case 0x8825:
			if gpsBase > 0 {
				putU32(ifd0[i].value, bo, gpsBase)
			}
		}
	}

	// serIFD serializes a single IFD, appending external value bytes to extData.
	var extData []byte
	serIFD := func(entries []ifdEntry, nextIFD uint32) []byte {
		buf := appendU16(nil, bo, uint16(len(entries)))
		for _, e := range entries {
			buf = appendU16(buf, bo, e.tag)
			buf = appendU16(buf, bo, e.typ)
			buf = appendU32(buf, bo, e.count)
			sz := typeSize(e.typ)
			valLen := uint64(e.count) * uint64(sz)
			if sz == 0 || valLen <= 4 {
				var pad [4]byte
				copy(pad[:], e.value)
				buf = append(buf, pad[:]...)
			} else {
				off := dataBase + uint32(len(extData))
				extData = append(extData, e.value...)
				if len(extData)%2 != 0 { // word-align (TIFF requirement)
					extData = append(extData, 0)
				}
				buf = appendU32(buf, bo, off)
			}
		}
		buf = appendU32(buf, bo, nextIFD)
		return buf
	}

	var tiff []byte
	// TIFF header
	if bo == binary.LittleEndian {
		tiff = append(tiff, 'I', 'I')
	} else {
		tiff = append(tiff, 'M', 'M')
	}
	tiff = appendU16(tiff, bo, 42)
	tiff = appendU32(tiff, bo, ifd0Base)

	tiff = append(tiff, serIFD(ifd0, 0)...)
	if len(exifSub) > 0 {
		tiff = append(tiff, serIFD(exifSub, 0)...)
	}
	if len(gpsSub) > 0 {
		tiff = append(tiff, serIFD(gpsSub, 0)...)
	}
	tiff = append(tiff, extData...)

	result := make([]byte, 6+len(tiff))
	copy(result, exifSig)
	copy(result[6:], tiff)
	return result
}

// ---- Level-specific EXIF processors ----

// no-camera: keep only shooting data, nothing identifying.
var noCameraIFD0Keep = map[uint16]bool{
	0x0100: true, // ImageWidth
	0x0101: true, // ImageLength
	// 0x8769 ExifIFD pointer added below if needed
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
	p, err := parseEXIF(data)
	if err != nil {
		return nil, err
	}

	ifd0 := filterKeep(p.ifd0, noCameraIFD0Keep)
	exifSub := filterKeep(p.exifSub, noCameraExifSubKeep)

	if len(exifSub) > 0 {
		ifd0 = append(ifd0, ptrEntry(0x8769))
	}
	sortByTag(ifd0)
	sortByTag(exifSub)

	return buildEXIF(p.bo, ifd0, exifSub, nil), nil
}

// no-gps: remove GPS sub-IFD and any other unresolvable pointer sub-IFDs.
// All other IFD0 and Exif sub-IFD data is preserved.
var noGPSIFD0Remove = map[uint16]bool{
	0x8825: true, // GPSIFD pointer — drops all GPS data
	0xA005: true, // Interoperability IFD — pointer cannot be safely rewritten
	0x014A: true, // SubIFDs — same
}

func processNoGPSEXIF(data []byte) ([]byte, error) {
	p, err := parseEXIF(data)
	if err != nil {
		return nil, err
	}

	ifd0 := filterRemove(p.ifd0, noGPSIFD0Remove)
	// 0x8769 in ifd0 has a stale offset; buildEXIF patches it from the computed layout.
	sortByTag(ifd0)

	exifSub := p.exifSub
	sortByTag(exifSub)

	return buildEXIF(p.bo, ifd0, exifSub, nil), nil
}
