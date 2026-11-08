// Package opencode holds the gateway-specific transport shared by the agent
// runtime and the settings connection test.
package opencode

import (
	"net/http"
	"net/url"
	"strings"
)

// IsGateway reports whether baseURL points at the OpenCode gateway, which
// requires a session header on every request.
func IsGateway(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "opencode.ai" || strings.HasSuffix(host, ".opencode.ai")
}

// SessionTransport wraps base to add the OpenCode session header.
func SessionTransport(base http.RoundTripper, sessionID string) http.RoundTripper {
	return sessionTransport{base: base, sessionID: sessionID}
}

type sessionTransport struct {
	base      http.RoundTripper
	sessionID string
}

func (t sessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if clone.Header.Get("x-opencode-session") == "" {
		clone.Header.Set("x-opencode-session", t.sessionID)
	}
	return t.base.RoundTrip(clone)
}
