package whoislib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"

	"golang.org/x/net/idna"
	"golang.org/x/net/proxy"
)

func WhoisQuery(ctx context.Context, domain, tld string, useProxy bool, cfg config.Config) (lookupinfo.DomainInfo, error) {
	info := lookupinfo.DomainInfo{LookupType: constant.LookupTypeWhois, DomainName: domain, ViaProxy: useProxy}
	if WhoisSupportedTlds[tld] == "" {
		tld, _ = idna.Lookup.ToUnicode(tld)
	}
	server := WhoisSupportedTlds[tld]
	if server == "" {
		return info, lookuperror.ErrorNotSupportedTld
	}
	matcher, ok := WhoisMatchers[tld]
	if !ok {
		return info, lookuperror.ErrorNoParseRuleForTld
	}

	release, err := network.Acquire(ctx, "whois:"+server, 3)
	if err != nil {
		return info, err
	}
	defer release()
	baseDialer := &net.Dialer{}
	dial := baseDialer.DialContext
	if useProxy {
		if cfg.SocketProxyHost == "" {
			return info, lookuperror.ErrorConnectToProxy
		}
		var auth *proxy.Auth
		if cfg.SocketProxyAuth {
			auth = &proxy.Auth{User: cfg.SocketProxyUser, Password: cfg.SocketProxyPassword}
		}
		dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort(cfg.SocketProxyHost, strconv.Itoa(cfg.SocketProxyPort)), auth, baseDialer)
		if err != nil {
			return info, lookuperror.ErrorConnectToProxy
		}
		dial = dialer.(proxy.ContextDialer).DialContext
	}

	conn, err := dial(ctx, "tcp", net.JoinHostPort(server, "43"))
	if err != nil {
		return info, transportError(ctx, err, useProxy)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return info, transportError(ctx, err, false)
		}
	}

	query := domain
	if options, ok := WhoisTldOptions[tld]; ok {
		query = options + " " + domain
	}
	if _, err := fmt.Fprint(conn, query+"\r\n"); err != nil {
		return info, transportError(ctx, err, false)
	}
	raw, err := io.ReadAll(io.LimitReader(conn, network.MaxResponseBytes+1))
	if err != nil {
		return info, transportError(ctx, err, false)
	}
	if len(raw) > network.MaxResponseBytes {
		return info, lookuperror.ErrorWhoisServerFailed
	}
	if strings.TrimSpace(string(raw)) == "" {
		return info, lookuperror.ErrorNoContentInWhoisResponse
	}
	info, err = ParseWhoisResponse(string(raw), domain, matcher)
	info.ViaProxy = useProxy
	return info, err
}

func transportError(ctx context.Context, err error, viaProxy bool) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return ctx.Err()
	}
	var netErr net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return lookuperror.ErrorWhoisTimeout
	}
	if viaProxy {
		return lookuperror.ErrorConnectToProxy
	}
	return lookuperror.ErrorWhoisServerFailed
}
