package whoislib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"typonamer/lookup/lookuperror"
)

func TestWhoisTransportFailuresAreNotAllTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cause   error
		timeout bool
	}{
		{"missing WHOIS server hostname", &net.DNSError{Err: "no such host", Name: "whois.registry.in", IsNotFound: true}, false},
		{"connection failure", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, false},
		{"interrupted response", io.ErrUnexpectedEOF, false},
		{"canceled request", context.Canceled, false},
		{"request deadline", context.DeadlineExceeded, true},
		{"wrapped DNS timeout", fmt.Errorf("dial: %w", &net.DNSError{Err: "i/o timeout", IsTimeout: true}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := whoisTransportError(tc.cause)
			if errors.Is(got, lookuperror.ErrorWhoisTimeout) != tc.timeout {
				t.Fatalf("wrong timeout classification: %v", got)
			}
			if errors.Is(got, lookuperror.ErrorWhoisServerFailed) == tc.timeout || !errors.Is(got, tc.cause) {
				t.Fatalf("server failure or original cause lost: %v", got)
			}
		})
	}
}
