package httpxgo

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	bufferSize            = 32 * 1024
	maxIdleConnsPerHost   = 2
	idleConnTimeout       = 2 * time.Minute
	expectContinueTimeout = 1 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	maxIdleConns          = 512
)

var defaultTransport = &http.Transport{
	DialContext: transportDailContext(),
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true,
	},
	MaxIdleConns:          maxIdleConns,
	MaxIdleConnsPerHost:   maxIdleConnsPerHost,
	IdleConnTimeout:       idleConnTimeout,
	ExpectContinueTimeout: expectContinueTimeout,
	TLSHandshakeTimeout:   tlsHandshakeTimeout,
	ForceAttemptHTTP2:     true,
	WriteBufferSize:       bufferSize,
	ReadBufferSize:        bufferSize,
}

// SetProxy set proxy to defaultTransport.
// if you're using custom transport it is assumed that you have provide proxy with it.
func SetProxy(proxy func(r *http.Request) (*url.URL, error)) {
	defaultTransport.Proxy = proxy
}

// SetSocket function used for connecting to various different socket such as unix, ip. tcp, ipv4,
// ipv6
func SetSocket(f func(ctx context.Context, network, addr string) (net.Conn, error)) {
	defaultTransport.DialContext = f
}

// GetDefaultTransport returns Cloned pointer to [net/http.Transport],
// which you can configure to your liking other than defaults.
func GetDefaultTransport() *http.Transport {
	return defaultTransport.Clone()
}

// transportDailContext return DailContext Func for setting it in transport.
// usable for field such as DialContext and DialTLSContext.
func transportDailContext() func(context.Context, string, string) (net.Conn, error) {
	return (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
}
