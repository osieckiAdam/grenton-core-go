package conformance_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/devices"
	"github.com/osieckiAdam/grenton-core-go/internal/normalize"
	"github.com/osieckiAdam/grenton-core-go/internal/wire"
	"github.com/osieckiAdam/grenton-core-go/model"
	"github.com/osieckiAdam/grenton-core-go/provisioning"
)

func init() {
	for op, h := range map[string]operation{
		"crypto.encrypt":         cryptoEncrypt,
		"crypto.decrypt":         cryptoDecrypt,
		"wire.request":           wireRequest,
		"wire.response":          wireResponse,
		"payload.register":       payloadRegister,
		"payload.fetch":          payloadFetch,
		"payload.action":         payloadAction,
		"payload.report":         payloadReport,
		"payload.values":         payloadValues,
		"value.cast":             valueCast,
		"provisioning.url":       provisioningURL,
		"coercion.boolFromOnOff": coercion(normalize.BoolFromOnOff),
		"coercion.truthy":        coercion(normalize.Truthy),
		"number.format":          numberFormat,
		"schedule.rules":         scheduleRules,
		"schedule.cron":          scheduleCron,
	} {
		registry[op] = h
	}
}

func newCipher(input map[string]any) (*clu.AESCipher, error) {
	key, err := str(input, "keyBase64")
	if err != nil {
		return nil, err
	}
	iv, err := str(input, "ivBase64")
	if err != nil {
		return nil, err
	}
	return clu.NewCipher(key, iv)
}

func hexInput(input map[string]any, key string) ([]byte, error) {
	s, err := str(input, key)
	if err != nil {
		return nil, err
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, runnerErr("bad hex in %q", key)
	}
	return b, nil
}

func cryptoEncrypt(_ *suite, input map[string]any) (any, error) {
	plaintext, err := hexInput(input, "plaintextHex")
	if err != nil {
		return nil, err
	}
	c, err := newCipher(input)
	if err != nil {
		return nil, err
	}
	out, err := c.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"ciphertextHex": hex.EncodeToString(out)}), nil
}

func cryptoDecrypt(_ *suite, input map[string]any) (any, error) {
	ciphertext, err := hexInput(input, "ciphertextHex")
	if err != nil {
		return nil, err
	}
	c, err := newCipher(input)
	if err != nil {
		return nil, err
	}
	out, good := c.Decrypt(ciphertext)
	if !good {
		return reject, nil
	}
	return ok(map[string]any{"plaintextHex": hex.EncodeToString(out)}), nil
}

func wireRequest(_ *suite, input map[string]any) (any, error) {
	id, err := str(input, "msgId")
	if err != nil {
		return nil, err
	}
	payload, err := str(input, "payload")
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"frame": wire.Request(id, payload)}), nil
}

func wireResponse(_ *suite, input map[string]any) (any, error) {
	frame, err := str(input, "frame")
	if err != nil {
		return nil, err
	}
	r, good := wire.ParseResponse(frame)
	if !good {
		return reject, nil
	}
	return ok(map[string]any{"cluIp": r.CluIP, "msgId": r.MsgID, "payload": r.Payload,
		"isNotification": r.IsNotification()}), nil
}

// keyRefs decodes the payload.register key format.
func keyRefs(input map[string]any) ([]model.ValueRef, error) {
	keys, err := list(input, "keys")
	if err != nil {
		return nil, err
	}
	refs := make([]model.ValueRef, 0, len(keys))
	for _, k := range keys {
		ref, err := keyRef(k)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func keyRef(k any) (model.ValueRef, error) {
	m, isMap := k.(map[string]any)
	if !isMap {
		return model.ValueRef{}, runnerErr("bad key %v", k)
	}
	switch m["type"] {
	case "variable":
		name, err := str(m, "name")
		return model.Variable("CLU", name), err
	case "attribute":
		obj, err := str(m, "objectName")
		if err != nil {
			return model.ValueRef{}, err
		}
		idx, err := str(m, "index")
		return model.Attribute("CLU", obj, idx), err
	}
	return model.ValueRef{}, runnerErr("bad key %v", k)
}

func payloadRegister(_ *suite, input map[string]any) (any, error) {
	session, err := integer(input, "sessionId")
	if err != nil {
		return nil, err
	}
	refs, err := keyRefs(input)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"payload": wire.ClientRegister(session, refs)}), nil
}

func payloadFetch(_ *suite, input map[string]any) (any, error) {
	refs, err := keyRefs(input)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"payload": wire.FetchValues(refs)}), nil
}

func payloadAction(_ *suite, input map[string]any) (any, error) {
	raw, err := json.Marshal(input["action"])
	if err != nil {
		return nil, runnerErr("payload.action: %v", err)
	}
	action, err := model.ParseActionRef(raw)
	if err != nil {
		return reject, nil // a parse failure (§3.6) is the case's reject
	}
	payload, err := wire.Action(action)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"payload": payload}), nil
}

func payloadReport(_ *suite, input map[string]any) (any, error) {
	payload, err := str(input, "payload")
	if err != nil {
		return nil, err
	}
	r, good := wire.ParseReport(payload)
	if !good {
		return reject, nil
	}
	return ok(map[string]any{"sessionId": r.Session, "values": taggedList(r.Values)}), nil
}

func payloadValues(_ *suite, input map[string]any) (any, error) {
	payload, err := str(input, "payload")
	if err != nil {
		return nil, err
	}
	values, good := wire.ParseValues(payload)
	if !good {
		return reject, nil
	}
	return ok(map[string]any{"values": taggedList(values)}), nil
}

