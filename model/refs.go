package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// ValueCallType says where a [ValueRef] reads from (spec §3.2).
type ValueCallType string

// The two state reference variants.
const (
	ValueVariable  ValueCallType = "VARIABLE"
	ValueAttribute ValueCallType = "ATTRIBUTE"
)

// ValueRef says where to read a value from on a CLU (spec §3.2): a global
// variable (Index is its name) or a feature Index of the object ObjectName.
// For a variable ObjectName is parsed but carries no meaning.
type ValueRef struct {
	CluID      string
	ObjectName string
	Index      string
	CallType   ValueCallType
}

// Variable returns a reference to the CLU global variable name.
func Variable(cluID, name string) ValueRef {
	return ValueRef{CluID: cluID, Index: name, CallType: ValueVariable}
}

// Attribute returns a reference to feature index of the object objectName.
func Attribute(cluID, objectName, index string) ValueRef {
	return ValueRef{CluID: cluID, ObjectName: objectName, Index: index, CallType: ValueAttribute}
}

// Key identifies the value a reference reads, ignoring a variable's
// ObjectName: two references with the same Key read the same value.
func (r ValueRef) Key() ValueRef {
	if r.CallType == ValueVariable {
		return ValueRef{CluID: r.CluID, Index: r.Index, CallType: ValueVariable}
	}
	return r
}

// ActionEvent is the UI role of an action in its widget (spec §3.3).
type ActionEvent string

// The three action events.
const (
	EventClick ActionEvent = "CLICK"
	EventOn    ActionEvent = "ON"
	EventOff   ActionEvent = "OFF"
)

// ActionCallType is the execution variant of an [ActionRef] (spec §3.3).
type ActionCallType string

// The four action variants.
const (
	ActionVariable  ActionCallType = "VARIABLE"
	ActionAttribute ActionCallType = "ATTRIBUTE"
	ActionMethod    ActionCallType = "METHOD"
	ActionScript    ActionCallType = "SCRIPT"
)

// ValuePlaceholder is the value Object Manager exports for setter actions;
// such an action is sent only with a value an intent provides (spec §3.3).
const ValuePlaceholder = "$value$"

// ActionRef says what to send to a CLU (spec §3.3). A script has no Index.
// ActionRef is a value: overriding Value on a copy never changes the
// widget's own action.
type ActionRef struct {
	Event      ActionEvent
	CluID      string
	ObjectName string
	Index      string // empty and meaningless for ActionScript
	Value      string
	CallType   ActionCallType
}

// WithValue returns a copy of the action with Value replaced.
func (a ActionRef) WithValue(value string) ActionRef {
	a.Value = value
	return a
}

// ParseActionRef decodes an action exactly as it appears in data.json
// (spec §3.3, §3.6): every field is required, except index for a script.
func ParseActionRef(data []byte) (ActionRef, error) {
	o, err := decodeObject(data)
	if err != nil {
		return ActionRef{}, err
	}
	return o.actionRef()
}

// errField is the decoding error of one field.
type errField struct {
	field  string
	reason string
}

func (e errField) Error() string { return e.field + ": " + e.reason }

var errNotObject = errors.New("not a JSON object")

// object is a decoded JSON object whose fields are decoded on demand, so
// that a missing or mistyped field names itself.
type object map[string]json.RawMessage

func decodeObject(data []byte) (object, error) {
	var o object
	if err := json.Unmarshal(data, &o); err != nil || o == nil {
		return nil, errNotObject
	}
	return o, nil
}

// present reports whether key holds a non-null value.
func (o object) present(key string) bool {
	raw, ok := o[key]
	return ok && string(raw) != "null"
}

func (o object) str(key string) (string, error) {
	if !o.present(key) {
		return "", errField{key, "missing"}
	}
	var s string
	if err := json.Unmarshal(o[key], &s); err != nil {
		return "", errField{key, "not a string"}
	}
	return s, nil
}

// optStr returns "" and false for a missing or null field.
func (o object) optStr(key string) (string, bool, error) {
	if !o.present(key) {
		return "", false, nil
	}
	s, err := o.str(key)
	return s, err == nil, err
}

func (o object) boolean(key string) (bool, error) {
	if !o.present(key) {
		return false, errField{key, "missing"}
	}
	var b bool
	if err := json.Unmarshal(o[key], &b); err != nil {
		return false, errField{key, "not a bool"}
	}
	return b, nil
}

