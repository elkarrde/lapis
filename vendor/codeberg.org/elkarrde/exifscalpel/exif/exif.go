// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package exif parses and edits the EXIF/TIFF block carried in a JPEG APP1
// segment. It operates on the segment payload (the bytes beginning with the
// "Exif\x00\x00" signature) and does not import the jpeg package, so it stays
// independently usable and testable.
//
// Two editing modes are offered, neither forced:
//
//   - Rebuild: Parse a payload into *Data, mutate it with Set/Remove/RemoveIFD,
//     then Build a fresh payload. The result may differ in length from the
//     input — correct when removing tags or whole IFDs.
//   - In-place: OverwriteValueInPlace patches an IFD0 entry's value bytes
//     within the original payload, NUL-padded to the original length, so the
//     payload size and every TIFF/JPEG offset are preserved. Suited to
//     scrubbing ASCII tags such as Software without moving offsets.
//
// The IFD engine is lifted from codeberg.org/elkarrde/lapis; the in-place mode
// is re-expressed from codeberg.org/elkarrde/tidy-exif.
package exif

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// Well-known IFD0 tags. SoftwareTag is the value Adobe tools write alongside
// the XMP signature; the pointer tags identify the sub-IFDs.
const (
	SoftwareTag    uint16 = 0x0131 // IFD0 "Software" (ASCII)
	ExifIFDPointer uint16 = 0x8769 // IFD0 -> Exif sub-IFD
	GPSIFDPointer  uint16 = 0x8825 // IFD0 -> GPS sub-IFD
)

// IFDID selects one of the parsed IFDs for the editing helpers.
type IFDID int

const (
	IFD0    IFDID = iota // primary image IFD
	ExifIFD              // Exif sub-IFD (via ExifIFDPointer)
	GPSIFD               // GPS sub-IFD (via GPSIFDPointer)
)

// exifSig is the prefix of an EXIF APP1 payload; the TIFF header follows it.
// Defined here (not imported from jpeg) so the package stands alone.
var exifSig = []byte("Exif\x00\x00")

// typeSizes maps TIFF type codes (1-12) to bytes-per-value.
var typeSizes = [13]uint32{0, 1, 1, 2, 4, 8, 1, 1, 2, 4, 8, 4, 8}

func typeSize(t uint16) uint32 {
	if int(t) < len(typeSizes) {
		return typeSizes[t]
	}
	return 0
}

// Entry is an EXIF IFD entry with its value fully resolved to bytes (never an
// offset). Count is the number of values; Value holds Count*sizeof(Type) bytes
// (or four raw bytes for an unknown Type).
type Entry struct {
	Tag, Type uint16
	Count     uint32
	Value     []byte
}

// Data is a parsed EXIF block: a byte order and the three IFDs the engine
// resolves. IFD1 (thumbnail) is intentionally dropped on parse — its entries
// reference raw thumbnail bytes by offset that cannot be safely relocated.
type Data struct {
	ByteOrder binary.ByteOrder
	IFD0      []Entry
	ExifSub   []Entry
	GPSSub    []Entry
}

// Parse reads an EXIF APP1 payload (starting with "Exif\x00\x00") and resolves
// every IFD entry's value from its inline bytes or external offset.
func Parse(payload []byte) (*Data, error) {
	if !bytes.HasPrefix(payload, exifSig) {
		return nil, fmt.Errorf("missing Exif header")
	}
	tiff := payload[6:]
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

	readIFD := func(offset uint32) ([]Entry, uint32, error) {
		if uint64(offset)+2 > uint64(len(tiff)) {
			return nil, 0, fmt.Errorf("IFD offset %d out of range", offset)
		}
		n := int(bo.Uint16(tiff[offset:]))
		base := int(offset) + 2
		entries := make([]Entry, 0, n)
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
			entries = append(entries, Entry{Tag: tag, Type: typ, Count: cnt, Value: val})
		}
		return entries, 0, nil
	}

	ifd0, _, err := readIFD(ifd0Off)
	if err != nil {
		return nil, fmt.Errorf("IFD0: %w", err)
	}

	d := &Data{ByteOrder: bo, IFD0: ifd0}

	for _, e := range ifd0 {
		if len(e.Value) < 4 {
			continue
		}
		switch e.Tag {
		case ExifIFDPointer:
			if sub, _, err := readIFD(bo.Uint32(e.Value)); err == nil {
				d.ExifSub = sub
			}
		case GPSIFDPointer:
			if sub, _, err := readIFD(bo.Uint32(e.Value)); err == nil {
				d.GPSSub = sub
			}
		}
	}

	return d, nil
}

