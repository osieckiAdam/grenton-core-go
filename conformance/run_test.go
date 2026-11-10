package conformance_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
)

// suiteRoot is the vendored suite; port.json declares what this port claims.
var (
	suiteRoot = filepath.Join("..", "vendor", "grenton-core-spec", "conformance")
	portPath  = "port.json"
)

var loaded struct {
	once  sync.Once
	suite *suite
	err   error
}

func theSuite(t *testing.T) *suite {
	t.Helper()
	loaded.once.Do(func() { loaded.suite, loaded.err = loadSuite(suiteRoot, portPath) })
	if loaded.err != nil {
		t.Fatalf("conformance suite: %v", loaded.err)
	}
	return loaded.suite
}

// errRunner marks a failure of the runner or of the case input, as opposed
// to an outcome of the port: it fails the case instead of being compared.
type errRunner struct{ msg string }

func (e errRunner) Error() string { return e.msg }

func runnerErr(format string, args ...any) error { return errRunner{fmt.Sprintf(format, args...)} }

// operation runs one case input through the production API and returns the
// observed expectation-shaped outcome.
type operation func(s *suite, input map[string]any) (any, error)

func TestSuiteIsPresentIntactAndDeclared(t *testing.T) {
	s := theSuite(t)
	if len(s.Cases) == 0 {
		t.Fatal("the suite holds no cases")
	}
	var unclaimed int
	for _, c := range s.Cases {
		if !s.declared(c) {
			unclaimed++
		}
	}
	if unclaimed > 0 {
		t.Logf("conformance: %d case(s) outside the declared profiles %v are not covered by the declaration", unclaimed, s.Port.Profiles)
	}
	for op := range operationsOf(s) {
		if _, ok := registry[op]; !ok {
			t.Errorf("declared operation %s has no handler", op)
		}
	}
}

// operationsOf lists the operations of the declared profiles.
func operationsOf(s *suite) map[string]bool {
	ops := map[string]bool{}
	for _, p := range s.Port.Profiles {
		for _, op := range s.Profiles[p] {
			ops[op] = true
		}
	}
	return ops
}

func TestConformance(t *testing.T) {
	s := theSuite(t)
	for _, id := range s.sortedIDs() {
		c := s.Cases[id]
		if !s.declared(c) {
			continue
		}
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			diff, observed, expected, err := runCase(s, c)
			if _, skipped := s.skip(id); skipped {
				if err == nil && diff == "" {
					t.Fatalf("%s is skipped in port.json but passes — remove the skip", id)
				}
				t.Skipf("skipped in port.json")
			}
			if err != nil {
				t.Fatalf("runner: %v", err)
			}
			if diff != "" {
				t.Fatalf("differs at %s\n  expected: %s\n  observed: %s", diff, compact(expected), compact(observed))
			}
		})
	}
}

// runCase returns the first differing JSON path ("" when equal).
func runCase(s *suite, c suiteCase) (diff string, observed, expected any, err error) {
	op, ok := registry[c.Operation]
	if !ok {
		return "", nil, nil, runnerErr("no handler for operation %s", c.Operation)
	}
	got, err := op(s, c.Input)
	if err != nil {
		var r errRunner
		if errors.As(err, &r) {
			return "", nil, nil, err
		}
		got = map[string]any{"outcome": "error", "code": errorCode(err)}
	}
	observed, err = normalize(got)
	if err != nil {
		return "", nil, nil, runnerErr("observation does not serialize: %v", err)
	}
	expected, _ = normalize(dropDecimals(c.Expect))
	return firstDifference(expected, observed, "$"), observed, expected, nil
}

// Projection helpers shared by the operations.

func ok(value map[string]any) map[string]any {
	return map[string]any{"outcome": "ok", "value": value}
}

var reject = map[string]any{"outcome": "reject"}

// normalize round-trips a value through JSON so that observations built
// from Go values compare with decoded expectations.
func normalize(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := decodeJSON(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// dropDecimals removes the informative `decimal` of tagged doubles; only the
// bits are normative (conformance.md §3).
func dropDecimals(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			if k == "decimal" && x["type"] == "double" {
				continue
			}
			out[k] = dropDecimals(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = dropDecimals(e)
		}
		return out
	default:
		return v
	}
}

// firstDifference compares structurally: object key order is irrelevant,
// arrays are positional, numbers compare by value ("1.0" equals "1").
func firstDifference(a, b any, at string) string {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return at
		}
		keys := map[string]bool{}
		for k := range x {
			keys[k] = true
		}
		for k := range y {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			xv, inX := x[k]
			yv, inY := y[k]
			if !inX || !inY {
				return at + "." + k
			}
			if d := firstDifference(xv, yv, at+"."+k); d != "" {
				return d
			}
		}
		return ""
	case []any:
		y, ok := b.([]any)
		if !ok {
			return at
		}
		if len(x) != len(y) {
			return fmt.Sprintf("%s (length %d vs %d)", at, len(x), len(y))
		}
		for i := range x {
			if d := firstDifference(x[i], y[i], fmt.Sprintf("%s[%d]", at, i)); d != "" {
				return d
			}
		}
		return ""
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return at
		}
		if x == y {
			return ""
		}
		fx, ex := strconv.ParseFloat(string(x), 64)
		fy, ey := strconv.ParseFloat(string(y), 64)
		if ex != nil || ey != nil || fx != fy {
			return at
		}
		return ""
	default:
		if a != b {
			return at
		}
		return ""
	}
}

func compact(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

func TestMain(m *testing.M) {
	if _, err := os.Stat(suiteRoot); err != nil {
		fmt.Fprintf(os.Stderr, "conformance suite not found at %s — run `git submodule update --init`\n", suiteRoot)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
