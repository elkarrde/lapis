// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package xmp performs field-level surgery on the XMP packet carried in a JPEG
// APP1 segment. It operates on the segment payload (the bytes beginning with
// the Adobe xap namespace signature) and does not import the jpeg package, so
// it stays independently usable and testable.
//
// Editing is length-preserving: Clean rewrites target field values in place via
// targeted regexp replacement, then expands the xpacket whitespace padding so
// the payload keeps its original byte length. Preserving length means none of
// the surrounding JPEG/TIFF offsets need to move.
//
// Adobe writes the xmpMM:History stEvt:softwareAgent in ATTRIBUTE form
// (<rdf:li stEvt:softwareAgent="..."/>), not only element form; Parse and Clean
// handle both. Missing the attribute form silently skipped every real
// Lightroom/Photoshop file, so patchAll covers both forms deliberately.
//
// Lifted from codeberg.org/elkarrde/tidy-exif.
package xmp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// XMP namespace URIs used to identify fields in the token stream.
const (
	nsXMP   = "http://ns.adobe.com/xap/1.0/"
	nsXMPMM = "http://ns.adobe.com/xap/1.0/mm/"
	nsStEvt = "http://ns.adobe.com/xap/1.0/sType/ResourceEvent#"
	nsRDF   = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
)

// xmpSig is the Adobe xap namespace signature that prefixes an XMP APP1
// payload. It matches jpeg.Segment.Data for an XMP segment; the package
// redefines it locally rather than importing jpeg.
var xmpSig = []byte("http://ns.adobe.com/xap/1.0/\x00")

// Fields holds the Adobe-specific XMP metadata targeted for removal or
// replacement.
type Fields struct {
	CreatorTool        string
	MetadataDate       string
	DocumentID         string
	InstanceID         string
	OriginalDocumentID string
	SoftwareAgents     []string // one entry per xmpMM:History item
}

// Any reports whether any target field holds a non-empty value.
func (f *Fields) Any() bool {
	if f.CreatorTool != "" || f.MetadataDate != "" || f.DocumentID != "" ||
		f.InstanceID != "" || f.OriginalDocumentID != "" {
		return true
	}
	for _, a := range f.SoftwareAgents {
		if a != "" {
			return true
		}
	}
	return false
}

// Parse extracts the target fields from a raw XMP APP1 payload using an
// encoding/xml token decoder. The payload must begin with the Adobe xap
// namespace signature. Both attribute and element forms of each field are
// handled, including the attribute-form history entries that Lightroom and
// Photoshop actually write.
func Parse(payload []byte) (*Fields, error) {
	if !bytes.HasPrefix(payload, xmpSig) {
		return nil, fmt.Errorf("xmp: not an XMP segment")
	}

	f := &Fields{}
	dec := xml.NewDecoder(bytes.NewReader(payload[len(xmpSig):]))
	dec.Strict = false

	inHistory := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // return whatever we extracted before the error
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Space == nsRDF && t.Name.Local == "Description":
				for _, a := range t.Attr {
					switch {
					case matchAttr(a, nsXMP, "CreatorTool", "xmp"):
						f.CreatorTool = a.Value
					case matchAttr(a, nsXMP, "MetadataDate", "xmp"):
						f.MetadataDate = a.Value
					case matchAttr(a, nsXMPMM, "DocumentID", "xmpMM"):
						f.DocumentID = a.Value
					case matchAttr(a, nsXMPMM, "InstanceID", "xmpMM"):
						f.InstanceID = a.Value
					case matchAttr(a, nsXMPMM, "OriginalDocumentID", "xmpMM"):
						f.OriginalDocumentID = a.Value
					}
				}

			// Element form of simple fields (less common but valid XMP).
			case t.Name.Space == nsXMP && t.Name.Local == "CreatorTool":
				f.CreatorTool = nextCharData(dec)
			case t.Name.Space == nsXMP && t.Name.Local == "MetadataDate":
				f.MetadataDate = nextCharData(dec)
			case t.Name.Space == nsXMPMM && t.Name.Local == "DocumentID":
				f.DocumentID = nextCharData(dec)
			case t.Name.Space == nsXMPMM && t.Name.Local == "InstanceID":
				f.InstanceID = nextCharData(dec)
			case t.Name.Space == nsXMPMM && t.Name.Local == "OriginalDocumentID":
				f.OriginalDocumentID = nextCharData(dec)

			case t.Name.Space == nsXMPMM && t.Name.Local == "History":
				inHistory = true
			case inHistory && t.Name.Space == nsStEvt && t.Name.Local == "softwareAgent":
				// element form: <stEvt:softwareAgent>value</stEvt:softwareAgent>
				f.SoftwareAgents = append(f.SoftwareAgents, nextCharData(dec))
			case inHistory:
				// attribute form (what Lightroom/Photoshop actually write):
				// <rdf:li ... stEvt:softwareAgent="value" .../>
				for _, a := range t.Attr {
					if matchAttr(a, nsStEvt, "softwareAgent", "stEvt") {
						f.SoftwareAgents = append(f.SoftwareAgents, a.Value)
					}
				}
			}

		case xml.EndElement:
			if t.Name.Space == nsXMPMM && t.Name.Local == "History" {
				inHistory = false
			}
		}
	}

	return f, nil
}

