package conformance_test

// The session profile (conformance.md §4b): a real clu.Session against a
// CLU the runner plays, on a virtual clock. The barrier is exact: the
// session reports its runnable work through clu.Activity, the runner counts
// its own call goroutines, datagrams are delivered and timers fire
// synchronously, so "count = 0" means the session cannot do anything more
// without time or a datagram. Negative assertions need no quiet window.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/internal/clutest"
	"github.com/osieckiAdam/grenton-core-go/model"
)

func init() {
	registry["session.scenario"] = sessionScenario
}

// Key and IV of spec §9.1: scenarios work on plaintext frames.
const (
	scenarioKey = "AAECAwQFBgcICQoLDA0ODw=="
	scenarioIV  = "EBESExQVFhcYGRobHB0eHw=="
)

// scenario runs one session.scenario case.
type scenario struct {
	act       *clutest.Activity
	clock     *clutest.Clock
	transport *clutest.Transport
	cipher    *clu.AESCipher
	session   *clu.Session
	cluIP     string
	reports   <-chan clu.Report
	labels    map[string]*clutest.Socket
	captures  map[string]string
	calls     map[string]chan any
}

func sessionScenario(_ *suite, input map[string]any) (any, error) {
	cluIn, err := object(input, "clu")
	if err != nil {
		return nil, err
	}
	id, err := str(cluIn, "id")
	if err != nil {
		return nil, err
	}
	ip, err := str(cluIn, "ip")
	if err != nil {
		return nil, err
	}
	port, err := integer(cluIn, "port")
	if err != nil {
		return nil, err
	}
	timeout, err := integer(input, "responseTimeoutMs")
	if err != nil {
		return nil, err
	}
	optMs := func(key string) (time.Duration, error) {
		if _, has := input[key]; !has {
			return 0, nil // the port's default (conformance.md §4b)
		}
		ms, err := integer(input, key)
		return time.Duration(ms) * time.Millisecond, err
	}
	pingInterval, err := optMs("pingIntervalMs")
	if err != nil {
		return nil, err
	}
	registerInterval, err := optMs("registerIntervalMs")
	if err != nil {
		return nil, err
	}
	var frameLimit int64
	if _, has := input["registerFrameLimitBytes"]; has {
		if frameLimit, err = integer(input, "registerFrameLimitBytes"); err != nil {
			return nil, err
		}
	}
	steps, err := list(input, "steps")
	if err != nil {
		return nil, err
	}
	cipher, err := clu.NewCipher(scenarioKey, scenarioIV)
	if err != nil {
		return nil, err
	}
	act := clutest.NewActivity()
	sc := &scenario{act: act, clock: clutest.NewClock(act),
		transport: &clutest.Transport{}, cipher: cipher, cluIP: ip,
		labels: map[string]*clutest.Socket{}, captures: map[string]string{}, calls: map[string]chan any{}}
	sc.session = clu.NewSession(clu.Config{
		CLU:    model.Clu{ID: id, Name: id, IP: ip, Port: port, ConnectionType: model.LocalOnly},
		Cipher: cipher, Transport: sc.transport, Clock: sc.clock, Activity: act,
		ResponseTimeout: time.Duration(timeout) * time.Millisecond,
		PingInterval:    pingInterval, RegisterInterval: registerInterval, FrameLimit: int(frameLimit),
	})
	var cancel func()
	sc.reports, cancel = sc.session.Reports()
	defer cancel()
	defer sc.session.Stop()

	for i, step := range steps {
		m, isMap := step.(map[string]any)
		if !isMap {
			return nil, runnerErr("step %d is not an object", i)
		}
		if err := sc.perform(m); err != nil {
			var r errRunner
			if errors.As(err, &r) {
				return nil, err
			}
			return ok(map[string]any{"completed": false, "failedStep": i, "observed": err.Error()}), nil
		}
	}
	// The end of a scenario is an implicit expectQuiet and expectNoReport.
	for _, end := range []map[string]any{{"expectQuiet": true}, {"expectNoReport": true}} {
		if err := sc.perform(end); err != nil {
			return ok(map[string]any{"completed": false, "failedStep": len(steps), "observed": err.Error()}), nil
		}
	}
	return ok(map[string]any{"completed": true}), nil
}

