package provisioning

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/osieckiAdam/grenton-core-go/model"
)

// Client downloads the interface from Object Manager (spec §4.1–§4.3).
// Object Manager serves it only while its "send to mobile device" dialog is
// open; the PIN is the token that dialog shows.
type Client struct {
	// Host is the computer running Object Manager: a name, an IPv4 address,
	// or an IPv6 address with or without brackets.
	Host string
	// Port defaults to DefaultPort.
	Port int
	// HTTP is the client to use; default one with a 30 s timeout. Redirects
	// are never followed, whatever its CheckRedirect says.
	HTTP *http.Client
	// Logger receives debug logs, which never contain the PIN.
	Logger *slog.Logger
}

// FetchInterface downloads and parses the interface. Its errors are
// *Error: AuthenticationFailed for a wrong PIN (401), Connection for a
// network failure or any other status (3xx included: one request, no
// redirect), InvalidData for a body that is not a valid archive. No error
// text contains the PIN in any form.
func (c *Client) FetchInterface(ctx context.Context, pin string) (*model.Interface, error) {
	port := c.Port
	if port == 0 {
		port = DefaultPort
	}
	endpoint, err := EndpointURL(c.Host, port, pin)
	if err != nil {
		return nil, err
	}
	log := c.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		copied := *c.HTTP
		client = &copied
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &Error{Kind: Connection, Detail: scrub(err.Error(), pin)}
	}
	log.Debug("downloading the interface", "host", c.Host, "port", port)
	resp, err := client.Do(req)
	if err != nil {
		// *url.Error's text holds the URL, and the URL holds the PIN:
		// report the cause only, scrubbed as well.
		cause := err
		var ue *url.Error
		if errors.As(err, &ue) {
			cause = ue.Err
		}
		return nil, &Error{Kind: Connection, Detail: scrub(cause.Error(), pin)}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, &Error{Kind: AuthenticationFailed, Status: resp.StatusCode, Detail: "wrong PIN"}
	case resp.StatusCode != http.StatusOK:
		return nil, &Error{Kind: Connection, Status: resp.StatusCode, Detail: "HTTP " + resp.Status}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, ArchiveLimit+1))
	if err != nil {
		return nil, invalidData("reading the response: %s", scrub(err.Error(), pin))
	}
	if len(body) > ArchiveLimit {
		return nil, invalidData("the response is larger than %d bytes", ArchiveLimit)
	}
	log.Debug("downloaded the interface archive", "bytes", len(body))
	return ParseArchive(body)
}
