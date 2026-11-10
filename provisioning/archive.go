package provisioning

import (
	"unicode/utf8"

	"github.com/osieckiAdam/grenton-core-go/model"
)

// ArchiveLimit is the limit of spec §4.3 item 4: 10 MiB for the response
// body and for data.json after decompression, counted on actual bytes.
const ArchiveLimit = 10 * mib

// interfaceEntry is the archive entry holding the interface (spec §4.3).
const interfaceEntry = "mygrenton/data.json"

// ParseArchive reads an interface archive as Object Manager sends it (a
// saved download): exactly one regular entry mygrenton/data.json, matched
// byte for byte, at most [ArchiveLimit] inflated, its size and CRC as its
// headers declare. Every failure is an [InvalidData] error.
func ParseArchive(data []byte) (*model.Interface, error) {
	if len(data) > ArchiveLimit {
		return nil, invalidData("the archive is larger than %d bytes", ArchiveLimit)
	}
	r, err := openZip(data)
	if err != nil {
		return nil, err
	}
	var found int
	for _, f := range r.File {
		if f.Name == interfaceEntry {
			found++
		}
	}
	if found != 1 {
		return nil, invalidData("the archive has %d entries %s, not one", found, interfaceEntry)
	}
	for _, f := range r.File {
		if f.Name != interfaceEntry {
			continue
		}
		if !f.Mode().IsRegular() {
			return nil, invalidData("%s is not a regular file", interfaceEntry)
		}
		raw, err := readEntry(f, ArchiveLimit)
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(raw) {
			return nil, invalidData("%s is not UTF-8", interfaceEntry)
		}
		in, err := model.DecodeInterface(raw)
		if err != nil {
			return nil, invalidData("%v", err)
		}
		return in, nil
	}
	return nil, invalidData("unreachable")
}
