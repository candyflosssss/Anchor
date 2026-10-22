package core

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"syscall"
)

func transportTimedOut(err error) bool {
	var network net.Error
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout())
}

// Preserve the underlying cause as a closed category. err.Error() includes the
// URL and can contain credentials, proxy addresses and remote-controlled text.
func transportCause(err error) string {
	var dns *net.DNSError
	var certificate *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	switch {
	case transportTimedOut(err):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.As(err, &dns):
		return "dns"
	case errors.As(err, &certificate), errors.As(err, &authority), errors.As(err, &hostname):
		return "tls_certificate"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection_reset"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	case errors.Is(err, syscall.EPIPE):
		return "broken_pipe"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected_eof"
	default:
		return "transport"
	}
}

func recordTransportFailure(r *http.Request, err error, connected bool, timeout int) {
	if t := trace(r); t != nil {
		t.row.Evidence["transport_cause"] = transportCause(err)
		phase := "connect"
		if connected {
			phase = "response_headers"
		}
		t.row.Evidence["transport_phase"] = phase
		t.row.Evidence["provider_timeout_seconds"] = strconv.Itoa(timeout)
	}
}
