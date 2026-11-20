package proxydial

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"typonamer/lookup/lookuperror"
)

// An in-memory connection verifies fragmented SOCKS responses without sockets.
type handshakeFixture struct {
	net.Conn
	input *bytes.Reader
}

func (c *handshakeFixture) Read(p []byte) (int, error)  { return c.input.Read(p) }
func (c *handshakeFixture) Write(p []byte) (int, error) { return len(p), nil }

func TestProxyHandshakeReplyClassification(t *testing.T) {
	cases := []struct {
		name  string
		reply []byte
		want  error
	}{
		{"success", []byte{5, 0, 0, 1}, nil},
		{"upstreamRefused", []byte{5, 5, 0, 1}, lookuperror.ErrorConnectToProxy},
		{"unknownReply", []byte{5, 9, 0, 1}, nil},
		{"invalidVersion", []byte{4, 5, 0, 1}, nil},
		{"invalidReserved", []byte{5, 5, 1, 1}, nil},
	}
	for rep := byte(1); rep <= 8; rep++ {
		cases = append(cases, struct {
			name  string
			reply []byte
			want  error
		}{fmt.Sprintf("REP%d", rep), []byte{5, rep, 0, 1}, lookuperror.ErrorConnectToProxy})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &proxyHandshakeConn{Conn: &handshakeFixture{input: bytes.NewReader(tc.reply)}}
			c.Write([]byte{5, 1, 0, 3, 1, 'a', 0, 43})
			var err error
			for i := 0; i < 4; i++ {
				_, err = c.Read(make([]byte, 1))
				if err != nil {
					break
				}
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v, want %v", err, tc.want)
			}
		})
	}
}

func TestProxyHandshakeAuthIsNotConnectReply(t *testing.T) {
	c := &proxyHandshakeConn{Conn: &handshakeFixture{input: bytes.NewReader([]byte{1, 1})}}
	c.Write([]byte{1, 3, 'a', 'b', 'c', 1, 'p'})
	if _, err := io.ReadFull(c, make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	if c.connectReply {
		t.Fatal("authentication mistaken for CONNECT")
	}
}
