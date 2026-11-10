// Package strictjson is the strict JSON of spec §4.5.1 item 5, used for the
// nodes of an Object Manager project: invalid UTF-8, a repeated key, NaN or
// Infinity, an integer outside Int64, a non-finite double or nesting deeper
// than the limit is an error. The parser is iterative, so a deep input never
// grows the goroutine stack.
//
// Values decode to nil, bool, string, int64 (a number without a fraction or
// exponent), float64, []any and map[string]any.
package strictjson

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// MaxDepth is the nesting limit (the spec allows 32–1024; real nodes have
// about ten levels).
const MaxDepth = 64

// MaxValues bounds the values of one document: a decoded tree costs far
// more memory than its text (spec feedback §4).
const MaxValues = 2_000_000

// ErrInvalid is every parse error.
var ErrInvalid = errors.New("invalid strict JSON")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Parse decodes one JSON value; only whitespace may follow it.
func Parse(data []byte) (any, error) {
	if !utf8.Valid(data) {
		return nil, invalid("not UTF-8")
	}
	p := &parser{s: data}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.space()
	if p.i != len(p.s) {
		return nil, invalid("trailing data at %d", p.i)
	}
	return v, nil
}

// frame is a container being filled.
type frame struct {
	array []any
	obj   map[string]any
	isObj bool
	key   string // the key whose value is being parsed
}

type parser struct {
	s      []byte
	i      int
	stack  []*frame
	values int
}

func (p *parser) space() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

// value parses a value with an explicit stack of open containers.
func (p *parser) value() (any, error) {
	for {
		p.space()
		if p.i >= len(p.s) {
			return nil, invalid("unexpected end")
		}
		if p.values++; p.values > MaxValues {
			return nil, invalid("more than %d values", MaxValues)
		}
		var v any
		switch c := p.s[p.i]; {
		case c == '{' || c == '[':
			if len(p.stack) >= MaxDepth {
				return nil, invalid("nesting deeper than %d", MaxDepth)
			}
			p.i++
			f := &frame{isObj: c == '{'}
			if f.isObj {
				f.obj = map[string]any{}
			} else {
				f.array = []any{}
			}
			p.space()
			closer := byte(']')
			if f.isObj {
				closer = '}'
			}
			if p.i < len(p.s) && p.s[p.i] == closer {
				p.i++
				if f.isObj {
					v = f.obj
				} else {
					v = f.array
				}
				break
			}
			p.stack = append(p.stack, f)
			if f.isObj {
				if err := p.objectKey(f); err != nil {
					return nil, err
				}
			}
			continue
		case c == '"':
			s, err := p.str()
			if err != nil {
				return nil, err
			}
			v = s
		case c == 't':
			if err := p.literal("true"); err != nil {
				return nil, err
			}
			v = true
		case c == 'f':
			if err := p.literal("false"); err != nil {
				return nil, err
			}
			v = false
		case c == 'n':
			if err := p.literal("null"); err != nil {
				return nil, err
			}
			v = nil
		case c == '-' || ('0' <= c && c <= '9'):
			n, err := p.number()
			if err != nil {
				return nil, err
			}
			v = n
		default:
			return nil, invalid("unexpected %q at %d", c, p.i)
		}
		// v is complete: store it in the innermost container, and close
		// every container that ends after it.
		for {
			if len(p.stack) == 0 {
				return v, nil
			}
			f := p.stack[len(p.stack)-1]
			if f.isObj {
				f.obj[f.key] = v
			} else {
				f.array = append(f.array, v)
			}
			p.space()
			if p.i >= len(p.s) {
				return nil, invalid("unexpected end")
			}
			c := p.s[p.i]
			p.i++
			switch {
			case c == ',':
				if f.isObj {
					if err := p.objectKey(f); err != nil {
						return nil, err
					}
				}
			case f.isObj && c == '}':
				p.stack = p.stack[:len(p.stack)-1]
				v = f.obj
				continue
			case !f.isObj && c == ']':
				p.stack = p.stack[:len(p.stack)-1]
				v = f.array
				continue
			default:
				return nil, invalid("unexpected %q at %d", c, p.i-1)
			}
			break
		}
	}
}

