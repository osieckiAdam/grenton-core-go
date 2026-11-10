// Command fetch-interface downloads the mobile interface from Object
// Manager and saves it.
//
//	go run ./examples/fetch-interface <om-host>[:port] <output.zip> <pin>
//	go run ./examples/fetch-interface 192.168.1.50 dist/interface.zip 149 853
//
// The PIN is the token shown in Object Manager's "Send to mobile devices"
// dialog, which must stay open while this runs. It goes last, so it can be
// typed exactly as the dialog shows it: `149 853` arrives as two arguments
// and is joined back (the library ignores whitespace in a PIN anyway). The
// port defaults to 9998; the dialog shows it, e.g. 192.168.1.50:9999.
//
// The saved ZIP lets later runs start without a PIN. It contains the CLU
// encryption key, so it is written readable by the owner only.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/osieckiAdam/grenton-core-go/provisioning"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: fetch-interface <om-host>[:port] <output.zip> <pin>")
		os.Exit(2)
	}
	outputPath := os.Args[2]
	pin := strings.Join(os.Args[3:], " ")

	// "host:port" for a hostname or IPv4 address; an IPv6 literal has
	// several colons and keeps the default port.
	host, port := os.Args[1], provisioning.DefaultPort
	if strings.Count(host, ":") == 1 {
		h, p, _ := strings.Cut(host, ":")
		parsed, err := strconv.Atoi(p)
		if err != nil {
			fmt.Println("bad port in", host)
			os.Exit(2)
		}
		host, port = h, parsed
	}

	// 1. One HTTP request to Object Manager returns the interface as a ZIP.
	// FetchInterface checks and parses it; the recorder keeps a copy of the
	// bytes it read, so that they can be saved.
	recorder := &bodyRecorder{}
	client := &provisioning.Client{
		Host: host,
		Port: port,
		HTTP: &http.Client{Timeout: 30 * time.Second, Transport: recorder},
	}
	in, err := client.FetchInterface(context.Background(), pin)
	switch {
	case errors.Is(err, provisioning.ErrAuthenticationFailed):
		fmt.Println("Object Manager rejected the PIN — check the token and keep its dialog open")
		os.Exit(1)
	case errors.Is(err, provisioning.ErrConnection):
		fmt.Printf("cannot reach Object Manager at %s:%d — check the IP and port in its "+
			"\"Send to mobile devices\" dialog, and keep that dialog open (%v)\n", host, port, err)
		os.Exit(1)
	case err != nil:
		fmt.Println("the download is not a valid interface:", err)
		os.Exit(1)
	}

	// 2. It parsed, so it is a valid interface.
	fmt.Printf("interface %q (version %d)\n", in.Name, in.Version)
	for _, c := range in.Clus {
		fmt.Printf("  CLU %s at %s:%d\n", c.Name, c.IP, c.Port)
	}

	// 3. Save it, owner-readable only (mode 600).
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputPath, recorder.body.Bytes(), 0o600); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	_ = os.Chmod(outputPath, 0o600) // WriteFile keeps the mode of an existing file
	fmt.Println("saved to", outputPath)
}

// bodyRecorder is an http.RoundTripper that copies the response body as
// it is read. The library offers no call that returns the archive bytes;
// this keeps the library's own checks (one request, no redirect, the size
// limit, no PIN in errors) and still sees the bytes.
type bodyRecorder struct{ body bytes.Buffer }

func (r *bodyRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.TeeReader(resp.Body, &r.body), resp.Body}
	return resp, nil
}