func (sc *scenario) perform(step map[string]any) error {
	if len(step) != 1 && step["call"] == nil && step["await"] == nil {
		return runnerErr("a step has exactly one verb: %v", step)
	}
	switch {
	case step["call"] != nil:
		return sc.startCall(step)
	case step["await"] != nil:
		name, _ := step["await"].(string)
		ch, known := sc.calls[name]
		if !known {
			return runnerErr("await of an unknown call %q", name)
		}
		var got any
		select {
		case got = <-ch:
		case <-time.After(3 * time.Second):
			return fmt.Errorf("%s did not finish", name)
		}
		if err := sc.act.Idle(); err != nil {
			return err
		}
		return compareJSON(step["result"], got, name+" result")
	case step["expectSent"] != nil:
		if err := sc.act.Idle(); err != nil {
			return err
		}
		body, _ := step["expectSent"].(map[string]any)
		s, okSent := sc.transport.TakeSent()
		if !okSent {
			return errors.New("nothing was sent")
		}
		plain, okPlain := sc.cipher.Decrypt(s.Data)
		frame := string(plain)
		if !okPlain || !strings.HasPrefix(frame, "req::") {
			return errors.New("the sent datagram is not a request frame")
		}
		msgID, payload, found := strings.Cut(strings.TrimPrefix(frame, "req::"), ":")
		if !found {
			return fmt.Errorf("sent frame %s has no msg_id", frame)
		}
		if err := sc.bind(body["socket"], s.Socket); err != nil {
			return err
		}
		if err := sc.match(body["msgId"], msgID); err != nil {
			return err
		}
		if want, _ := body["payload"].(string); payload != want {
			return fmt.Errorf("sent payload %s, expected %s", payload, want)
		}
		return nil
	case step["expectQuiet"] != nil:
		if err := sc.act.Idle(); err != nil {
			return err
		}
		if s, extra := sc.transport.TakeSent(); extra {
			plain, _ := sc.cipher.Decrypt(s.Data)
			return fmt.Errorf("expected nothing sent, but the session sent %s", plain)
		}
		return nil
	case step["respond"] != nil:
		body, _ := step["respond"].(map[string]any)
		s, err := sc.labeled(body["socket"])
		if err != nil {
			return err
		}
		msgID, err := sc.resolve(body["msgId"])
		if err != nil {
			return err
		}
		if body["upper"] == true {
			msgID = strings.ToUpper(msgID)
		}
		payload, _ := body["payload"].(string)
		data, _ := sc.cipher.Encrypt([]byte("resp:" + sc.cluIP + ":" + msgID + ":" + payload))
		s.Deliver(data)
		return sc.act.Idle()
	case step["deliver"] != nil:
		body, _ := step["deliver"].(map[string]any)
		s, err := sc.labeled(body["socket"])
		if err != nil {
			return err
		}
		frame, _ := body["frame"].(string)
		encrypted, isBool := body["encrypted"].(bool)
		if !isBool {
			return runnerErr("deliver needs encrypted")
		}
		data := []byte(frame)
		if encrypted {
			data, _ = sc.cipher.Encrypt(data)
		}
		s.Deliver(data)
		return sc.act.Idle()
	case step["advance"] != nil:
		ms, isNum := step["advance"].(json.Number)
		n, convErr := ms.Int64()
		if !isNum || convErr != nil || n <= 0 {
			return runnerErr("advance needs a positive number of ms")
		}
		if err := sc.act.Idle(); err != nil {
			return err
		}
		sc.clock.Advance(time.Duration(n) * time.Millisecond)
		return sc.act.Idle()
	case step["expectState"] != nil:
		if err := sc.act.Idle(); err != nil {
			return err
		}
		if got, want := sc.session.State().String(), step["expectState"]; got != want {
			return fmt.Errorf("state is %s, expected %v", got, want)
		}
		return nil
	case step["expectReport"] != nil:
		if err := sc.act.Idle(); err != nil {
			return err
		}
		select {
		case r := <-sc.reports:
			return compareJSON(step["expectReport"], reportJSON(r), "report")
		default:
			return errors.New("no report arrived")
		}
	case step["expectNoReport"] != nil:
		if err := sc.act.Idle(); err != nil {
			return err
		}
		select {
		case r := <-sc.reports:
			return fmt.Errorf("unexpected report for %d key(s)", len(r.Keys))
		default:
			return nil
		}
	case step["expectClosed"] != nil:
		if err := sc.act.Idle(); err != nil {
			return err
		}
		s, err := sc.labeled(step["expectClosed"])
		if err != nil {
			return err
		}
		if !s.IsClosed() {
			return fmt.Errorf("socket %v is still open", step["expectClosed"])
		}
		return nil
	case step["fail"] != nil:
		s, err := sc.labeled(step["fail"])
		if err != nil {
			return err
		}
		s.R.Closed(errors.New("the transport failed"))
		return sc.act.Idle()
	}
	return runnerErr("unknown step %v", step)
}