// objectKey reads `"key" :` and refuses a key the object already has.
func (p *parser) objectKey(f *frame) error {
	p.space()
	if p.i >= len(p.s) || p.s[p.i] != '"' {
		return invalid("object key expected at %d", p.i)
	}
	k, err := p.str()
	if err != nil {
		return err
	}
	if _, dup := f.obj[k]; dup {
		return invalid("repeated key %q", k)
	}
	p.space()
	if p.i >= len(p.s) || p.s[p.i] != ':' {
		return invalid("':' expected at %d", p.i)
	}
	p.i++
	f.key = k
	return nil
}

func (p *parser) literal(word string) error {
	if len(p.s)-p.i < len(word) || string(p.s[p.i:p.i+len(word)]) != word {
		return invalid("bad literal at %d", p.i)
	}
	p.i += len(word)
	return nil
}

// number reads the JSON number grammar; an integer form must fit Int64 and
// any other form must be a finite double.
func (p *parser) number() (any, error) {
	start := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	digits := func() int {
		n := 0
		for p.i < len(p.s) && '0' <= p.s[p.i] && p.s[p.i] <= '9' {
			p.i++
			n++
		}
		return n
	}
	intStart := p.i
	if n := digits(); n == 0 || (n > 1 && p.s[intStart] == '0') {
		return nil, invalid("bad number at %d", start)
	}
	isFloat := false
	if p.i < len(p.s) && p.s[p.i] == '.' {
		p.i++
		isFloat = true
		if digits() == 0 {
			return nil, invalid("bad number at %d", start)
		}
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		p.i++
		isFloat = true
		if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
			p.i++
		}
		if digits() == 0 {
			return nil, invalid("bad number at %d", start)
		}
	}
	text := string(p.s[start:p.i])
	if !isFloat {
		i, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, invalid("integer %s outside Int64", text)
		}
		return i, nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(f, 0) {
		return nil, invalid("number %s is not a finite double", text)
	}
	return f, nil
}

// str reads a string with its escapes; unpaired surrogates are errors.
func (p *parser) str() (string, error) {
	p.i++ // opening quote
	var out []byte
	for {
		if p.i >= len(p.s) {
			return "", invalid("unterminated string")
		}
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return string(out), nil
		case c < 0x20:
			return "", invalid("control character in string at %d", p.i)
		case c != '\\':
			out = append(out, c)
			p.i++
			continue
		}
		p.i++
		if p.i >= len(p.s) {
			return "", invalid("unterminated escape")
		}
		e := p.s[p.i]
		p.i++
		switch e {
		case '"', '\\', '/':
			out = append(out, e)
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			r, err := p.hex4()
			if err != nil {
				return "", err
			}
			// An unpaired surrogate escape is not among the errors of
			// §4.5.1 item 5; it becomes U+FFFD (a Go string cannot hold it).
			if utf16.IsSurrogate(r) {
				r = p.lowSurrogate(r)
			}
			out = utf8.AppendRune(out, r)
		default:
			return "", invalid("bad escape \\%c", e)
		}
	}
}

// lowSurrogate pairs a high surrogate with a following \uDC00–\uDFFF
// escape, or returns U+FFFD and leaves the input where it was.
func (p *parser) lowSurrogate(high rune) rune {
	if high >= 0xDC00 || len(p.s)-p.i < 6 || p.s[p.i] != '\\' || p.s[p.i+1] != 'u' {
		return utf8.RuneError
	}
	save := p.i
	p.i += 2
	low, err := p.hex4()
	if err == nil {
		if r := utf16.DecodeRune(high, low); r != utf8.RuneError {
			return r
		}
	}
	p.i = save
	return utf8.RuneError
}

func (p *parser) hex4() (rune, error) {
	if len(p.s)-p.i < 4 {
		return 0, invalid("short \\u escape")
	}
	n, err := strconv.ParseUint(string(p.s[p.i:p.i+4]), 16, 32)
	if err != nil {
		return 0, invalid("bad \\u escape")
	}
	p.i += 4
	return rune(n), nil
}
