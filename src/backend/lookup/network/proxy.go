package network

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
	"typonamer/config"
	"typonamer/lookup/lookuperror"
)

const proxyTestTargetHost = "rdap.verisign.com"
const proxyTestTargetPort = "443"

// ProxyTestTarget returns the fixed public TLS endpoint used by the admin proxy connectivity check.
func ProxyTestTarget() string {
	return net.JoinHostPort(proxyTestTargetHost, proxyTestTargetPort)
}

type socks5Dialer struct {
	inner    proxy.ContextDialer
	password string
}

func (d *socks5Dialer) DialContext(ctx context.Context, networkName, address string) (net.Conn, error) {
	conn, err := d.inner.DialContext(ctx, networkName, address)
	if err != nil {
		return nil, classifyProxyError(err, d.password)
	}
	return conn, nil
}

func (d *socks5Dialer) Dial(networkName, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), networkName, address)
}

func sanitizeProxyMessage(msg, secret string) string {
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	return msg
}

func classifyProxyError(err error, password string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return fmt.Errorf("%w（连接或握手超时）: %w", lookuperror.ErrorConnectToProxy, context.DeadlineExceeded)
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "unexpected protocol version 72"):
		return fmt.Errorf("%w（目标端口返回的不是 SOCKS5 协议，疑似填写了 HTTP 代理端口，请确认使用 SOCKS5 端口如 22228）", lookuperror.ErrorConnectToProxy)
	case strings.Contains(lower, "unexpected protocol version"):
		return fmt.Errorf("%w（代理服务器未返回有效的 SOCKS5 协议响应，请核对代理地址和端口）", lookuperror.ErrorConnectToProxy)
	case strings.Contains(lower, "authentication failed") || strings.Contains(lower, "no acceptable authentication methods"):
		return fmt.Errorf("%w（SOCKS5 代理认证失败，请检查用户名和密码）", lookuperror.ErrorConnectToProxy)
	case strings.Contains(lower, "connection not allowed by ruleset"):
		return fmt.Errorf("%w（代理拒绝连接：请检查代理凭据或目标端口/区域规则限制）", lookuperror.ErrorConnectToProxy)
	case strings.Contains(lower, "connection refused"):
		return fmt.Errorf("%w（代理服务器拒绝连接，请检查主机和端口）", lookuperror.ErrorConnectToProxy)
	case strings.Contains(lower, "no such host"):
		return fmt.Errorf("%w（无法解析代理主机域名）", lookuperror.ErrorConnectToProxy)
	default:
		return fmt.Errorf("%w: %s", lookuperror.ErrorConnectToProxy, sanitizeProxyMessage(err.Error(), password))
	}
}

// SOCKS5Dialer constructs a SOCKS5 dialer bound to the caller's request context.
func SOCKS5Dialer(c config.Config) (proxy.ContextDialer, error) {
	host := config.NormalizeProxyHost(c.SocketProxyHost)
	if host == "" || c.SocketProxyPort < 1 || c.SocketProxyPort > 65535 {
		return nil, lookuperror.ErrorProxyNotConfigured
	}
	if err := config.ValidateProxy(c); err != nil {
		return nil, fmt.Errorf("%w: %s", lookuperror.ErrorProxyNotConfigured, err.Error())
	}
	var auth *proxy.Auth
	if c.SocketProxyAuth {
		auth = &proxy.Auth{User: strings.TrimSpace(c.SocketProxyUser), Password: c.SocketProxyPassword}
	}
	forward := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	d, err := proxy.SOCKS5("tcp", net.JoinHostPort(host, strconv.Itoa(c.SocketProxyPort)), auth, forward)
	if err != nil {
		return nil, classifyProxyError(err, c.SocketProxyPassword)
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		return nil, lookuperror.ErrorConnectToProxy
	}
	return &socks5Dialer{inner: cd, password: c.SocketProxyPassword}, nil
}

// TestSOCKS5Proxy verifies SOCKS5 authentication, remote hostname resolution, and TLS reachability
// against a fixed public RDAP endpoint.
func TestSOCKS5Proxy(ctx context.Context, c config.Config) (time.Duration, error) {
	dialer, err := SOCKS5Dialer(c)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", ProxyTestTarget())
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	tlsConn := tls.Client(conn, &tls.Config{ServerName: proxyTestTargetHost, MinVersion: tls.VersionTLS12})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return 0, classifyProxyError(err, c.SocketProxyPassword)
	}
	_ = tlsConn.Close()
	return time.Since(start), nil
}
