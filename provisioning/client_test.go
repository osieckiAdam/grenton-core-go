package provisioning

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
)

// No log line holds the PIN (spec §4.2); the suite checks only error texts.
func TestLogsNeverHoldThePIN(t *testing.T) {
	const pin = "149 853"
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not a zip"))
	})}
	go srv.Serve(ln)
	defer srv.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	for _, c := range []*Client{
		{Host: "127.0.0.1", Port: port, Logger: log}, // an invalid body
		{Host: "127.0.0.1", Port: 1, Logger: log},    // refused
		{Host: "no-such-host.invalid", Logger: log},  // DNS
	} {
		_, err := c.FetchInterface(context.Background(), pin)
		if err == nil {
			t.Fatal("want an error")
		}
		for _, form := range pinForms(pin) {
			if strings.Contains(err.Error(), form) {
				t.Errorf("error %q holds the PIN", err)
			}
		}
	}
	if logs.Len() == 0 {
		t.Fatal("no log line was written")
	}
	for _, form := range pinForms(pin) {
		if strings.Contains(logs.String(), form) {
			t.Fatalf("a log line holds the PIN:\n%s", logs.String())
		}
	}
}
