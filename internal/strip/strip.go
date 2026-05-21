package strip

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Level controls how aggressively metadata is stripped.
type Level int

const (
	LevelScout      Level = iota + 1 // GPS and location data only
	LevelJournalist                  // all identifying metadata; keep shooting data
	LevelGhost                       // excise all EXIF and IPTC segments entirely
)

const ghostWarning = `WARNING: Pixel-level steganographic fingerprints are not addressed by --level ghost.
         Camera manufacturers (Canon, Nikon, Fuji) may embed invisible identifying
         patterns in image data. Use goindigo (planned) for full mitigation.`

// ParseLevel converts a string flag value to a Level.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "scout":
		return LevelScout, nil
	case "journalist":
		return LevelJournalist, nil
	case "ghost":
		return LevelGhost, nil
	default:
		return 0, fmt.Errorf("unknown level %q: must be scout, journalist, or ghost", s)
	}
}

// Strip reads a JPEG from r, applies the given stripping level, and writes
// the result to w. Returns an error if r is not a valid JPEG.
func Strip(r io.Reader, w io.Writer, level Level) error {
	segs, tail, err := parseJPEG(r)
	if err != nil {
		return err
	}
	out, err := processSegments(segs, level)
	if err != nil {
		return err
	}
	return writeJPEG(w, out, tail)
}

// ---- JPEG segment handling ----

var (
	exifSig = []byte{'E', 'x', 'i', 'f', 0, 0}
	xmpSig  = []byte("http://ns.adobe.com/xap/1.0/\x00")
)

type jpegSeg struct {
	marker byte
	data   []byte
}

func isExifSeg(s jpegSeg) bool {
	return s.marker == 0xE1 && bytes.HasPrefix(s.data, exifSig)
}

func isXMPSeg(s jpegSeg) bool {
	return s.marker == 0xE1 && bytes.HasPrefix(s.data, xmpSig)
}

// parseJPEG reads a complete JPEG and returns its segments plus the raw bytes
// from the SOS segment onward (the compressed image data and EOI).
func parseJPEG(r io.Reader) ([]jpegSeg, []byte, error) {
	all, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	if len(all) < 2 || all[0] != 0xFF || all[1] != 0xD8 {
		return nil, nil, fmt.Errorf("not a JPEG file")
	}

	var segs []jpegSeg
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
			segs = append(segs, jpegSeg{marker: m, data: payload})
			return segs, all[i+length:], nil
		}

		segs = append(segs, jpegSeg{marker: m, data: payload})
		i += length
	}
	return segs, nil, nil
}

// writeJPEG emits a JPEG from its segment list and the raw tail (image data + EOI).
func writeJPEG(w io.Writer, segs []jpegSeg, tail []byte) error {
	if _, err := w.Write([]byte{0xFF, 0xD8}); err != nil {
		return err
	}
	hdr := make([]byte, 4)
	for _, s := range segs {
		hdr[0] = 0xFF
		hdr[1] = s.marker
		binary.BigEndian.PutUint16(hdr[2:], uint16(len(s.data)+2))
		if _, err := w.Write(hdr); err != nil {
			return err
		}
		if _, err := w.Write(s.data); err != nil {
			return err
		}
	}
	_, err := w.Write(tail)
	return err
}

func processSegments(segs []jpegSeg, level Level) ([]jpegSeg, error) {
	var out []jpegSeg
	switch level {

	case LevelGhost:
		fmt.Fprintln(os.Stderr, ghostWarning)
		for _, s := range segs {
			// drop APP1 (EXIF and XMP) and APP13 (IPTC)
			if s.marker == 0xE1 || s.marker == 0xED {
				continue
			}
			out = append(out, s)
		}

	case LevelJournalist:
		for _, s := range segs {
			switch {
			case s.marker == 0xED: // APP13 / IPTC — excise entirely
			case isXMPSeg(s): // XMP APP1 — excise
			case isExifSeg(s):
				newData, err := processJournalistEXIF(s.data)
				if err == nil {
					out = append(out, jpegSeg{marker: 0xE1, data: newData})
				}
				// on parse failure, drop segment rather than expose raw data
			default:
				out = append(out, s)
			}
		}

	case LevelScout:
		for _, s := range segs {
			if isExifSeg(s) {
				newData, err := processScoutEXIF(s.data)
				if err != nil {
					out = append(out, s) // best-effort: keep original on parse failure
				} else {
					out = append(out, jpegSeg{marker: 0xE1, data: newData})
				}
				continue
			}
			// TODO: strip IPTC location fields from APP13 (requires IPTC segment parser)
			out = append(out, s)
		}
	}
	return out, nil
}
