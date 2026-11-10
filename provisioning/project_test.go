package provisioning

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
	"unicode/utf16"
)

const testProperties = `<object-stream><ProjectProperties id="1"><projectCipherKey id="2">
<keyBytes>AAECAwQFBgcICQoLDA0ODw==</keyBytes><ivBytes>EBESExQVFhcYGRobHB0eHw==</ivBytes>
</projectCipherKey></ProjectProperties></object-stream>`

const testSystem = `<?xml version="1.0" encoding="UTF-16"?>
<object-stream>
  <TreeObject id="1"><specificObject id="2"><name>Main</name><nameOnCLU>CLU_A</nameOnCLU>
    <ipAddress>10.0.0.1</ipAddress><serialNumber>0000000a</serialNumber></specificObject></TreeObject>
  <TreeObject id="3"><specificObject id="4"><name>Spare</name><nameOnCLU>CLU_B</nameOnCLU>
    <ipAddress>10.0.0.2</ipAddress><serialNumber>0000000b</serialNumber></specificObject></TreeObject>
</object-stream>`

// A multisensor whose channels name CLU_B: the only reference to it.
const testNode = `{"id": "iface-1", "label": "Test", "icon": "home", "theme": "blue", "pages": [
 {"label": "P", "icon": "home", "widgets": [
  {"type": "MULTISENSOR", "dtoId": "m1", "icon": "x", "label": "Air", "labelVisible": true,
   "objectTemperature": {"value": {"cluId": "CLU_B", "objectName": "SEN1001", "index": "0", "callType": "ATTRIBUTE"}}}
 ]}]}`

func utf16Document(text string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(text)) {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

func buildProject(t *testing.T, entries map[string][]byte, order []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportsUTF16AndCountsMultisensorReferences(t *testing.T) {
	data := buildProject(t, map[string][]byte{
		"properties.xml":    []byte(testProperties),
		"system.xml":        utf16Document(testSystem),
		"nodes/1/data.json": []byte(testNode),
	}, []string{"properties.xml", "system.xml", "nodes/1/data.json"})
	p, err := OpenProject(data)
	if err != nil {
		t.Fatal(err)
	}
	in, err := p.Interface("")
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Clus) != 1 || in.Clus[0].ID != "CLU_B" || in.Clus[0].SerialNumber != "11" {
		t.Fatalf("clus = %+v, want only CLU_B (serial 11)", in.Clus)
	}
	if in.Theme != "BLUE" || in.Version != 0 {
		t.Fatalf("theme %q version %d", in.Theme, in.Version)
	}
}

func TestRejectsAnotherEncoding(t *testing.T) {
	data := buildProject(t, map[string][]byte{
		"properties.xml":    []byte(testProperties),
		"system.xml":        []byte(`<?xml version="1.0" encoding="ISO-8859-2"?><object-stream/>`),
		"nodes/1/data.json": []byte(testNode),
	}, []string{"properties.xml", "system.xml", "nodes/1/data.json"})
	if _, err := OpenProject(data); !errors.Is(err, ErrInvalidData) {
		t.Fatalf("want invalidData, got %v", err)
	}
}
