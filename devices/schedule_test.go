package devices

import "testing"

// §6.3.13: a row is exactly three fields; an extra or embedded quote makes
// it another shape, so no rules are known.
func TestRuleRowWithAnotherShape(t *testing.T) {
	for _, text := range []string{`{{1, 1, "0 20 * * *", "x"}}`, `{{1, 1, "0 20 "a" * *"}}`} {
		if r := ParseScheduleRules(text); r != nil {
			t.Errorf("%s: got %+v, want nil", text, r)
		}
	}
}