// Clean empties or replaces the target fields in a raw XMP payload in place,
// preserving the payload's byte length via xpacket whitespace padding.
//
// replacements maps field names ("CreatorTool", "MetadataDate", "DocumentID",
// "InstanceID", "OriginalDocumentID", "SoftwareAgent") to replacement values;
// any field absent from the map is emptied. All history entries are replaced
// with the single "SoftwareAgent" value.
//
// If the payload carries no Adobe data, Clean returns the original payload
// unchanged with changed == false.
func Clean(payload []byte, replacements map[string]string) (out []byte, changed bool, err error) {
	f, err := Parse(payload)
	if err != nil {
		return nil, false, err
	}
	if !f.Any() {
		return payload, false, nil
	}
	out, err = marshal(payload, cleanFields(f, replacements))
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// matchAttr reports whether an xml.Attr matches the given namespace and local
// name. It handles both the URI-resolved form (proper XML namespace handling)
// and the prefixed local-name form that Go's encoding/xml may produce for
// attributes.
func matchAttr(a xml.Attr, ns, local, prefix string) bool {
	return (a.Name.Space == ns && a.Name.Local == local) ||
		(a.Name.Space == "" && a.Name.Local == prefix+":"+local)
}

// nextCharData reads the next decoder token and returns it as a trimmed string
// if it is character data. Returns "" for empty elements or on any error.
func nextCharData(dec *xml.Decoder) string {
	tok, err := dec.Token()
	if err != nil {
		return ""
	}
	if cd, ok := tok.(xml.CharData); ok {
		return strings.TrimSpace(string(cd))
	}
	return ""
}

// cleanFields returns a new Fields with all target fields replaced.
// replacements maps field names to replacement values; any field absent from
// the map is set to "".
func cleanFields(f *Fields, replacements map[string]string) *Fields {
	repl := func(key string) string {
		if v, ok := replacements[key]; ok {
			return v
		}
		return ""
	}
	agents := make([]string, len(f.SoftwareAgents))
	for i := range f.SoftwareAgents {
		agents[i] = repl("SoftwareAgent")
	}
	return &Fields{
		CreatorTool:        repl("CreatorTool"),
		MetadataDate:       repl("MetadataDate"),
		DocumentID:         repl("DocumentID"),
		InstanceID:         repl("InstanceID"),
		OriginalDocumentID: repl("OriginalDocumentID"),
		SoftwareAgents:     agents,
	}
}

// marshal applies cleaned field values back to the original raw payload bytes
// via targeted regexp replacement, then adjusts xpacket padding to preserve
// length.
func marshal(original []byte, cleaned *Fields) ([]byte, error) {
	if !bytes.HasPrefix(original, xmpSig) {
		return nil, fmt.Errorf("xmp: not an XMP segment")
	}

	xmlPart := make([]byte, len(original)-len(xmpSig))
	copy(xmlPart, original[len(xmpSig):])

	fields := []struct{ name, value string }{
		{"xmp:CreatorTool", cleaned.CreatorTool},
		{"xmp:MetadataDate", cleaned.MetadataDate},
		{"xmpMM:DocumentID", cleaned.DocumentID},
		{"xmpMM:InstanceID", cleaned.InstanceID},
		{"xmpMM:OriginalDocumentID", cleaned.OriginalDocumentID},
	}
	for _, fld := range fields {
		xmlPart = patchField(xmlPart, fld.name, fld.value)
	}
	// All history entries are replaced with the same value (see cleanFields),
	// so a single global replacement across attribute and element forms
	// suffices.
	agentRepl := ""
	if len(cleaned.SoftwareAgents) > 0 {
		agentRepl = cleaned.SoftwareAgents[0]
	}
	xmlPart = patchAll(xmlPart, "stEvt:softwareAgent", agentRepl)

	patched := append(append([]byte(nil), xmpSig...), xmlPart...)
	return adjustPadding(patched, len(original))
}

// patchField replaces the value of a named XMP field in raw XML bytes.
// Handles attribute form (name="value") and element form (<name>value</name>).
func patchField(xmlBytes []byte, name, replacement string) []byte {
	qn := regexp.QuoteMeta(name)
	// attribute, double-quoted
	if re := regexp.MustCompile(qn + `="[^"]*"`); re.Match(xmlBytes) {
		return re.ReplaceAll(xmlBytes, []byte(name+`="`+replacement+`"`))
	}
	// attribute, single-quoted
	if re := regexp.MustCompile(qn + `='[^']*'`); re.Match(xmlBytes) {
		return re.ReplaceAll(xmlBytes, []byte(name+`='`+replacement+`'`))
	}
	// element form
	if re := regexp.MustCompile(`<` + qn + `>[^<]*</` + qn + `>`); re.Match(xmlBytes) {
		return re.ReplaceAll(xmlBytes, []byte(`<`+name+`>`+replacement+`</`+name+`>`))
	}
	return xmlBytes
}

// patchAll replaces the value of every occurrence of a field in raw XML bytes,
// in both attribute form (name="value" / name='value') and element form
// (<name>value</name>). Used for repeated fields such as history softwareAgent;
// covering the attribute form is the Lightroom/Photoshop bug fix.
func patchAll(xmlBytes []byte, name, replacement string) []byte {
	qn := regexp.QuoteMeta(name)
	out := xmlBytes
	out = regexp.MustCompile(qn+`="[^"]*"`).ReplaceAll(out, []byte(name+`="`+replacement+`"`))
	out = regexp.MustCompile(qn+`='[^']*'`).ReplaceAll(out, []byte(name+`='`+replacement+`'`))
	out = regexp.MustCompile(`<`+qn+`>[^<]*</`+qn+`>`).ReplaceAll(out, []byte(`<`+name+`>`+replacement+`</`+name+`>`))
	return out
}

// adjustPadding expands the xpacket padding in data to reach exactly targetLen
// bytes. XMP padding lives between </x:xmpmeta> and <?xpacket end=...?>.
func adjustPadding(data []byte, targetLen int) ([]byte, error) {
	diff := targetLen - len(data)
	if diff == 0 {
		return data, nil
	}
	if diff < 0 {
		return nil, fmt.Errorf("xmp: patched XMP is %d bytes larger than original segment", -diff)
	}

	extra := bytes.Repeat([]byte{' '}, diff)

	endMetaIdx := bytes.Index(data, []byte("</x:xmpmeta>"))
	endPktIdx := bytes.Index(data, []byte("<?xpacket end"))

	insertAt := -1
	switch {
	case endMetaIdx >= 0 && endPktIdx > endMetaIdx:
		insertAt = endPktIdx // standard xpacket padding area
	case endPktIdx >= 0:
		insertAt = endPktIdx // no end-meta tag; insert before end-packet anyway
	}

	if insertAt >= 0 {
		out := make([]byte, 0, targetLen)
		out = append(out, data[:insertAt]...)
		out = append(out, extra...)
		out = append(out, data[insertAt:]...)
		return out, nil
	}

	// No xpacket structure found; pad at end as a last resort.
	return append(data, extra...), nil
}
