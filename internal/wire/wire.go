// Package wire is the frame and payload format of the Runtime module (spec
// §5.2–§5.3): request frames, response parsing, the payloads of every
// request and the parsers of report and values payloads. It is internal:
// applications talk to a clu.Session.
package wire

import (
	"errors"
	"strconv"
	"strings"

	"github.com/osieckiAdam/grenton-core-go/model"
)

// NotificationID is the msg_id of unprompted notifications (spec §5.2).
const NotificationID = "00000000"

// DefaultFrameLimit is the register frame limit in UTF-8 bytes (spec §5.3.2).
const DefaultFrameLimit = 1471

// ErrUnresolvedPlaceholder: an action whose value is exactly $value$ was
// sent without an override (spec §3.3); nothing reaches the CLU.
var ErrUnresolvedPlaceholder = errors.New("action value $value$ needs an override")

// Request is the frame req::{msgId}:{payload}, msgId in lower case.
func Request(msgID, payload string) string {
	return "req::" + strings.ToLower(msgID) + ":" + payload
}

// Response is a parsed resp frame.
type Response struct {
	CluIP   string
	MsgID   string // lower case
	Payload string
}

// IsNotification reports msg_id 00000000.
func (r Response) IsNotification() bool { return r.MsgID == NotificationID }

// IsClientReport reports a notification carrying a clientReport payload.
func (r Response) IsClientReport() bool {
	return r.IsNotification() && strings.HasPrefix(r.Payload, "clientReport:")
}

// ParseResponse splits resp:{ip}:{msgId}:{payload} on the first three
// colons; anything else is not a response (ok false).
func ParseResponse(frame string) (Response, bool) {
	parts := strings.SplitN(frame, ":", 4)
	if len(parts) != 4 || parts[0] != "resp" {
		return Response{}, false
	}
	return Response{CluIP: parts[1], MsgID: strings.ToLower(parts[2]), Payload: parts[3]}, true
}

// CheckAlive is the ping payload (spec §5.3.1).
const CheckAlive = "checkAlive()"

// Key is a subscription key on the wire (spec §3.2): "name" for a variable,
// {object,index} for an attribute.
func Key(r model.ValueRef) string {
	if r.CallType == model.ValueVariable {
		return `"` + r.Index + `"`
	}
	return "{" + r.ObjectName + "," + r.Index + "}"
}

func keyList(keys []model.ValueRef) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = Key(k)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// ClientRegister is SYSTEM:clientRegister(0,0,{session},{keys}).
func ClientRegister(session int64, keys []model.ValueRef) string {
	return "SYSTEM:clientRegister(0,0," + strconv.FormatInt(session, 10) + "," + keyList(keys) + ")"
}

// ClientDestroy is SYSTEM:clientDestroy(0,0,{session}).
func ClientDestroy(session int64) string {
	return "SYSTEM:clientDestroy(0,0," + strconv.FormatInt(session, 10) + ")"
}

// FetchValues is SYSTEM:fetchValues({keys}); keys must not be empty.
func FetchValues(keys []model.ValueRef) string {
	return "SYSTEM:fetchValues(" + keyList(keys) + ")"
}

// Action is the payload of an action (spec §5.3.4), its fields verbatim.
func Action(a model.ActionRef) (string, error) {
	if a.Value == model.ValuePlaceholder {
		return "", ErrUnresolvedPlaceholder
	}
	switch a.CallType {
	case model.ActionVariable:
		return `setVar("` + a.Index + `","` + a.Value + `")`, nil
	case model.ActionAttribute:
		return a.ObjectName + ":set(" + a.Index + `,"` + a.Value + `")`, nil
	case model.ActionMethod:
		return a.ObjectName + ":execute(" + a.Index + `,"` + a.Value + `")`, nil
	default:
		return a.ObjectName + "(" + a.Value + ")", nil
	}
}