func compareJSON(want, got any, what string) error {
	w, err := normalizeJSON(dropDecimals(want))
	if err != nil {
		return runnerErr("%s: %v", what, err)
	}
	g, err := normalizeJSON(got)
	if err != nil {
		return runnerErr("%s: %v", what, err)
	}
	if d := firstDifference(w, g, "$"); d != "" {
		return fmt.Errorf("%s %s differs from %s at %s", what, compact(g), compact(w), d)
	}
	return nil
}

func (sc *scenario) startCall(step map[string]any) error {
	name, _ := step["as"].(string)
	if name == "" || sc.calls[name] != nil {
		return runnerErr("a call needs a new name")
	}
	var run func() any
	switch step["call"] {
	case "start":
		run = func() any { sc.session.Start(); return nil }
	case "stop":
		run = func() any { sc.session.Stop(); return nil }
	case "ping":
		run = func() any { return map[string]any{"ok": sc.session.Ping()} }
	case "register", "fetch":
		refs, err := keyRefs(step)
		if err != nil {
			return err
		}
		if step["call"] == "register" {
			run = func() any { return map[string]any{"slots": slotsJSON(sc.session.Register(refs))} }
		} else {
			run = func() any { return map[string]any{"slots": slotsJSON(sc.session.Fetch(refs))} }
		}
	case "execute":
		raw, _ := json.Marshal(step["action"])
		action, err := model.ParseActionRef(raw)
		if err != nil {
			return runnerErr("execute: %v", err)
		}
		run = func() any {
			if err := sc.session.Execute(action); err != nil {
				return map[string]any{"error": sessionErrorCode(err)}
			}
			return map[string]any{"ok": true}
		}
	default:
		return runnerErr("unknown call %v", step["call"])
	}
	ch := make(chan any, 1)
	sc.calls[name] = ch
	sc.act.Add(1)
	go func() {
		defer sc.act.Add(-1)
		ch <- run()
	}()
	return sc.act.Idle()
}

func sessionErrorCode(err error) string {
	switch {
	case errors.Is(err, clu.ErrTimeout):
		return "session.timeout"
	case errors.Is(err, clu.ErrNotConnected):
		return "session.notConnected"
	case errors.Is(err, clu.ErrConnectionClosed):
		return "session.connectionClosed"
	case errors.Is(err, clu.ErrUnresolvedPlaceholder):
		return "payload.unresolvedPlaceholder"
	}
	return "unmapped: " + err.Error()
}

func slotsJSON(slots []clu.Slot) []any {
	out := make([]any, len(slots))
	for i, s := range slots {
		if s.Failed {
			out[i] = map[string]any{"failed": true}
		} else {
			out[i] = tagged(s.Value)
		}
	}
	return out
}

func reportJSON(r clu.Report) map[string]any {
	keys := make([]any, len(r.Keys))
	for i, k := range r.Keys {
		if k.CallType == model.ValueVariable {
			keys[i] = map[string]any{"type": "variable", "name": k.Index}
		} else {
			keys[i] = map[string]any{"type": "attribute", "objectName": k.ObjectName, "index": k.Index}
		}
	}
	return map[string]any{"keys": keys, "values": slotsJSON(r.Slots)}
}

// Labels and captures (conformance.md §4b).

func (sc *scenario) bind(label any, s *clutest.Socket) error {
	name, _ := label.(string)
	if bound, has := sc.labels[name]; has {
		if bound != s {
			return fmt.Errorf("%s is socket #%d, but this came from #%d", name, bound.Serial, s.Serial)
		}
		return nil
	}
	for other, bound := range sc.labels {
		if bound == s {
			return fmt.Errorf("%s must be a new socket, but this is %s", name, other)
		}
	}
	sc.labels[name] = s
	return nil
}

func (sc *scenario) labeled(label any) (*clutest.Socket, error) {
	name, _ := label.(string)
	s, has := sc.labels[name]
	if !has {
		return nil, runnerErr("socket %s is not bound", name)
	}
	return s, nil
}

func (sc *scenario) match(want any, got string) error {
	w, _ := want.(string)
	if !strings.HasPrefix(w, "$") {
		if w != got {
			return fmt.Errorf("msg_id %s, expected %s", got, w)
		}
		return nil
	}
	if bound, has := sc.captures[w]; has {
		if bound != got {
			return fmt.Errorf("%s was %s, now %s", w, bound, got)
		}
		return nil
	}
	sc.captures[w] = got
	return nil
}

func (sc *scenario) resolve(id any) (string, error) {
	s, _ := id.(string)
	if !strings.HasPrefix(s, "$") {
		return s, nil
	}
	v, has := sc.captures[s]
	if !has {
		return "", runnerErr("%s is not captured", s)
	}
	return v, nil
}