// integer accepts a JSON number with an integral value within Int64.
func (o object) integer(key string) (int64, error) {
	if !o.present(key) {
		return 0, errField{key, "missing"}
	}
	var n json.Number
	if err := json.Unmarshal(o[key], &n); err != nil {
		return 0, errField{key, "not a number"}
	}
	if i, err := n.Int64(); err == nil {
		return i, nil
	}
	f, err := n.Float64()
	if err != nil || f != math.Trunc(f) || f < -9223372036854775808 || f >= 9223372036854775808 {
		return 0, errField{key, "not an integer"}
	}
	return int64(f), nil
}

func (o object) double(key string) (float64, error) {
	if !o.present(key) {
		return 0, errField{key, "missing"}
	}
	var f float64
	if err := json.Unmarshal(o[key], &f); err != nil {
		return 0, errField{key, "not a number"}
	}
	return f, nil
}

func (o object) child(key string) (object, error) {
	if !o.present(key) {
		return nil, errField{key, "missing"}
	}
	c, err := decodeObject(o[key])
	if err != nil {
		return nil, errField{key, "not an object"}
	}
	return c, nil
}

func (o object) array(key string) ([]json.RawMessage, error) {
	if !o.present(key) {
		return nil, errField{key, "missing"}
	}
	var a []json.RawMessage
	if err := json.Unmarshal(o[key], &a); err != nil {
		return nil, errField{key, "not an array"}
	}
	return a, nil
}

// enum reads a string that must be one of allowed.
func (o object) enum(key string, allowed ...string) (string, error) {
	s, err := o.str(key)
	if err != nil {
		return "", err
	}
	for _, a := range allowed {
		if s == a {
			return s, nil
		}
	}
	return "", errField{key, fmt.Sprintf("unknown value %q", s)}
}

func (o object) valueRef() (ValueRef, error) {
	var r ValueRef
	var err error
	if r.CluID, err = o.str("cluId"); err != nil {
		return r, err
	}
	if r.ObjectName, err = o.str("objectName"); err != nil {
		return r, err
	}
	if r.Index, err = o.str("index"); err != nil {
		return r, err
	}
	ct, err := o.enum("callType", string(ValueVariable), string(ValueAttribute))
	r.CallType = ValueCallType(ct)
	return r, err
}

func (o object) actionRef() (ActionRef, error) {
	var a ActionRef
	ev, err := o.enum("event", string(EventClick), string(EventOn), string(EventOff))
	if err != nil {
		return a, err
	}
	a.Event = ActionEvent(ev)
	if a.CluID, err = o.str("cluId"); err != nil {
		return a, err
	}
	if a.ObjectName, err = o.str("objectName"); err != nil {
		return a, err
	}
	if a.Value, err = o.str("value"); err != nil {
		return a, err
	}
	ct, err := o.enum("callType", string(ActionVariable), string(ActionAttribute), string(ActionMethod), string(ActionScript))
	if err != nil {
		return a, err
	}
	a.CallType = ActionCallType(ct)
	index, has, err := o.optStr("index")
	if err != nil {
		return a, err
	}
	if !has && a.CallType != ActionScript {
		return a, errField{"index", "required for callType " + ct}
	}
	if a.CallType != ActionScript {
		a.Index = index
	}
	return a, nil
}

// refField decodes the ValueRef under key.
func (o object) refField(key string) (ValueRef, error) {
	c, err := o.child(key)
	if err != nil {
		return ValueRef{}, err
	}
	r, err := c.valueRef()
	if err != nil {
		return r, errField{key, err.Error()}
	}
	return r, nil
}

// optRef decodes an optional ValueRef: absent or null is (nil, nil).
func (o object) optRef(key string) (*ValueRef, error) {
	if !o.present(key) {
		return nil, nil
	}
	r, err := o.refField(key)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (o object) actionField(key string) (ActionRef, error) {
	c, err := o.child(key)
	if err != nil {
		return ActionRef{}, err
	}
	a, err := c.actionRef()
	if err != nil {
		return a, errField{key, err.Error()}
	}
	return a, nil
}

func (o object) optAction(key string) (*ActionRef, error) {
	if !o.present(key) {
		return nil, nil
	}
	a, err := o.actionField(key)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (o object) actions(key string) ([]ActionRef, error) {
	raws, err := o.array(key)
	if err != nil {
		return nil, err
	}
	out := make([]ActionRef, 0, len(raws))
	for n, raw := range raws {
		c, err := decodeObject(raw)
		if err != nil {
			return nil, errField{fmt.Sprintf("%s[%d]", key, n), "not an object"}
		}
		a, err := c.actionRef()
		if err != nil {
			return nil, errField{fmt.Sprintf("%s[%d]", key, n), err.Error()}
		}
		out = append(out, a)
	}
	return out, nil
}
