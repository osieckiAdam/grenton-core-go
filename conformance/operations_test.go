package conformance_test

import (
	"encoding/json"
	"fmt"
)

// registry maps every operation of the suite to its handler. Each handler
// decodes the case input, calls the production API and projects the
// observation; no protocol logic lives here.
var registry = map[string]operation{}

// errorCode projects the port's errors onto the codes of conformance.md §2.
func errorCode(err error) string {
	return fmt.Sprintf("unmapped: %v", err)
}

// Input accessors: a missing or mistyped field is a runner error.

func str(input map[string]any, key string) (string, error) {
	s, ok := input[key].(string)
	if !ok {
		return "", runnerErr("missing string input %q", key)
	}
	return s, nil
}

func integer(input map[string]any, key string) (int64, error) {
	n, ok := input[key].(json.Number)
	if !ok {
		return 0, runnerErr("missing integer input %q", key)
	}
	i, err := n.Int64()
	if err != nil {
		return 0, runnerErr("input %q is not an integer: %v", key, n)
	}
	return i, nil
}

func object(input map[string]any, key string) (map[string]any, error) {
	o, ok := input[key].(map[string]any)
	if !ok {
		return nil, runnerErr("missing object input %q", key)
	}
	return o, nil
}

func list(input map[string]any, key string) ([]any, error) {
	l, ok := input[key].([]any)
	if !ok {
		return nil, runnerErr("missing list input %q", key)
	}
	return l, nil
}
