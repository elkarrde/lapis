// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package xmp

import (
	"bytes"
	"fmt"
)

// locationFields lists the XMP field names (in prefix:local form) whose values
// carry location or GPS data. CleanLocation blanks every occurrence of each,
// covering both the flat fields on an rdf:Description and the leaf fields nested
// inside the structured Iptc4xmpExt:LocationCreated / LocationShown containers
// (an rdf:Bag of location structures). Because editing is value-blanking rather
// than element deletion, the same name matches whether the datum sits at the
// top level or inside a struct, so no container-aware parsing is needed.
//
// GPS coordinates are the priority leak; the flat place-names and the structured
// Iptc4xmpExt leaf names follow. Blanking every occurrence empties the data
// while keeping the surrounding rdf:Description / rdf:Bag well-formed.
var locationFields = []string{
	// GPS coordinates (the real leak) — may appear flat or inside a struct.
	"exif:GPSLatitude",
	"exif:GPSLongitude",
	"exif:GPSAltitude",
	"exif:GPSAltitudeRef",
	// Flat place-names.
	"photoshop:City",
	"photoshop:State",
	"photoshop:Country",
	"Iptc4xmpCore:Location",
	"Iptc4xmpCore:CountryCode",
	// Structured Iptc4xmpExt leaf fields (inside LocationCreated / LocationShown).
	"Iptc4xmpExt:City",
	"Iptc4xmpExt:ProvinceState",
	"Iptc4xmpExt:CountryName",
	"Iptc4xmpExt:CountryCode",
	"Iptc4xmpExt:Sublocation",
	"Iptc4xmpExt:WorldRegion",
	"Iptc4xmpExt:LocationName",
}

// CleanLocation blanks the value of every location and GPS field in a raw XMP
// APP1 payload, preserving the payload's byte length via xpacket whitespace
// padding (the same length-preserving technique as Clean). It is a distinct,
// opt-in entry point: the default Clean targets only Adobe-signature fields and
// is consumed by tidy-exif, so folding location handling into it would silently
// change that behavior.
//
// Both attribute form (name="value") and element form (<name>value</name>) are
// blanked, at every nesting depth, so location data inside the structured
// Iptc4xmpExt:LocationCreated / LocationShown containers is emptied along with
// the flat fields. The empty containers are left in place; they carry no data.
//
// If the payload holds no location data (nothing changes), CleanLocation returns
// the original payload unchanged with changed == false.
func CleanLocation(payload []byte) (out []byte, changed bool, err error) {
	if !bytes.HasPrefix(payload, xmpSig) {
		return nil, false, fmt.Errorf("xmp: not an XMP segment")
	}

	xmlPart := append([]byte(nil), payload[len(xmpSig):]...)
	cleaned := xmlPart
	for _, name := range locationFields {
		cleaned = patchAll(cleaned, name, "")
	}
	if bytes.Equal(cleaned, xmlPart) {
		return payload, false, nil
	}

	patched := append(append([]byte(nil), xmpSig...), cleaned...)
	out, err = adjustPadding(patched, len(payload))
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}
