// Package httpclient provides a shared HTTP client with sane timeouts.
//
// Unlike http.DefaultClient (which has no timeouts at all), this client
// sets dial, TLS handshake, and response header timeouts. These catch
// hung servers without imposing an overall request timeout that would
// break large file downloads.
package httpclient

import (
	"net"
	"net/http"
	"time"
)

// Client is a shared HTTP client configured with connection-level timeouts.
var Client = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}
