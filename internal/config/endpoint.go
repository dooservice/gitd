package config

import (
	"fmt"
	"net"
	"path/filepath"
)

// Endpoint is gitd's network address, off unless Listen is set: sandboxes that cannot mount
// its directories clone and push there with tokens of gitd's own. URL is the address they
// are given, e.g. `https://gitd.internal:9420`, by default Listen's; TLSCertFile and
// TLSKeyFile serve it over TLS, without which it should be on a private network only; and
// MaxPushBytes bounds one push.
type Endpoint struct {
	Listen       string `toml:"listen"`
	URL          string `toml:"url"`
	TLSCertFile  string `toml:"tls_cert_file"`
	TLSKeyFile   string `toml:"tls_key_file"`
	MaxPushBytes int64  `toml:"max_push_bytes"`
}

// TLS says whether the endpoint is served over TLS.
func (endpoint Endpoint) TLS() bool {
	return endpoint.TLSCertFile != ""
}

// normalize checks the endpoint and fills in its URL.
func (endpoint *Endpoint) normalize() error {
	if endpoint.Listen == "" {
		return nil
	}
	if _, _, err := net.SplitHostPort(endpoint.Listen); err != nil {
		return fmt.Errorf("endpoint: listen %q is not host:port", endpoint.Listen)
	}
	if (endpoint.TLSCertFile == "") != (endpoint.TLSKeyFile == "") {
		return fmt.Errorf("endpoint: tls_cert_file and tls_key_file go together")
	}
	if endpoint.MaxPushBytes < 0 {
		return fmt.Errorf("endpoint: max_push_bytes cannot be negative")
	}
	if endpoint.URL == "" {
		scheme := "http"
		if endpoint.TLS() {
			scheme = "https"
		}
		endpoint.URL = scheme + "://" + endpoint.Listen
	}
	return nil
}

// Sandboxes is where the sandbox repositories of the endpoint live.
func (settings Config) Sandboxes() string {
	return filepath.Join(settings.State, "sandboxes")
}
