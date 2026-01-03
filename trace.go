package httpxgo

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http/httptrace"
	"time"
)

type TraceInfo struct {
	// DNSLookup is the duration that transport took to perform
	// DNS lookup.
	DNSLookup time.Duration `json:"dns_lookup_time"`
	// ConnTime is the duration it took to obtain a successful connection.
	ConnTime time.Duration `json:"connection_time"`
	// TCPConnTime is the duration it took to obtain the TCP connection.
	TCPConnTime time.Duration `json:"tcp_connection_time"`
	// TLSHandshake is the duration of the TLS handshake.
	TLSHandshake time.Duration `json:"tls_handshake_time"`
	// ServerTime is the server's duration for responding to the first byte.
	ServerTime time.Duration `json:"server_time"`
	// ResponseTime is the duration since the first response byte from the server to
	// request completion.
	ResponseTime time.Duration `json:"response_time"`
	// TotalTime is the duration of the total time request taken end-to-end.
	TotalTime time.Duration `json:"total_time"`
	// IsConnReused is whether this connection has been previously
	// used for another HTTP request.
	IsConnReused bool `json:"is_connection_reused"`
	// IsConnWasIdle is whether this connection was obtained from an
	// idle pool.
	IsConnWasIdle bool `json:"is_connection_was_idle"`
	// ConnIdleTime is the duration how long the connection that was previously
	// idle, if IsConnWasIdle is true.
	ConnIdleTime time.Duration `json:"connection_idle_time"`
	// RemoteAddr returns the remote network address.
	RemoteAddr string `json:"remote_address"`
}

// String method returns string representation of request trace information.
func (ti *TraceInfo) String() string {
	return fmt.Sprintf(`TRACE INFO:
  DNSLookupTime : %v
  ConnTime      : %v
  TCPConnTime   : %v
  TLSHandshake  : %v
  ServerTime    : %v
  ResponseTime  : %v
  TotalTime     : %v
  IsConnReused  : %v
  IsConnWasIdle : %v
  ConnIdleTime  : %v
  RemoteAddr    : %v`, ti.DNSLookup, ti.ConnTime, ti.TCPConnTime,
		ti.TLSHandshake, ti.ServerTime, ti.ResponseTime, ti.TotalTime,
		ti.IsConnReused, ti.IsConnWasIdle, ti.ConnIdleTime, ti.RemoteAddr)
}

func (ti *TraceInfo) Tracer(ctx context.Context) context.Context {
	var dnsStart, connectSart, getConn, gotConn, tlsHandshakeStart time.Time
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			ti.DNSLookup = time.Since(dnsStart)
		},
		ConnectStart: func(_, _ string) {
			connectSart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			ti.TCPConnTime = time.Since(connectSart)
		},
		GetConn: func(_ string) {
			getConn = time.Now()
		},
		GotConn: func(gci httptrace.GotConnInfo) {
			gotConn := time.Now()
			ti.ConnTime = gotConn.Sub(getConn)
			ti.RemoteAddr = gci.Conn.RemoteAddr().String()
			ti.ConnIdleTime = gci.IdleTime
			ti.IsConnReused = gci.Reused
		},
		GotFirstResponseByte: func() {
			ti.ServerTime = time.Since(gotConn)
		},
		TLSHandshakeStart: func() {
			tlsHandshakeStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			ti.TLSHandshake = time.Since(tlsHandshakeStart)
		},
	})
}
