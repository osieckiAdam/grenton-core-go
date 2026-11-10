package provisioning

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
)

const mib = 1 << 20

// maxEntries bounds the number of entries of any archive read here.
const maxEntries = 10_000

// openZip lists an archive's central directory. Entry names are never used
// as paths, so archive/zip's insecure-path report is not an error here.
func openZip(data []byte) (*zip.Reader, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, invalidData("bad zip")
	}
	if len(r.File) > maxEntries {
		return nil, invalidData("more than %d archive entries", maxEntries)
	}
	return r, nil
}

// readEntry inflates an entry, counting the bytes actually produced and
// stopping one past limit; the header's declared size is never trusted.
// archive/zip's reader also fails an entry whose inflated size or CRC-32
// differs from its headers (spec §4.3 items 4–5).
func readEntry(f *zip.File, limit int64) ([]byte, error) {
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
	return data, nil
}
