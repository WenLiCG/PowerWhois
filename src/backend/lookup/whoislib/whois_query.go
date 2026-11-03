package whoislib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"

	"github.com/duke-git/lancet/v2/maputil"
	"github.com/duke-git/lancet/v2/strutil"
	"golang.org/x/net/proxy"
)

// WhoisQuery function is used to query the WHOIS information for a given domain.
// If the useProxy parameter is set to true, it will use the proxy server to query the WHOIS information.
func WhoisQueryContext(ctx context.Context, domain string, tld string, useProxy bool) (lookupinfo.DomainInfo, error) {
	log.Debugf("Querying whois for domain: %s", domain)

	var domainInfo = lookupinfo.DomainInfo{
		LookupType: constant.LookupTypeWhois,
		ViaProxy:   useProxy,
	}

	// Check if the TLD is supported
	whoisServer, ok := WhoisSupportedTlds[tld]
	if !ok {
		log.Warnf("Whois not supported for TLD: %s", tld)
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNotSupportedTld, tld)
	}

	// Read the configuration
	cfg := config.GetConfig()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.WhoisTimeout)*time.Second)
	defer cancel()
	release, err := network.Acquire(ctx, "whois")
	if err != nil {
		return domainInfo, err
	}
	defer release()
	if err := network.Wait(ctx, "whois", whoisServer); err != nil {
		return domainInfo, err
	}
	domainInfo.Source = whoisServer

	// Create the address for the WHOIS server
	whoisAddr := net.JoinHostPort(whoisServer, "43")

	// Create the connection
	conn := net.Conn(nil)
	if useProxy {
		proxyServer := net.JoinHostPort(cfg.SocketProxyHost, strconv.Itoa(cfg.SocketProxyPort))
		var proxyDialer proxy.Dialer
		var err error

		baseTCPDialer := &net.Dialer{
			Timeout:  time.Second * time.Duration(cfg.WhoisTimeout),
			Deadline: time.Now().Add(time.Second * time.Duration(cfg.WhoisTimeout*2)),
		}

		if cfg.SocketProxyAuth {
			proxyAuth := proxy.Auth{
				User:     cfg.SocketProxyUser,
				Password: cfg.SocketProxyPassword,
			}

			proxyDialer, err = proxy.SOCKS5("tcp", proxyServer, &proxyAuth, baseTCPDialer)
			if err != nil {
				log.Warnf("Failed to connect to the proxy server with auth: %s", err)
				return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorConnectToProxy, err.Error())
			}
		} else {
			proxyDialer, err = proxy.SOCKS5("tcp", proxyServer, nil, baseTCPDialer)
			if err != nil {
				log.Warnf("Failed to connect to the proxy server without auth: %s", err)
				return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorConnectToProxy, err.Error())
			}
		}

		proxyConn, err := proxyDialer.(proxy.ContextDialer).DialContext(ctx, "tcp", whoisAddr)
		if err != nil {
			log.Warnf("Failed to connect to the whois server %s via proxy: %s", whoisAddr, err)
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
		}

		// Set timeouts for the proxy connection
		err = proxyConn.SetDeadline(time.Now().Add(time.Second * time.Duration(cfg.WhoisTimeout)))
		if err != nil {
			log.Warnf("Failed to set deadline for proxy connection: %s", err)
			proxyConn.Close()
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
		}

		conn = proxyConn
	} else {
		rawConn, err := (&net.Dialer{Timeout: time.Second * time.Duration(cfg.WhoisTimeout)}).DialContext(ctx, "tcp", whoisAddr)
		if err != nil {
			log.Warnf("Failed to connect to the whois server %s: %s", whoisAddr, err)
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
		}

		// Set timeouts for the direct connection
		err = rawConn.SetDeadline(time.Now().Add(time.Second * time.Duration(cfg.WhoisTimeout)))
		if err != nil {
			log.Warnf("Failed to set deadline for connection: %s", err)
			rawConn.Close()
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
		}

		conn = rawConn
	}

	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	log.Infof("Querying WHOIS for domain: %s with TLD: %s on server: %s", domain, tld, whoisServer)

	// Set write deadline
	err = conn.SetWriteDeadline(time.Now().Add(time.Second * time.Duration(cfg.WhoisTimeout)))
	if err != nil {
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
	}

	queryInfo := fmt.Sprintf("%s\r\n", domain)
	if maputil.HasKey(WhoisTldOptions, tld) {
		queryInfo = fmt.Sprintf("%s %s\r\n", WhoisTldOptions[tld], domain)
	}

	// Write the query to the server
	_, err = conn.Write([]byte(queryInfo))
	if err != nil {
		log.Warnf("Failed to write query: %s", err)
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
	}

	// Set read deadline
	err = conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(cfg.WhoisTimeout)))
	if err != nil {
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
	}

	// Read the response from the server
	var buf bytes.Buffer
	_, err = io.Copy(&buf, io.LimitReader(conn, 2*1024*1024+1))
	if err != nil {
		log.Warnf("Failed to read WHOIS response: %s", err)
		if _, ok := err.(net.Error); ok {
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
		}
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisServerFailed, err.Error())
	}

	if buf.Len() > 2*1024*1024 {
		return domainInfo, fmt.Errorf("WHOIS response too large")
	}
	// Get the query result from the buffer
	queryResult := buf.String()
	domainInfo.RawResponse = queryResult

	// Check if the query result is empty
	if strutil.Trim(queryResult) == "" {
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNoContentInWhoisResponse, domain)
	}

	// Use the matcher corresponding to the TLD to parse the WHOIS data
	if matcher, ok := WhoisMatchers[tld]; ok {
		domainInfo, err = ParseWhoisResponse(queryResult, domain, matcher)
		domainInfo.ViaProxy = useProxy
		domainInfo.Source = whoisServer
		if err != nil {
			if errors.Is(err, lookuperror.ErrorReservedDomain) {
				return domainInfo, err
			}
			if err.Error() == "domain not found" {
				log.Infof("Domain %s is not registered", domain)
				return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisNotFound, domain)
			} else {
				log.Errorf("Failed to parse WHOIS response for domain %s: %s", domain, err)
				return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorParseWhoisResponse, err.Error())
			}
		}

		return domainInfo, nil
	} else {
		log.Error("No parsing rule for TLD: ", tld)
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNoParseRuleForTld, tld)
	}
}

func WhoisQuery(domain, tld string, viaProxy bool) (lookupinfo.DomainInfo, error) {
	return WhoisQueryContext(context.Background(), domain, tld, viaProxy)
}
