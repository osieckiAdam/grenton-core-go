package strictjson

import (
	"errors"
	"strings"
	"testing"
)

func TestRejects(t *testing.T) {
	for name, text := range map[string]string{
		"repeated key":      `{"a": 1, "a": 2}`,
		"NaN":               `[NaN]`,
		"Infinity":          `[Infinity]`,
		"integer overflow":  `[9223372036854775808]`,
		"non-finite double": `[1e400]`,
		"leading zero":      `[01]`,
		"trailing comma":    `[1,]`,
		"trailing data":     `{} {}`,
		"bad UTF-8":         "[\"\xff\"]",
		"too deep":          strings.Repeat("[", MaxDepth+1) + strings.Repeat("]", MaxDepth+1),
	} {
		if _, err := Parse([]byte(text)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
}

// Ten thousand levels end in an error, not a stack overflow (§4.5.1 item 5).
func TestDeepInputIsAnError(t *testing.T) {
	deep := strings.Repeat(`{"a":`, 10000) + "1" + strings.Repeat("}", 10000)
	if _, err := Parse([]byte(deep)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestAcceptsAtTheLimit(t *testing.T) {
	text := strings.Repeat("[", MaxDepth) + strings.Repeat("]", MaxDepth)
	if _, err := Parse([]byte(text)); err != nil {
		t.Fatal(err)
	}
}

func TestValues(t *testing.T) {
	v, err := Parse([]byte(`{"i": -5, "f": 2.5, "e": 1e2, "s": "aé😀", "b": true, "n": null, "l": [1, {}]}`))
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[string]any)
	if m["i"] != int64(-5) || m["f"] != 2.5 || m["e"] != 100.0 || m["s"] != "aé😀" || m["b"] != true || m["n"] != nil {
		t.Fatalf("got %#v", m)
	}
	if l := m["l"].([]any); len(l) != 2 || l[0] != int64(1) {
		t.Fatalf("got %#v", m["l"])
	}
}

func TestUnpairedSurrogateIsReplacementCharacter(t *testing.T) {
	v, err := Parse([]byte(`["\ud800x", "\udc00"]`))
	if err != nil {
		t.Fatal(err)
	}
	if l := v.([]any); l[0] != "�x" || l[1] != "�" {
		t.Fatalf("got %q", l)
	}
}
