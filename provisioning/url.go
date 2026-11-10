package provisioning

import (
	"strconv"
	"strings"
	"unicode"
)

// DefaultPort is Object Manager's interface port.
const DefaultPort = 9998

// EndpointURL builds the download URL of spec §4.1:
// http://{host}:{port}/api/v1/interface/hash/{pin}. Every Unicode
// White_Space character is removed from the PIN, which is then one
// percent-encoded path segment. An IPv6 host is bracketed. An empty PIN or
// a port outside 1–65535 is a Connection error.
func EndpointURL(host string, port int, pin string) (string, error) {
	segment := pinSegment(pin)
	if segment == "" {
		return "", &Error{Kind: Connection, Detail: "the PIN is empty"}
	}
	if port < 1 || port > 65535 {
		return "", &Error{Kind: Connection, Detail: "port " + strconv.Itoa(port) + " is outside 1–65535"}
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + strconv.Itoa(port) + "/api/v1/interface/hash/" + segment, nil
}

// compactPIN removes every White_Space character (unicode.IsSpace is that
// property).
func compactPIN(pin string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, pin)
}

// pinSegment is the compact PIN percent-encoded as one path segment: every
// byte outside unreserved, sub-delims, ':' and '@' (RFC 3986) as %XX.
func pinSegment(pin string) string {
	compact := compactPIN(pin)
	var b strings.Builder
	for i := 0; i < len(compact); i++ {
		c := compact[i]
		if isPathChar(c) {
			b.WriteByte(c)
		} else {
			b.WriteString("%" + strings.ToUpper(strconv.FormatUint(uint64(c)>>4, 16)) +
				strings.ToUpper(strconv.FormatUint(uint64(c)&15, 16)))
		}
	}
	return b.String()
}

func isPathChar(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("-._~!$&'()*+,;=:@", c) >= 0
}

// pinForms are the forms of a PIN that must never appear in an error or a
// log (spec §4.2): as given, without whitespace, and percent-encoded.
func pinForms(pin string) []string {
	var forms []string
	for _, f := range []string{pin, compactPIN(pin), pinSegment(pin)} {
		if f != "" {
			forms = append(forms, f)
		}
	}
	return forms
}

// scrub removes every form of the PIN from a text.
func scrub(text, pin string) string {
	for _, f := range pinForms(pin) {
		text = strings.ReplaceAll(text, f, "<pin>")
	}
	return text
}
