package network

import (
	"net"
	"strconv"
	"strings"

	"golang.org/x/net/proxy"
	"typonamer/config"
	"typonamer/lookup/lookuperror"
)

// SOCKS5Dialer only constructs a dialer. The caller supplies its request deadline.
func SOCKS5Dialer(c config.Config) (proxy.ContextDialer, error) {
	host := strings.TrimSpace(c.SocketProxyHost)
	if host == "" || c.SocketProxyPort < 1 || c.SocketProxyPort > 65535 {
		return nil, lookuperror.ErrorProxyNotConfigured
	}
	var auth *proxy.Auth
	if c.SocketProxyAuth {
		auth = &proxy.Auth{User: c.SocketProxyUser, Password: c.SocketProxyPassword}
	}
	d, err := proxy.SOCKS5("tcp", net.JoinHostPort(host, strconv.Itoa(c.SocketProxyPort)), auth, &net.Dialer{})
	if err != nil {
		return nil, lookuperror.ErrorConnectToProxy
	}
	return d.(proxy.ContextDialer), nil
}
