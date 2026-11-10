package conformance_test

// provisioning.fetch (conformance.md §4d): the production client against a
// real HTTP/1.1 server on 127.0.0.1 that answers every request with the
// case's response. The runner counts the requests and searches every text
// of an error for the PIN.

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/osieckiAdam/grenton-core-go/provisioning"
)

func init() {
	registry["provisioning.fetch"] = provisioningFetch
}

type loopbackServer struct {
	ln      net.Listener
	mu      sync.Mutex
	targets []string
	wg      sync.WaitGroup
}

func startServer(response []byte) (*loopbackServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &loopbackServer{ln: ln}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			reader := bufio.NewReader(conn)
			line, _ := reader.ReadString('\n')
			// Read the rest of the head before answering, so that closing
			// does not reset a connection with unread data.
			for {
				h, err := reader.ReadString('\n')
				if err != nil || h == "\r\n" {
					break
				}
			}
			if parts := strings.Fields(line); len(parts) >= 2 {
				s.mu.Lock()
				s.targets = append(s.targets, parts[1])
				s.mu.Unlock()
			}
			conn.Write(response)
			conn.Close()
		}
	}()
	return s, nil
}

func (s *loopbackServer) port() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *loopbackServer) stop() []string {
	s.ln.Close()
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.targets
}

// closedPort binds a TCP port and closes it: nothing listens there.
func closedPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	return port, ln.Close()
}

func provisioningFetch(s *suite, input map[string]any) (any, error) {
	pin, err := str(input, "pin")
	if err != nil {
		return nil, err
	}
	var server *loopbackServer
	var port int
	if input["unreachable"] == true {
		if port, err = closedPort(); err != nil {
			return nil, runnerErr("closed port: %v", err)
		}
	} else {
		resp, err := object(input, "response")
		if err != nil {
			return nil, err
		}
		raw, err := responseBytes(s, resp)
		if err != nil {
			return nil, err
		}
		if server, err = startServer(raw); err != nil {
			return nil, runnerErr("server: %v", err)
		}
		port = server.port()
	}
	client := &provisioning.Client{Host: "127.0.0.1", Port: port}
	in, fetchErr := client.FetchInterface(context.Background(), pin)
	var targets []string
	if server != nil {
		targets = server.stop()
	}
	// §4.2: one request per download; more means a redirect was followed.
	if len(targets) > 1 {
		return map[string]any{"outcome": "repeated", "requests": len(targets)}, nil
	}
	if fetchErr != nil {
		// §4.2: no text of the error holds the PIN in any form.
		texts := []string{fetchErr.Error(), fmt.Sprintf("%v", fetchErr), fmt.Sprintf("%+v", fetchErr), fmt.Sprintf("%#v", fetchErr)}
		for _, form := range pinForms(pin) {
			for _, text := range texts {
				if strings.Contains(text, form) {
					return map[string]any{"outcome": "leak", "text": text}, nil
				}
			}
		}
		return nil, fetchErr
	}
	if len(targets) == 0 {
		return nil, runnerErr("the server saw no request")
	}
	return ok(map[string]any{"requestTarget": targets[0], "interface": interfaceJSON(in), "mapping": mappingJSON(in)}), nil
}

func responseBytes(s *suite, resp map[string]any) ([]byte, error) {
	status, err := integer(resp, "status")
	if err != nil {
		return nil, err
	}
	var body []byte
	if b, has := resp["body"].(map[string]any); has {
		switch {
		case b["fixture"] != nil:
			rel, _ := b["fixture"].(string)
			if body, err = os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(rel))); err != nil {
				return nil, runnerErr("fixture: %v", err)
			}
		case b["zeros"] != nil:
			n, err := integer(b, "zeros")
			if err != nil {
				return nil, err
			}
			body = make([]byte, n)
		}
	}
	head := fmt.Sprintf("HTTP/1.1 %d Status\r\nContent-Length: %d\r\nConnection: close\r\n", status, len(body))
	if headers, has := resp["headers"].(map[string]any); has {
		for k, v := range headers {
			head += fmt.Sprintf("%s: %v\r\n", k, v)
		}
	}
	return append([]byte(head+"\r\n"), body...), nil
}

// pinForms: as given, without White_Space, and that form percent-encoded
// as one path segment (§4.1). The runner computes them itself.
func pinForms(pin string) []string {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, pin)
	encoded := strings.ReplaceAll(url.PathEscape(compact), "+", "%2B")
	var forms []string
	for _, f := range []string{pin, compact, encoded} {
		if f != "" {
			forms = append(forms, f)
		}
	}
	return forms
}