func valueCast(_ *suite, input map[string]any) (any, error) {
	text, err := str(input, "text")
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"value": tagged(model.ParseWireValue(text))}), nil
}

func provisioningURL(_ *suite, input map[string]any) (any, error) {
	host, err := str(input, "host")
	if err != nil {
		return nil, err
	}
	pin, err := str(input, "pin")
	if err != nil {
		return nil, err
	}
	port := int64(provisioning.DefaultPort)
	if _, has := input["port"]; has {
		if port, err = integer(input, "port"); err != nil {
			return nil, err
		}
	}
	url, err := provisioning.EndpointURL(host, int(port), pin)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"url": url}), nil
}

func coercion(f func(model.Value) (bool, bool)) operation {
	return func(_ *suite, input map[string]any) (any, error) {
		v, err := untagged(input["value"])
		if err != nil {
			return nil, err
		}
		b, known := f(v)
		return ok(map[string]any{"result": optionalBool(b, known)}), nil
	}
}

func numberFormat(_ *suite, input map[string]any) (any, error) {
	v, err := untagged(input["value"])
	if err != nil {
		return nil, err
	}
	d, isDouble := v.AsDouble()
	if !isDouble {
		return nil, runnerErr("number.format needs a double")
	}
	p, err := integer(input, "precision")
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"text": normalize.FormatNumber(d, int(p))}), nil
}

func scheduleRules(_ *suite, input map[string]any) (any, error) {
	text, err := str(input, "text")
	if err != nil {
		return nil, err
	}
	rules := devices.ParseScheduleRules(text)
	if rules == nil {
		return ok(map[string]any{"rules": nil}), nil
	}
	out := make([]any, len(rules))
	for i, r := range rules {
		out[i] = ruleJSON(r)
	}
	return ok(map[string]any{"rules": out}), nil
}

// ruleJSON projects a rule as schedule.rules does.
func ruleJSON(r devices.ScheduleRule) map[string]any {
	o := map[string]any{"id": r.ID, "isEnabled": r.Enabled, "cron": r.Cron, "hour": nil, "minute": nil, "days": nil}
	if r.Supported {
		days := make([]any, len(r.Days))
		for i, d := range r.Days {
			days[i] = int(d)
		}
		o["hour"], o["minute"], o["days"] = r.Hour, r.Minute, days
	}
	return o
}

func scheduleCron(_ *suite, input map[string]any) (any, error) {
	hour, err := integer(input, "hour")
	if err != nil {
		return nil, err
	}
	minute, err := integer(input, "minute")
	if err != nil {
		return nil, err
	}
	raw, err := list(input, "days")
	if err != nil {
		return nil, err
	}
	days := make([]time.Weekday, len(raw))
	for i, d := range raw {
		n, isNum := d.(json.Number)
		v, convErr := n.Int64()
		if !isNum || convErr != nil {
			return nil, runnerErr("schedule.cron: day %v", d)
		}
		days[i] = time.Weekday(v)
	}
	text, good := devices.CronText(int(hour), int(minute), days)
	if !good {
		return reject, nil
	}
	return ok(map[string]any{"text": text}), nil
}

// Tagged values (conformance.md §3).

func tagged(v model.Value) map[string]any {
	switch v.Kind() {
	case model.KindBool:
		b, _ := v.AsBool()
		return map[string]any{"type": "bool", "value": b}
	case model.KindInt:
		i, _ := v.AsInt()
		return map[string]any{"type": "int", "value": strconv.FormatInt(i, 10)}
	case model.KindDouble:
		f, _ := v.AsDouble()
		return map[string]any{"type": "double", "bits": fmt.Sprintf("%016x", math.Float64bits(f))}
	case model.KindString:
		s, _ := v.AsString()
		return map[string]any{"type": "string", "value": s}
	default:
		return map[string]any{"type": "null"}
	}
}

func taggedList(values []model.Value) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = tagged(v)
	}
	return out
}

func untagged(x any) (model.Value, error) {
	m, isMap := x.(map[string]any)
	if !isMap {
		return model.Value{}, runnerErr("bad tagged value %v", x)
	}
	switch m["type"] {
	case "null":
		return model.Null(), nil
	case "bool":
		if b, isBool := m["value"].(bool); isBool {
			return model.Bool(b), nil
		}
	case "int":
		if s, isStr := m["value"].(string); isStr {
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return model.Int(i), nil
			}
		}
	case "string":
		if s, isStr := m["value"].(string); isStr {
			return model.String(s), nil
		}
	case "double":
		if s, isStr := m["bits"].(string); isStr {
			if bits, err := strconv.ParseUint(s, 16, 64); err == nil {
				return model.Double(math.Float64frombits(bits)), nil
			}
		}
	}
	return model.Value{}, runnerErr("bad tagged value %v", x)
}

func optionalBool(b, known bool) any {
	if !known {
		return nil
	}
	return b
}

// errorCodeOf maps the port's errors onto the suite's codes; "" when the
// error belongs to no known category.
func errorCodeOf(err error) string {
	var pe *provisioning.Error
	switch {
	case errors.As(err, &pe):
		return "provisioning." + pe.Kind.String()
	case errors.Is(err, clu.ErrInvalidKeyMaterial):
		return "crypto.invalidKeyMaterial"
	case errors.Is(err, clu.ErrEncryptFailed):
		return "crypto.encryptFailed"
	case errors.Is(err, wire.ErrUnresolvedPlaceholder):
		return "payload.unresolvedPlaceholder"
	}
	return ""
}
