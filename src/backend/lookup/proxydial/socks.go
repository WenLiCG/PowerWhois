package proxydial

import (
	"context"
	"net"
	"typonamer/log"
	"typonamer/lookup/lookuperror"
)

// ForwardDialer bounds both TCP establishment and SOCKS negotiation by the
// caller's deadline. Each connection tracks its own CONNECT reply, including fragmented reads.
type ForwardDialer struct{}

func (d *ForwardDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d *ForwardDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	c, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return nil, lookuperror.ErrorConnectToProxy
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := c.SetDeadline(deadline); err != nil {
			c.Close()
			return nil, lookuperror.ErrorConnectToProxy
		}
	}
	return &proxyHandshakeConn{Conn: c}, nil
}

// x/net/proxy erases the SOCKS reply code in its error. Preserve that code's
// meaning at the wire boundary instead of inspecting error text or addresses.
type proxyHandshakeConn struct {
	net.Conn
	connectReply bool
	reply        []byte
}

func (c *proxyHandshakeConn) Write(p []byte) (int, error) {
	// Greeting and username/password auth use different packet headers.
	if len(p) >= 7 && p[0] == 5 && p[1] == 1 && p[2] == 0 {
		c.connectReply = true
	}
	return c.Conn.Write(p)
}

func (c *proxyHandshakeConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if c.connectReply {
		c.reply = append(c.reply, p[:n]...)
		if len(c.reply) >= 4 {
			c.connectReply = false
			if c.reply[0] == 5 && c.reply[2] == 0 && c.reply[1] >= 1 && c.reply[1] <= 8 {
				log.Warnf("代理 CONNECT 被拒绝 rep=%d", c.reply[1])
				return 0, lookuperror.ErrorConnectToProxy
			}
		}
	}
	return n, err
}
