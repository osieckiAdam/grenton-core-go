// Package normalize holds the value rules of the Devices module (spec §6.1,
// §6.2, §10 item 3): coercions, integer reads, range mapping and the number
// format of action payloads. It is internal so that the rules stay in one
// place without becoming public API.
package normalize

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/osieckiAdam/grenton-core-go/model"
)

// BoolFromOnOff is the switch coercion of spec §6.2: true for "ON", true, or
// a number equal to 1; false for anything else; unknown (ok false) for null.
func BoolFromOnOff(v model.Value) (on, ok bool) {
	switch v.Kind() {
	case model.KindNull:
		return false, false
	case model.KindString:
		s, _ := v.AsString()
		return s == "ON", true
	case model.KindBool:
		b, _ := v.AsBool()
		return b, true
	default:
		n, _ := v.Number()
		return n == 1, true
	}
}

// Truthy is Python truthiness (spec §6.2): 0, 0.0, "" and false are false;
// null is unknown (ok false).
func Truthy(v model.Value) (truth, ok bool) {
	switch v.Kind() {
	case model.KindNull:
		return false, false
	case model.KindString:
		s, _ := v.AsString()
		return s != "", true
	case model.KindBool:
		b, _ := v.AsBool()
		return b, true
	default:
		n, _ := v.Number()
		return n != 0, true
	}
}

// Integer is the integer read of spec §6.2: an int as it is, a double
// truncated toward zero; a non-finite double, one whose truncation leaves
// Int64, or any other type is unknown (ok false). It never panics.
func Integer(v model.Value) (int64, bool) {
	if i, ok := v.AsInt(); ok {
		return i, true
	}
	f, ok := v.AsDouble()
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	t := math.Trunc(f)
	// 2^63 is exactly representable; anything at or above it overflows.
	if t < -9223372036854775808 || t >= 9223372036854775808 {
		return 0, false
	}
	return int64(t), true
}

// MapRange maps v from (a,b) to (c,d); c when b−a ≤ 0 (spec §6.1). It does
// not clamp.
func MapRange(a, b, c, d, v float64) float64 {
	if b-a <= 0 {
		return c
	}
	return c + (v-a)/(b-a)*(d-c)
}

// Clamp limits v to [lo, hi].
func Clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

// Round rounds v to precision decimal places (negative: tens, hundreds…)
// like Python's round(v, precision): the exact binary value, halves to even,
// then the nearest double. The sign of a zero result is v's.
func Round(v float64, precision int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	// Beyond ±400 places the result is v itself or a zero, as in Python;
	// the cap keeps an absurd precision from data.json from costing memory.
	switch {
	case precision > 400:
		return v
	case precision < -400:
		return math.Copysign(0, v)
	}
	exact := new(big.Rat).SetFloat64(v)
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(precision))), nil))
	if precision >= 0 {
		exact.Mul(exact, scale)
	} else {
		exact.Quo(exact, scale)
	}
	rounded := new(big.Rat).SetInt(roundHalfEven(exact))
	if precision >= 0 {
		rounded.Quo(rounded, scale)
	} else {
		rounded.Mul(rounded, scale)
	}
	f, _ := rounded.Float64()
	if f == 0 {
		return math.Copysign(0, v)
	}
	return f
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// roundHalfEven rounds a rational to the nearest integer, halves to even.
func roundHalfEven(r *big.Rat) *big.Int {
	num, den := r.Num(), r.Denom()
	q, m := new(big.Int).QuoRem(num, den, new(big.Int)) // truncated toward zero
	twice := new(big.Int).Mul(new(big.Int).Abs(m), big.NewInt(2))
	switch twice.Cmp(den) {
	case 1:
		q.Add(q, big.NewInt(int64(num.Sign())))
	case 0:
		if q.Bit(0) == 1 {
			q.Add(q, big.NewInt(int64(num.Sign())))
		}
	}
	return q
}

// FormatNumber is the number format of action payloads (spec §10 item 3):
// Python's str(round(v, precision)), e.g. "5.0" at precision 0.
func FormatNumber(v float64, precision int) string {
	return PythonRepr(Round(v, precision))
}

// PythonRepr formats a double as Python's repr: the shortest digits that
// round-trip, in fixed notation for 1e-4 ≤ |v| < 1e16 (always with a
// fractional part) and in scientific notation ("1e+16", "1.5e-05") outside.
func PythonRepr(v float64) string {
	switch {
	case math.IsNaN(v):
		return "nan"
	case math.IsInf(v, 1):
		return "inf"
	case math.IsInf(v, -1):
		return "-inf"
	}
	sign := ""
	if math.Signbit(v) {
		sign, v = "-", -v
	}
	if v == 0 {
		return sign + "0.0"
	}
	e := strconv.FormatFloat(v, 'e', -1, 64) // d.ddde±XX
	mantissa, expText, _ := strings.Cut(e, "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	exp, _ := strconv.Atoi(expText)
	decpt := exp + 1 // digits before the decimal point
	if decpt > -4 && decpt <= 16 {
		switch {
		case decpt <= 0:
			return sign + "0." + strings.Repeat("0", -decpt) + digits
		case decpt >= len(digits):
			return sign + digits + strings.Repeat("0", decpt-len(digits)) + ".0"
		default:
			return sign + digits[:decpt] + "." + digits[decpt:]
		}
	}
	out := digits[:1]
	if len(digits) > 1 {
		out += "." + digits[1:]
	}
	expSign := "+"
	if exp < 0 {
		expSign, exp = "-", -exp
	}
	expDigits := strconv.Itoa(exp)
	if len(expDigits) < 2 {
		expDigits = "0" + expDigits
	}
	return sign + out + "e" + expSign + expDigits
}
