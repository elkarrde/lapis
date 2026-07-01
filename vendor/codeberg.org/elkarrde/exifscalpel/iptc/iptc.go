// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package iptc performs surgical edits on the IPTC-IIM metadata carried in a
// JPEG APP13 (Photoshop Image Resource) segment. It operates on the segment
// payload (the bytes beginning with the "Photoshop 3.0\0" signature) and does
// not import the jpeg package, so it stays independently usable and testable.
//
// APP13 is a nested container: after the signature comes a sequence of Image
// Resource Blocks (IRBs), each "8BIM" + resource ID + Pascal name + size +
// data. The IPTC-IIM datasets live inside the single IRB with resource ID
// 0x0404; every other block (thumbnail 0x040C, ICC, embedded EXIF, ...) is
// retained verbatim. Inside 0x0404 are IIM datasets: 0x1C + record + dataset +
// length + value.
//
// Editing rebuilds the payload (length may change) rather than patching in
// place: APP13 is not referenced by any TIFF/JPEG offset, so recomputing the
// 0x0404 block size and even-padding on Build is safe. Which datasets count as
// "location" (or anything else) is policy and belongs in the consumer; this
// package removes datasets by record:number and preserves the rest byte for
// byte.
//
// All multi-byte fields are big-endian (no byte-order ambiguity).
package iptc

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// photoshopSig is the signature that prefixes a Photoshop APP13 payload. A
// payload lacking it is not a Photoshop resource block and Parse rejects it so
// the consumer can leave the segment untouched.
var photoshopSig = []byte("Photoshop 3.0\x00")

const (
	irbSig       = "8BIM" // Image Resource Block signature
	resIDIPTC    = 0x0404 // IPTC-IIM Image Resource Block
	iimTagMarker = 0x1C   // marks the start of an IIM dataset
	iimExtFlag   = 0x8000 // high bit of a dataset length => extended length
	iimStdMax    = 0x7fff // largest value length encodable in a standard (2-byte) length
)

// Dataset is a single IPTC-IIM dataset: its record:number key and raw value.
// Record 2 (application) holds the descriptive/location fields; the value is
// stored verbatim (no charset interpretation).
type Dataset struct {
	Record uint8
	Number uint8
	Value  []byte
}

// block is one Photoshop Image Resource Block. For every block except the
// IPTC-IIM block (0x0404) the resource data is retained verbatim in data; the
// IPTC-IIM block's datasets are parsed into datasets and reserialized on Build.
type block struct {
	resourceID uint16
	name       []byte // raw Pascal name field incl. length byte, already even-padded
	data       []byte // raw resource data (non-IPTC blocks only)
	isIPTC     bool
	datasets   []Dataset
}

// Data is a parsed Photoshop APP13 payload.
type Data struct {
	header []byte // "Photoshop 3.0\0", retained verbatim
	blocks []block
}

// Parse reads a Photoshop APP13 payload into resource blocks, decoding the
// IPTC-IIM datasets of the 0x0404 block. It returns an error for a payload that
// is not a well-formed Photoshop resource sequence (wrong signature, truncated,
// or unparseable IIM) so the consumer can leave such a segment untouched rather
// than risk corrupting it.
func Parse(payload []byte) (*Data, error) {
	if !bytes.HasPrefix(payload, photoshopSig) {
		return nil, fmt.Errorf("iptc: not a Photoshop APP13 segment")
	}

	d := &Data{header: append([]byte(nil), photoshopSig...)}
	p := payload[len(photoshopSig):]

	for len(p) >= 4 && string(p[:4]) == irbSig {
		p = p[4:]

		if len(p) < 2 {
			return nil, fmt.Errorf("iptc: truncated resource id")
		}
		resID := binary.BigEndian.Uint16(p)
		p = p[2:]

		// Pascal name: length byte + name bytes, padded so the whole field
		// (including the length byte) is an even number of bytes.
		if len(p) < 1 {
			return nil, fmt.Errorf("iptc: truncated resource name")
		}
		nameField := 1 + int(p[0])
		if nameField%2 != 0 {
			nameField++
		}
		if len(p) < nameField {
			return nil, fmt.Errorf("iptc: truncated resource name")
		}
		name := append([]byte(nil), p[:nameField]...)
		p = p[nameField:]

		if len(p) < 4 {
			return nil, fmt.Errorf("iptc: truncated resource size")
		}
		size := int(binary.BigEndian.Uint32(p))
		p = p[4:]
		if size < 0 || len(p) < size {
			return nil, fmt.Errorf("iptc: resource data overruns segment")
		}
		data := p[:size]
		// Resource data is even-padded; the pad byte may be absent if this is
		// the final block and the writer omitted it.
		adv := size
		if size%2 != 0 && len(p) > size {
			adv = size + 1
		}
		p = p[adv:]

		b := block{resourceID: resID, name: name}
		if resID == resIDIPTC {
			b.isIPTC = true
			ds, err := parseDatasets(data)
			if err != nil {
				return nil, err
			}
			b.datasets = ds
		} else {
			b.data = append([]byte(nil), data...)
		}
		d.blocks = append(d.blocks, b)
	}

	// Anything left over must be benign trailing padding; non-zero trailing
	// bytes mean the payload is not a structure we understand, so bail safely.
	for _, c := range p {
		if c != 0 {
			return nil, fmt.Errorf("iptc: unexpected trailing data after resource blocks")
		}
	}

	return d, nil
}

