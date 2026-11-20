package whoislib

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/proxydial"

	"github.com/duke-git/lancet/v2/maputil"
	"github.com/duke-git/lancet/v2/strutil"
	"golang.org/x/net/proxy"
)

// WhoisQuery function is used to query the WHOIS information for a given domain.
// If the useProxy parameter is set to true, it will use the proxy server to query the WHOIS information.
func WhoisQuery(domain string, tld string, useProxy bool) (lookupinfo.DomainInfo, error) {
	log.Debugf("Querying whois for domain: %s", domain)

	var domainInfo = lookupinfo.DomainInfo{
		LookupType: constant.LookupTypeWhois,
		ViaProxy:   useProxy,
	}

	cfg := config.GetConfig()
	if useProxy && (strings.TrimSpace(cfg.SocketProxyHost) == "" || cfg.SocketProxyPort <= 0) {
		return domainInfo, lookuperror.ErrorProxyNotConfigured
	}

	// Check if the TLD is supported
	whoisServer, ok := WhoisSupportedTlds[tld]
	if !ok {
		log.Warnf("Whois not supported for TLD: %s", tld)
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNotSupportedTld, tld)
	}

	// Create the address for the WHOIS server
	whoisAddr := net.JoinHostPort(whoisServer, "43")

	// Create the connection
	conn := net.Conn(nil)
	if useProxy {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(cfg.WhoisTimeout))
		defer cancel()
		forward := &proxydial.ForwardDialer{}
		var auth *proxy.Auth
		if cfg.SocketProxyAuth {
			auth = &proxy.Auth{User: cfg.SocketProxyUser, Password: cfg.SocketProxyPassword}
		}
		dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort(strings.TrimSpace(cfg.SocketProxyHost), strconv.Itoa(cfg.SocketProxyPort)), auth, forward)
		if err != nil {
			return domainInfo, lookuperror.ErrorConnectToProxy
		}
		proxyConn, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", whoisAddr)
		if err != nil {
			// Never expose the original proxy error or credentials in logs/results.
			return domainInfo, lookuperror.ErrorConnectToProxy
		}
		if err := proxyConn.SetDeadline(time.Now().Add(time.Second * time.Duration(cfg.WhoisTimeout))); err != nil {
			proxyConn.Close()
			return domainInfo, lookuperror.ErrorWhoisTimeout
		}
		conn = proxyConn
	} else {
		rawConn, err := net.DialTimeout("tcp", whoisAddr, time.Second*time.Duration(cfg.WhoisTimeout))
		if err != nil {
			log.Warnf("Failed to connect to the whois server %s: %s", whoisAddr, err)
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
		}

		// Set timeouts for the direct connection
		err = rawConn.SetDeadline(time.Now().Add(time.Second * time.Duration(cfg.WhoisTimeout)))
		if err != nil {
			log.Warnf("Failed to set deadline for connection: %s", err)
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
		}

		conn = rawConn
	}

	defer conn.Close()

	log.Infof("Querying WHOIS for domain: %s with TLD: %s on server: %s", domain, tld, whoisServer)

	// Set write deadline
	err := conn.SetWriteDeadline(time.Now().Add(time.Second * time.Duration(cfg.WhoisTimeout)))
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
	_, err = io.Copy(&buf, conn)
	if err != nil {
		log.Warnf("Failed to read WHOIS response: %s", err)
		if _, ok := err.(net.Error); ok {
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
		}
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisServerFailed, err.Error())
	}

	// Get the query result from the buffer
	queryResult := buf.String()
	domainInfo.RawResponse = queryResult

	log.Debugf("Whois query raw result: \n%s", queryResult)

	// Check if the query result is empty
	if strutil.Trim(queryResult) == "" {
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNoContentInWhoisResponse, domain)
	}

	// Use the matcher corresponding to the TLD to parse the WHOIS data
	if matcher, ok := WhoisMatchers[tld]; ok {
		domainInfo, err = ParseWhoisResponse(queryResult, domain, matcher)
		domainInfo.ViaProxy = useProxy
		if err != nil {
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