// ifd returns a pointer to the slice backing the given IFD, or nil.
func (d *Data) ifd(id IFDID) *[]Entry {
	switch id {
	case IFD0:
		return &d.IFD0
	case ExifIFD:
		return &d.ExifSub
	case GPSIFD:
		return &d.GPSSub
	}
	return nil
}

// Find returns a pointer to the first entry with the given tag in the named
// IFD (so the caller may read or mutate it in place), and whether it exists.
func (d *Data) Find(id IFDID, tag uint16) (*Entry, bool) {
	s := d.ifd(id)
	if s == nil {
		return nil, false
	}
	for i := range *s {
		if (*s)[i].Tag == tag {
			return &(*s)[i], true
		}
	}
	return nil, false
}

// Set replaces the value of the tag's entry in the named IFD, updating Count to
// match (Count = len(value)/sizeof(Type)). If the tag is absent, a new ASCII
// (Type 2) entry is appended; construct the Entry directly for other types.
func (d *Data) Set(id IFDID, tag uint16, value []byte) {
	if e, ok := d.Find(id, tag); ok {
		e.Value = value
		if sz := typeSize(e.Type); sz > 0 {
			e.Count = uint32(len(value)) / sz
		} else {
			e.Count = uint32(len(value))
		}
		return
	}
	s := d.ifd(id)
	if s == nil {
		return
	}
	*s = append(*s, Entry{Tag: tag, Type: 2, Count: uint32(len(value)), Value: value})
}

// Remove deletes every entry with the given tag from the named IFD.
func (d *Data) Remove(id IFDID, tag uint16) {
	s := d.ifd(id)
	if s == nil {
		return
	}
	out := (*s)[:0:0]
	for _, e := range *s {
		if e.Tag != tag {
			out = append(out, e)
		}
	}
	*s = out
}

// RemoveIFD drops a whole IFD. For ExifIFD/GPSIFD it also removes the dangling
// pointer entry from IFD0 (e.g. RemoveIFD(GPSIFD) excises GPS wholesale).
func (d *Data) RemoveIFD(id IFDID) {
	switch id {
	case ExifIFD:
		d.ExifSub = nil
		d.Remove(IFD0, ExifIFDPointer)
	case GPSIFD:
		d.GPSSub = nil
		d.Remove(IFD0, GPSIFDPointer)
	case IFD0:
		d.IFD0 = nil
	}
}

// ptrEntry creates a LONG sub-IFD pointer entry; Build fills in its offset.
func ptrEntry(tag uint16) Entry {
	return Entry{Tag: tag, Type: 4, Count: 1, Value: make([]byte, 4)}
}

func sortByTag(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Tag < entries[j].Tag })
}

func hasTag(entries []Entry, tag uint16) bool {
	for _, e := range entries {
		if e.Tag == tag {
			return true
		}
	}
	return false
}

func withoutTag(entries []Entry, tag uint16) []Entry {
	out := entries[:0:0]
	for _, e := range entries {
		if e.Tag != tag {
			out = append(out, e)
		}
	}
	return out
}

