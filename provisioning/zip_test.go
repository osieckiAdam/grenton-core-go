package provisioning

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func storedArchive(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// CreateRaw writes the sizes and CRC into the local header (no data
	// descriptor), as Object Manager's archives have them.
	w, err := zw.CreateRaw(&zip.FileHeader{Name: interfaceEntry, Method: zip.Store, CRC32: crc32.ChecksumIEEE(content),
		CompressedSize64: uint64(len(content)), UncompressedSize64: uint64(len(content))})
	if err != nil {
		t.Fatal(err)
	}
	w.Write(content)
	zw.Close()
	return buf.Bytes()
}

func fixtureInterface(t *testing.T) []byte {
	data, err := os.ReadFile(filepath.Join("..", "vendor", "grenton-core-spec", "conformance", "fixtures", "interfaces", "all-widget-types.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// patch overwrites 4 bytes at the n-th occurrence of a header signature.
func patch(data []byte, sig uint32, offset int, value uint32) []byte {
	out := append([]byte(nil), data...)
	var s [4]byte
	binary.LittleEndian.PutUint32(s[:], sig)
	i := bytes.Index(out, s[:])
	binary.LittleEndian.PutUint32(out[i+offset:], value)
	return out
}

func TestArchiveIntegrity(t *testing.T) {
	good := storedArchive(t, fixtureInterface(t))
	if _, err := ParseArchive(good); err != nil {
		t.Fatalf("the good archive: %v", err)
	}
	for name, data := range map[string][]byte{
		// A declared CRC of 0 in both headers: archive/zip alone skips it.
		"zero crc": patch(patch(good, 0x02014b50, 16, 0), 0x04034b50, 14, 0),
		// Only the local header lies.
		"local crc":  patch(good, 0x04034b50, 14, 1),
		"local size": patch(good, 0x04034b50, 22, 1),
	} {
		if _, err := ParseArchive(data); !errors.Is(err, ErrInvalidData) {
			t.Errorf("%s: want invalidData, got %v", name, err)
		}
	}
}
