package devices_test

// The System over two scripted CLUs: what the suite does not cover (spec
// §9.5 item 8): the update signals of §6.5 through the whole path, the
// routing of commands by CLU, and resource release after Stop.

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/devices"
	"github.com/osieckiAdam/grenton-core-go/internal/clutest"
	"github.com/osieckiAdam/grenton-core-go/model"
)

const twoCLUs = `{"id": "i", "version": 1, "name": "Test", "icon": "home", "theme": "GRENTON",
 "encryption": {"key": "AAECAwQFBgcICQoLDA0ODw==", "iv": "EBESExQVFhcYGRobHB0eHw=="},
 "clus": [
  {"id": "CLU_A", "serialNumber": "1", "name": "A", "ip": "10.0.0.1", "port": 1234, "connectionType": "LOCAL_ONLY"},
  {"id": "CLU_B", "serialNumber": "2", "name": "B", "ip": "10.0.0.2", "port": 1234, "connectionType": "LOCAL_ONLY"}],
 "pushNotifications": [],
 "pages": [{"name": "P", "icon": "home", "isFullscreenWidget": false, "widgets": [
  {"id": "lamp", "type": "ON_OFF", "components": [{"type": "BUTTON_BISTABLE", "label": "Lamp A", "rowId": 0,
    "unit": "UNKNOWN", "onIndication": "on", "offIndication": "off",
    "state": {"cluId": "CLU_A", "objectName": "DOU1001", "index": "0", "callType": "ATTRIBUTE"},
    "actions": [
     {"event": "ON", "cluId": "CLU_A", "objectName": "DOU1001", "index": "0", "value": "1", "callType": "ATTRIBUTE"},
     {"event": "OFF", "cluId": "CLU_A", "objectName": "DOU1001", "index": "0", "value": "0", "callType": "ATTRIBUTE"}]}]},
  {"id": "temp", "type": "VALUE_V2", "label": "Temp", "icon": "t", "unit": "DEGREE", "valueType": "FLOAT",
   "precision": 1, "object": {"value": {"cluId": "CLU_B", "objectName": "SEN1001", "index": "0", "callType": "ATTRIBUTE"}}},
  {"id": "fan", "type": "ON_OFF", "components": [{"type": "BUTTON_BISTABLE", "label": "Fan", "rowId": 0,
    "unit": "UNKNOWN", "onIndication": "on", "offIndication": "off",
    "state": {"cluId": "CLU_B", "objectName": "DOU2001", "index": "0", "callType": "ATTRIBUTE"},
    "actions": [
     {"event": "ON", "cluId": "CLU_B", "objectName": "DOU2001", "index": "0", "value": "1", "callType": "ATTRIBUTE"},
     {"event": "OFF", "cluId": "CLU_B", "objectName": "DOU2001", "index": "0", "value": "0", "callType": "ATTRIBUTE"}]}]}
 ]}]}`

type fixture struct {
	t         *testing.T
	act       *clutest.Activity
	transport *clutest.Transport
	cipher    *clu.AESCipher
	sys       *devices.System
}

func newFixture(t *testing.T) *fixture {
	in, err := model.DecodeInterface([]byte(twoCLUs))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, act: clutest.NewActivity(), transport: &clutest.Transport{}}
	f.cipher, _ = clu.NewCipher(in.Encryption.Key, in.Encryption.IV)
	f.sys, err = devices.NewSystem(in, devices.SystemConfig{Transport: f.transport, Clock: clutest.NewClock(f.act),
		Activity: f.act, PingInterval: time.Hour, RegisterInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) idle() {
	f.t.Helper()
	if err := f.act.Idle(); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) async(fn func()) <-chan struct{} {
	done := make(chan struct{})
	f.act.Add(1)
	go func() {
		defer f.act.Add(-1)
		fn()
		close(done)
	}()
	f.idle()
	return done
}

// answer replies to every sent request; payload maps a payload prefix to
// the reply payload ("" answers anything with "1").
func (f *fixture) answer(replies map[string]string) []string {
	var seen []string
	for {
		f.idle()
		s, ok := f.transport.TakeSent()
		if !ok {
			return seen
		}
		plain, _ := f.cipher.Decrypt(s.Data)
		frame := string(plain)
		id, payload, _ := strings.Cut(strings.TrimPrefix(frame, "req::"), ":")
		seen = append(seen, s.Socket.Host+" "+payload)
		reply := "1"
		for prefix, r := range replies {
			if strings.HasPrefix(payload, prefix) {
				reply = r
			}
		}
		data, _ := f.cipher.Encrypt([]byte("resp:" + s.Socket.Host + ":" + id + ":" + reply))
		s.Socket.Deliver(data)
	}
}

func (f *fixture) notify(host string, payload string) {
	for _, s := range f.transport.Sockets() {
		if s.Host == host && !s.IsClosed() {
			data, _ := f.cipher.Encrypt([]byte("resp:" + host + ":00000000:" + payload))
			s.Deliver(data)
		}
	}
	f.idle()
}

func wait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the call did not finish")
	}
}

func TestSystemOverTwoCLUs(t *testing.T) {
	before := runtime.NumGoroutine()
	f := newFixture(t)
	done := f.async(f.sys.Start)
	f.answer(map[string]string{
		"SYSTEM:clientRegister(0,0,1,{{DOU1001,0}})":             "clientReport:1:{0}",
		"SYSTEM:clientRegister(0,0,1,{{SEN1001,0},{DOU2001,0}})": "clientReport:1:{21.5,1}",
	})
	wait(t, done)

	lamp := f.sys.Entity("lamp_0").(*devices.Switch)
	temp := f.sys.Entity("temp_0").(*devices.Sensor)
	fan := f.sys.Entity("fan_0").(*devices.Switch)
	if on, ok := lamp.IsOn(); !ok || on {
		t.Fatalf("lamp %v %v, want off", on, ok)
	}
	if v, _ := temp.Value(); v != model.Double(21.5) {
		t.Fatalf("temp %v", v)
	}
	if on, _ := fan.IsOn(); !on {
		t.Fatal("fan off, want on")
	}

	// §6.5: a change of the fan's key signals the fan; a report that also
	// changes other keys of the chunk does not void it; the lamp, on
	// another CLU, gets no signal.
	fanUpdates, cancelFan := fan.Updates()
	defer cancelFan()
	lampUpdates, cancelLamp := lamp.Updates()
	defer cancelLamp()
	f.notify("10.0.0.2", "clientReport:1:{22.0,0}")
	select {
	case <-fanUpdates:
	default:
		t.Fatal("no update signal for the fan")
	}
	if on, _ := fan.IsOn(); on {
		t.Fatal("the store does not hold the change the signal announced")
	}
	select {
	case <-lampUpdates:
		t.Fatal("the lamp was signalled for another CLU's change")
	default:
	}

	// Commands go to the session of their CLU.
	done = f.async(func() {
		if err := lamp.TurnOn(); err != nil {
			t.Error(err)
		}
		if err := fan.TurnOn(); err != nil {
			t.Error(err)
		}
	})
	sent := f.answer(nil)
	wait(t, done)
	want := []string{`10.0.0.1 DOU1001:set(0,"1")`, `10.0.0.2 DOU2001:set(0,"1")`}
	if strings.Join(sent, "|") != strings.Join(want, "|") {
		t.Fatalf("sent %q, want %q", sent, want)
	}

	done = f.async(f.sys.Stop)
	wait(t, done)
	for _, s := range f.transport.Sockets() {
		if !s.IsClosed() {
			t.Fatalf("socket to %s open after Stop", s.Host)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines remain, want %d", runtime.NumGoroutine(), before)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