// Chunks splits keys, in order, into the longest runs whose frame
// req::{8 hex}:{payload(run)} fits limit bytes (spec §5.4.2 item 1); a key
// too large on its own forms its own chunk. payload builds the frame text
// of a run; the msg_id counts as 8 characters.
func Chunks(keys []model.ValueRef, limit int, payload func([]model.ValueRef) string) [][]model.ValueRef {
	var out [][]model.ValueRef
	start := 0
	for start < len(keys) {
		end := start + 1
		for end < len(keys) && len(Request("00000000", payload(keys[start:end+1]))) <= limit {
			end++
		}
		out = append(out, keys[start:end])
		start = end
	}
	return out
}

// RegisterChunks splits for clientRegister, the number counted as 6 digits.
func RegisterChunks(keys []model.ValueRef, limit int) [][]model.ValueRef {
	return Chunks(keys, limit, func(run []model.ValueRef) string { return ClientRegister(999999, run) })
}

// FetchChunks splits for fetchValues.
func FetchChunks(keys []model.ValueRef, limit int) [][]model.ValueRef {
	return Chunks(keys, limit, FetchValues)
}

// Report is a parsed clientReport payload.
type Report struct {
	Session int64
	Values  []model.Value
}

// ParseReport parses clientReport:{session}:{v1,…} (spec §5.3.2 items 1–3).
// ok is false for a rejected payload; a body without its outer braces or
// with empty content yields an empty value list.
func ParseReport(payload string) (Report, bool) {
	rest, found := strings.CutPrefix(payload, "clientReport:")
	if !found {
		return Report{}, false
	}
	number, body, found := strings.Cut(rest, ":")
	if !found {
		return Report{}, false
	}
	session, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return Report{}, false
	}
	values, ok := parseBody(body)
	if !ok {
		return Report{}, false
	}
	return Report{Session: session, Values: values}, true
}

// ParseValues parses values:{v1,…}, the reply to fetchValues.
func ParseValues(payload string) ([]model.Value, bool) {
	body, found := strings.CutPrefix(payload, "values:")
	if !found {
		return nil, false
	}
	if !strings.HasPrefix(body, "{") || !strings.HasSuffix(body, "}") || len(body) < 2 {
		return nil, false
	}
	values, ok := parseBody(body)
	if !ok || len(values) == 0 {
		return nil, false
	}
	return values, true
}

// parseBody reads {v1,…}: elements split on commas outside text and nested
// braces, a quoted Lua table read to its balancing brace. Without the outer
// braces, or with empty content, it is an empty list; unclosed text or an
// unbalanced brace inside rejects it.
func parseBody(body string) ([]model.Value, bool) {
	if len(body) < 2 || body[0] != '{' || body[len(body)-1] != '}' {
		return []model.Value{}, true
	}
	content := body[1 : len(body)-1]
	if content == "" {
		return []model.Value{}, true
	}
	var values []model.Value
	depth, start := 0, 0
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '"':
			end, ok := textEnd(content, i)
			if !ok {
				return nil, false
			}
			i = end
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return nil, false
			}
			depth--
		case ',':
			if depth == 0 {
				values = append(values, element(content[start:i]))
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, false
	}
	return append(values, element(content[start:])), true
}

// textEnd returns the index of the quote closing the text opened at open.
// Text that begins with "{ is a quoted table: it ends at the } balancing its
// first { when a " follows it; otherwise text ends at the next quote.
func textEnd(s string, open int) (int, bool) {
	if open+1 < len(s) && s[open+1] == '{' {
		depth := 0
		for j := open + 1; j < len(s); j++ {
			switch s[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					if j+1 < len(s) && s[j+1] == '"' {
						return j + 1, true
					}
					j = len(s) // no closing quote after the balancing brace
				}
			}
		}
	}
	next := strings.IndexByte(s[open+1:], '"')
	if next < 0 {
		return 0, false
	}
	return open + 1 + next, true
}

// element is one value of a report (spec §5.3.2 item 2): trim, strip the
// quotes, then the cast of §3.1 (which trims and strips again).
func element(text string) model.Value {
	return model.ParseWireValue(strings.Trim(model.TrimSpace(text), `"`))
}