// parseDatasets decodes the IIM dataset stream inside a 0x0404 resource block.
func parseDatasets(data []byte) ([]Dataset, error) {
	var out []Dataset
	i := 0
	for i < len(data) {
		if data[i] != iimTagMarker {
			return nil, fmt.Errorf("iptc: expected IIM tag marker 0x1C at offset %d", i)
		}
		if i+5 > len(data) {
			return nil, fmt.Errorf("iptc: truncated IIM dataset header")
		}
		record := data[i+1]
		number := data[i+2]
		length := int(binary.BigEndian.Uint16(data[i+3:]))
		i += 5

		if length&iimExtFlag != 0 {
			// Extended length: the low 15 bits give the count of bytes that
			// hold the real length.
			n := length &^ iimExtFlag
			if n == 0 || n > 4 || i+n > len(data) {
				return nil, fmt.Errorf("iptc: unsupported extended IIM length")
			}
			length = 0
			for j := 0; j < n; j++ {
				length = length<<8 | int(data[i+j])
			}
			i += n
		}

		if length < 0 || i+length > len(data) {
			return nil, fmt.Errorf("iptc: IIM dataset value overruns block")
		}
		out = append(out, Dataset{
			Record: record,
			Number: number,
			Value:  append([]byte(nil), data[i:i+length]...),
		})
		i += length
	}
	return out, nil
}

// Datasets returns the IIM datasets of the IPTC-IIM (0x0404) block in order, or
// nil if the payload carries no IPTC-IIM block. The returned slice aliases the
// internal storage; use Remove to mutate.
func (d *Data) Datasets() []Dataset {
	for _, b := range d.blocks {
		if b.isIPTC {
			return b.datasets
		}
	}
	return nil
}

// Remove deletes every IIM dataset matching record:number and reports how many
// were removed (0 if none, e.g. no IPTC-IIM block or no match). Repeatable
// datasets such as 2:26/2:27 are all removed.
func (d *Data) Remove(record, number uint8) int {
	removed := 0
	for i := range d.blocks {
		if !d.blocks[i].isIPTC {
			continue
		}
		kept := d.blocks[i].datasets[:0]
		for _, ds := range d.blocks[i].datasets {
			if ds.Record == record && ds.Number == number {
				removed++
				continue
			}
			kept = append(kept, ds)
		}
		d.blocks[i].datasets = kept
	}
	return removed
}

// Empty reports whether the payload would serialize to no resource blocks: the
// IPTC-IIM block is empty (or absent) and there are no other blocks. When true,
// the consumer should drop the whole APP13 segment rather than emit a bare
// "Photoshop 3.0\0" header.
func (d *Data) Empty() bool {
	for _, b := range d.blocks {
		if b.isIPTC {
			if len(b.datasets) > 0 {
				return false
			}
			continue
		}
		return false
	}
	return true
}

// Build reserializes the payload: the "Photoshop 3.0\0" header followed by each
// resource block, recomputing the IPTC-IIM block's size and even-padding. An
// IPTC-IIM block left with no datasets is dropped entirely; all other blocks
// are emitted verbatim.
func (d *Data) Build() ([]byte, error) {
	out := append([]byte(nil), d.header...)
	for _, b := range d.blocks {
		var data []byte
		if b.isIPTC {
			if len(b.datasets) == 0 {
				continue // drop empty IPTC-IIM block
			}
			data = buildDatasets(b.datasets)
		} else {
			data = b.data
		}

		out = append(out, irbSig...)
		out = binary.BigEndian.AppendUint16(out, b.resourceID)
		out = append(out, b.name...) // already even-padded
		out = binary.BigEndian.AppendUint32(out, uint32(len(data)))
		out = append(out, data...)
		if len(data)%2 != 0 {
			out = append(out, 0) // even-pad resource data
		}
	}
	return out, nil
}

// buildDatasets serializes IIM datasets, choosing the standard 2-byte length
// when the value fits and a 4-byte extended length otherwise.
func buildDatasets(ds []Dataset) []byte {
	var out []byte
	for _, x := range ds {
		out = append(out, iimTagMarker, x.Record, x.Number)
		n := len(x.Value)
		if n <= iimStdMax {
			out = binary.BigEndian.AppendUint16(out, uint16(n))
		} else {
			out = binary.BigEndian.AppendUint16(out, iimExtFlag|4) // extended, 4-byte length
			out = binary.BigEndian.AppendUint32(out, uint32(n))
		}
		out = append(out, x.Value...)
	}
	return out
}