// cloneEntries deep-copies entries (including Value) so Build never mutates the
// caller's *Data when it patches sub-IFD pointer offsets.
func cloneEntries(in []Entry) []Entry {
	out := make([]Entry, len(in))
	for i, e := range in {
		out[i] = e
		out[i].Value = append([]byte(nil), e.Value...)
	}
	return out
}

// Build serializes the EXIF APP1 payload from the current IFD data. The result
// may differ in length from the parsed input (rebuild mode). Sub-IFD pointer
// entries in IFD0 are reconciled automatically: added when the sub-IFD is
// non-empty, dropped when it is empty, and their offsets are computed here.
func (d *Data) Build() ([]byte, error) {
	if d.ByteOrder == nil {
		return nil, fmt.Errorf("nil byte order")
	}
	bo := d.ByteOrder

	ifd0 := cloneEntries(d.IFD0)
	exifSub := cloneEntries(d.ExifSub)
	gpsSub := cloneEntries(d.GPSSub)

	// Reconcile sub-IFD pointers against the actual sub-IFDs.
	for _, p := range []struct {
		tag uint16
		sub []Entry
	}{{ExifIFDPointer, exifSub}, {GPSIFDPointer, gpsSub}} {
		if len(p.sub) > 0 {
			if !hasTag(ifd0, p.tag) {
				ifd0 = append(ifd0, ptrEntry(p.tag))
			}
		} else {
			ifd0 = withoutTag(ifd0, p.tag)
		}
	}
	sortByTag(ifd0)
	sortByTag(exifSub)
	sortByTag(gpsSub)

	const headerSize = 8
	ifd0Base := uint32(headerSize)
	ifd0Sz := uint32(2 + len(ifd0)*12 + 4)

	exifBase, exifSz := uint32(0), uint32(0)
	if len(exifSub) > 0 {
		exifBase = ifd0Base + ifd0Sz
		exifSz = uint32(2 + len(exifSub)*12 + 4)
	}
	gpsBase := uint32(0)
	if len(gpsSub) > 0 {
		gpsBase = ifd0Base + ifd0Sz + exifSz
	}
	dataBase := ifd0Base + ifd0Sz + exifSz
	if len(gpsSub) > 0 {
		dataBase += uint32(2 + len(gpsSub)*12 + 4)
	}

	// Patch sub-IFD pointer values now that offsets are known.
	for i := range ifd0 {
		switch ifd0[i].Tag {
		case ExifIFDPointer:
			if exifBase > 0 {
				putU32(ifd0[i].Value, bo, exifBase)
			}
		case GPSIFDPointer:
			if gpsBase > 0 {
				putU32(ifd0[i].Value, bo, gpsBase)
			}
		}
	}

	var extData []byte
	serIFD := func(entries []Entry, nextIFD uint32) []byte {
		buf := appendU16(nil, bo, uint16(len(entries)))
		for _, e := range entries {
			buf = appendU16(buf, bo, e.Tag)
			buf = appendU16(buf, bo, e.Type)
			buf = appendU32(buf, bo, e.Count)
			sz := typeSize(e.Type)
			valLen := uint64(e.Count) * uint64(sz)
			if sz == 0 || valLen <= 4 {
				var pad [4]byte
				copy(pad[:], e.Value)
				buf = append(buf, pad[:]...)
			} else {
				off := dataBase + uint32(len(extData))
				extData = append(extData, e.Value...)
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
	return result, nil
}

// ---- In-place, length-preserving editing (from tidy-exif) ----

// OverwriteValueInPlace overwrites, within the original EXIF payload bytes, the
// value of the IFD0 entry identified by tag — NUL-padding replacement to the
// entry's existing value length and keeping a trailing NUL. It is
// length-preserving: the payload size and every TIFF/JPEG offset are unchanged.
// Suited to scrubbing ASCII tags such as Software (0x0131).
//
// It returns changed=false (err=nil) when the tag is absent or already equals
// the padded replacement. err is non-nil only when payload is not parseable
// EXIF or the located value range is out of bounds. replacement is truncated if
// it would not fit the original value region.
func OverwriteValueInPlace(payload []byte, tag uint16, replacement []byte) (changed bool, err error) {
	start, end, found, err := ifd0ValueRange(payload, tag)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	repl := replacement
	if len(repl) > (end-start)-1 { // leave room for a NUL terminator
		repl = repl[:(end-start)-1]
	}
	want := make([]byte, end-start)
	copy(want, repl)
	if bytes.Equal(payload[start:end], want) {
		return false, nil
	}
	copy(payload[start:end], want)
	return true, nil
}

// ReadValue returns the IFD0 entry's value bytes (NUL trimmed) from the payload,
// or "" if the tag is absent. A convenience for the in-place path.
func ReadValue(payload []byte, tag uint16) string {
	start, end, found, err := ifd0ValueRange(payload, tag)
	if err != nil || !found {
		return ""
	}
	v := payload[start:end]
	if i := bytes.IndexByte(v, 0); i >= 0 {
		v = v[:i]
	}
	return string(bytes.TrimSpace(v))
}

// ifd0ValueRange returns the byte range [start,end) within payload holding the
// IFD0 entry value for tag. found=false with err=nil means the tag is absent;
// err!=nil means the payload is not parseable EXIF or the range is invalid.
func ifd0ValueRange(payload []byte, tag uint16) (start, end int, found bool, err error) {
	order, ifd0, ok := tiffIFD0(payload)
	if !ok {
		return 0, 0, false, fmt.Errorf("not parseable EXIF")
	}
	tiff := len(exifSig)
	n := int(order.Uint16(payload[ifd0:]))
	for e := 0; e < n; e++ {
		off := ifd0 + 2 + e*12
		if off+12 > len(payload) {
			break
		}
		if order.Uint16(payload[off:]) != tag {
			continue
		}
		typ := order.Uint16(payload[off+2:])
		count := int(order.Uint32(payload[off+4:]))
		sz := int(typeSize(typ))
		if sz == 0 {
			return 0, 0, false, fmt.Errorf("tag 0x%04X has unknown type %d", tag, typ)
		}
		length := count * sz
		if length == 0 {
			return 0, 0, false, nil
		}
		if length <= 4 {
			start = off + 8
		} else {
			start = tiff + int(order.Uint32(payload[off+8:]))
		}
		end = start + length
		if start < tiff || end > len(payload) {
			return 0, 0, false, fmt.Errorf("tag 0x%04X value out of range", tag)
		}
		return start, end, true, nil
	}
	return 0, 0, false, nil
}

// tiffIFD0 locates the TIFF block in an EXIF payload and returns the byte order
// plus the absolute offset (within payload) of the IFD0 entry list.
func tiffIFD0(payload []byte) (order binary.ByteOrder, ifd0 int, ok bool) {
	if !bytes.HasPrefix(payload, exifSig) {
		return nil, 0, false
	}
	tiff := len(exifSig)
	if len(payload) < tiff+8 {
		return nil, 0, false
	}
	switch {
	case payload[tiff] == 'I' && payload[tiff+1] == 'I':
		order = binary.LittleEndian
	case payload[tiff] == 'M' && payload[tiff+1] == 'M':
		order = binary.BigEndian
	default:
		return nil, 0, false
	}
	if order.Uint16(payload[tiff+2:]) != 42 {
		return nil, 0, false
	}
	ifd0 = tiff + int(order.Uint32(payload[tiff+4:]))
	if ifd0 < tiff || ifd0+2 > len(payload) {
		return nil, 0, false
	}
	return order, ifd0, true
}

// ---- byte-order helpers ----

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

func putU16(b []byte, bo binary.ByteOrder, v uint16) { bo.PutUint16(b, v) }
func putU32(b []byte, bo binary.ByteOrder, v uint32) { bo.PutUint32(b, v) }
