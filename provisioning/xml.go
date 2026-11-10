package provisioning

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"unicode/utf16"
)

// maxElements bounds a document's elements: the tree costs far more memory
// than the bytes it came from, and real system.xml files have well under a
// million (spec feedback §4).
const maxElements = 1_000_000

// xmlElement is a parsed element: the minimal tree §4.5 reads.
type xmlElement struct {
	name     string
	attrs    map[string]string
	children []*xmlElement
	text     string // the character data before the first child element
	buf      strings.Builder
}

// child returns the first direct child named name.
func (e *xmlElement) child(name string) *xmlElement {
	for _, c := range e.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

// xmlDoc is a document with its id index (spec §4.5.1 item 6).
type xmlDoc struct {
	root     *xmlElement
	byID     map[string]*xmlElement
	resolved map[*xmlElement]*xmlElement // memo: each chain is walked once
}

// parseXML reads a project document: well-formed, no DTD in any encoding,
// UTF-8 or UTF-16 (spec §4.5.1 item 4), ids unique.
func parseXML(data []byte) (*xmlDoc, error) {
	text, err := xmlText(data)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(text))
	dec.Strict = true
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		// The text is UTF-8 by now: a UTF-16 document was transcoded.
		switch strings.ToLower(charset) {
		case "utf-16", "utf-16le", "utf-16be":
			return input, nil
		}
		return nil, errors.New("unsupported encoding " + charset)
	}
	doc := &xmlDoc{byID: map[string]*xmlElement{}, resolved: map[*xmlElement]*xmlElement{}}
	var stack []*xmlElement
	elements := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalidData("xml: %v", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			if bytes.HasPrefix(bytes.TrimSpace(t), []byte("DOCTYPE")) {
				return nil, invalidData("xml: a document type declaration")
			}
			return nil, invalidData("xml: a markup declaration")
		case xml.StartElement:
			if elements++; elements > maxElements {
				return nil, invalidData("xml: more than %d elements", maxElements)
			}
			e := &xmlElement{name: t.Name.Local, attrs: map[string]string{}}
			for _, a := range t.Attr {
				e.attrs[a.Name.Local] = a.Value
			}
			if id, has := e.attrs["id"]; has {
				if _, dup := doc.byID[id]; dup {
					return nil, invalidData("xml: two elements with id %q", id)
				}
				doc.byID[id] = e
			}
			if len(stack) == 0 {
				if doc.root != nil {
					return nil, invalidData("xml: more than one root element")
				}
				doc.root = e
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, e)
			}
			stack = append(stack, e)
		case xml.EndElement:
			e := stack[len(stack)-1]
			e.text = e.buf.String()
			e.buf = strings.Builder{}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if n := len(stack); n > 0 && len(stack[n-1].children) == 0 {
				stack[n-1].buf.Write(t)
			}
		}
	}
	if doc.root == nil {
		return nil, invalidData("xml: no root element")
	}
	return doc, nil
}

// xmlText returns the document as UTF-8: a UTF-8 BOM is dropped, a UTF-16
// document (by its BOM) is transcoded.
func xmlText(data []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return data[3:], nil
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		body := data[2:]
		if len(body)%2 != 0 {
			return nil, invalidData("xml: odd-length UTF-16")
		}
		units := make([]uint16, len(body)/2)
		for i := range units {
			if data[0] == 0xFF {
				units[i] = uint16(body[2*i]) | uint16(body[2*i+1])<<8
			} else {
				units[i] = uint16(body[2*i])<<8 | uint16(body[2*i+1])
			}
		}
		return []byte(string(utf16.Decode(units))), nil
	}
	return data, nil
}

// resolve follows reference="…" to the element with that id; a missing
// target or a cycle is invalidData (spec §4.5.1 item 6). Results are
// memoized, so every chain is walked once however often it is read.
func (d *xmlDoc) resolve(e *xmlElement) (*xmlElement, error) {
	var chain []*xmlElement
	onChain := map[*xmlElement]bool{}
	cur := e
	for {
		if r, done := d.resolved[cur]; done {
			cur = r
			break
		}
		target, has := cur.attrs["reference"]
		if !has {
			break
		}
		next, ok := d.byID[target]
		if !ok || onChain[next] || next == cur {
			return nil, invalidData("xml: reference %q has no target or is a cycle", target)
		}
		chain = append(chain, cur)
		onChain[cur] = true
		cur = next
	}
	for _, c := range chain {
		d.resolved[c] = cur
	}
	return cur, nil
}

// child is the first direct child named name, resolved; nil when absent.
func (d *xmlDoc) child(e *xmlElement, name string) (*xmlElement, error) {
	c := e.child(name)
	if c == nil {
		return nil, nil
	}
	return d.resolve(c)
}

// text is a field's trimmed text (spec §4.5.1 item 7); "" means absent.
func (d *xmlDoc) text(e *xmlElement, name string) (string, error) {
	c, err := d.child(e, name)
	if c == nil || err != nil {
		return "", err
	}
	return strings.TrimSpace(c.text), nil
}

// hexNumber is 1–8 hex digits without sign, prefix or separators.
func hexNumber(text string) (uint64, bool) {
	if len(text) < 1 || len(text) > 8 || strings.Trim(text, "0123456789abcdefABCDEF") != "" {
		return 0, false
	}
	var n uint64
	for _, c := range []byte(text) {
		switch {
		case c <= '9':
			n = n<<4 | uint64(c-'0')
		case c >= 'a':
			n = n<<4 | uint64(c-'a'+10)
		default:
			n = n<<4 | uint64(c-'A'+10)
		}
	}
	return n, true
}
