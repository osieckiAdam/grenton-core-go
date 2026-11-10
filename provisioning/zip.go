package provisioning

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

const mib = 1 << 20

// maxEntries bounds the number of entries of any archive read here.
const maxEntries = 10_000

// archive is a ZIP archive with its bytes, so that local headers can be
// checked against the central directory.
type archive struct {
	*zip.Reader
	data []byte
}

// openZip lists an archive's central directory. Entry names are never used
// as paths, so archive/zip's insecure-path report is not an error here.
func openZip(data []byte) (*archive, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, invalidData("bad zip")
	}
	if len(r.File) > maxEntries {
		return nil, invalidData("more than %d archive entries", maxEntries)
	}
	return &archive{Reader: r, data: data}, nil
}

// readEntry inflates an entry, counting the bytes actually produced and
// stopping one past limit; the declared size is never trusted. The result
// must match the size and CRC-32 of both its headers (spec §4.3 items 4–5):
// archive/zip checks only the central directory, and skips a declared CRC
// of 0, so the check is done here.
func (a *archive) readEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, invalidData("%s: %v", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if int64(len(data)) > limit {
		return nil, invalidData("%s inflates beyond %d bytes", f.Name, limit)
	}
	if err != nil {
		return nil, invalidData("%s: %v", f.Name, err)
	}
	if uint64(len(data)) != f.UncompressedSize64 || crc32.ChecksumIEEE(data) != f.CRC32 {
		return nil, invalidData("%s: size or CRC-32 differs from its central header", f.Name)
	}
	if err := a.checkLocalHeader(f, data); err != nil {
		return nil, err
	}
	return data, nil
}

// checkLocalHeader compares the local file header of f with the inflated
// data. With a data descriptor (flag bit 3) the local fields may be zero
// and the descriptor's values are archive/zip's to check.
func (a *archive) checkLocalHeader(f *zip.File, data []byte) error {
	dataOffset, err := f.DataOffset()
	if err != nil {
		return invalidData("%s: %v", f.Name, err)
	}
	// The header is 30 bytes, the name and an extra field of unknown length.
	for extra := int64(0); extra <= 0xFFFF; extra++ {
		h := dataOffset - 30 - int64(len(f.Name)) - extra
		if h < 0 {
			break
		}
		hdr := a.data[h : h+30]
		if binary.LittleEndian.Uint32(hdr) != 0x04034b50 || int64(binary.LittleEndian.Uint16(hdr[28:])) != extra {
			continue
		}
		if binary.LittleEndian.Uint16(hdr[6:])&0x8 != 0 {
			return nil
		}
		crc, size := binary.LittleEndian.Uint32(hdr[14:]), binary.LittleEndian.Uint32(hdr[22:])
		if size == 0xFFFFFFFF { // ZIP64: the size is in the extra field
			return nil
		}
		if crc != crc32.ChecksumIEEE(data) || uint64(size) != uint64(len(data)) {
			return invalidData("%s: size or CRC-32 differs from its local header", f.Name)
		}
		return nil
	}
	return invalidData("%s: no local header", f.Name)
}
