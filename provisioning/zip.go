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
	data    []byte
	headers map[*zip.File]int64 // local header offsets; nil when not readable here
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
	a := &archive{Reader: r, data: data}
	if offsets := localHeaderOffsets(data); len(offsets) == len(r.File) {
		a.headers = make(map[*zip.File]int64, len(offsets))
		for i, f := range r.File {
			a.headers[f] = offsets[i]
		}
	}
	return a, nil
}

// localHeaderOffsets reads the central directory's local header offsets, in
// entry order. It returns nil for a ZIP64 archive or one it cannot read; an
// archive that large is past every limit here anyway.
func localHeaderOffsets(data []byte) []int64 {
	const eocdLen = 22
	start := len(data) - eocdLen - 0xFFFF
	if start < 0 {
		start = 0
	}
	eocd := bytes.LastIndex(data[start:], []byte{0x50, 0x4b, 0x05, 0x06})
	if eocd < 0 || start+eocd+eocdLen > len(data) {
		return nil
	}
	e := data[start+eocd:]
	count := int(binary.LittleEndian.Uint16(e[10:]))
	pos := int64(binary.LittleEndian.Uint32(e[16:]))
	if count == 0xFFFF || pos == 0xFFFFFFFF {
		return nil
	}
	offsets := make([]int64, 0, count)
	for range count {
		if pos < 0 || pos+46 > int64(len(data)) || binary.LittleEndian.Uint32(data[pos:]) != 0x02014b50 {
			return nil
		}
		h := data[pos:]
		offsets = append(offsets, int64(binary.LittleEndian.Uint32(h[42:])))
		pos += 46 + int64(binary.LittleEndian.Uint16(h[28:])) + int64(binary.LittleEndian.Uint16(h[30:])) +
			int64(binary.LittleEndian.Uint16(h[32:]))
	}
	return offsets
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
	h, known := a.headers[f]
	if !known {
		return nil
	}
	if h < 0 || h+30 > int64(len(a.data)) || binary.LittleEndian.Uint32(a.data[h:]) != 0x04034b50 {
		return invalidData("%s: no local header", f.Name)
	}
	hdr := a.data[h : h+30]
	if binary.LittleEndian.Uint16(hdr[6:])&0x8 != 0 {
		return nil
	}
	crc, size := binary.LittleEndian.Uint32(hdr[14:]), binary.LittleEndian.Uint32(hdr[22:])
	if size != 0xFFFFFFFF && (crc != crc32.ChecksumIEEE(data) || uint64(size) != uint64(len(data))) {
		return invalidData("%s: size or CRC-32 differs from its local header", f.Name)
	}
	return nil
}
