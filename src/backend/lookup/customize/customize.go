package customize

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"typonamer/lookup/network"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"

	"github.com/duke-git/lancet/v2/strutil"
)

func SetupLimiter() {}

func CustomizeLookupContext(ctx context.Context, domain string, queryType string) (lookupinfo.DomainInfo, error) {
	var domainInfo = lookupinfo.DomainInfo{
		DomainName: domain,
		LookupType: queryType,
	}

	cfg := config.GetConfig()

	var apiInfo config.WhoisApi
	for _, api := range cfg.WhoisApis {
		if api.ApiName == queryType {
			apiInfo = api
			break
		}
	}

	if apiInfo.ApiName == "" {
		log.Debugf("Invalid query type: %s, no whois api found", queryType)
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorInvalidQueryType, queryType)
	}

	apiUrl := strutil.TemplateReplace(apiInfo.ApiUrl, map[string]string{
		"domain": domain,
	})

	release, err := network.Acquire(ctx, "custom:"+queryType)
	if err != nil {
		return domainInfo, err
	}
	defer release()
	response, err := getResponse(ctx, apiUrl)
	domainInfo.Source = "custom:" + queryType

	if err != nil {
		log.Errorf("Request whois api %s response error: %v", apiInfo.ApiName, err)
		if response != "" {
			domainInfo.RawResponse = response
		} else {
			domainInfo.RawResponse = err.Error()
		}
		return domainInfo, err
	}

	domainInfo.RawResponse = response

	if strutil.ContainsAny(response, apiInfo.FreeText) && strutil.ContainsAny(response, apiInfo.TakenText) {
		log.Errorf("Request whois api %s with domain %s, response both contains free and taken text", apiInfo.ApiName, domain)
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorCustomizeApiWhoisResult, domain)
	} else if strutil.ContainsAny(response, apiInfo.FreeText) {
		log.Debugf("Request whois api %s with domain %s, response contains all free text", apiInfo.ApiName, domain)
		domainInfo.CustomizedResult = constant.DomainRegisterStatusFree
	} else if strutil.ContainsAny(response, apiInfo.TakenText) {
		log.Debugf("Request whois api %s with domain %s, response contains all taken text", apiInfo.ApiName, domain)
		domainInfo.CustomizedResult = constant.DomainRegisterStatusTaken
	} else {
		log.Errorf("Request whois api %s with domain %s, response not contains free or taken text", apiInfo.ApiName, domain)
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorCustomizeApiWhoisResult, domain)
	}

	return domainInfo, nil
}

func getResponse(ctx context.Context, apiURL string) (string, error) {
	cfg := config.GetConfig()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.WhoisTimeout)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", lookuperror.ErrorCustomizeApiServerResponse
	}
	client := &http.Client{Transport: network.Transport{Base: http.DefaultTransport, Protocol: "custom"}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", lookuperror.ErrorCustomizeApiServerResponse
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return "", lookuperror.ErrorRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", lookuperror.ErrorCustomizeApiServerResponse
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return "", lookuperror.ErrorCustomizeApiServerResponse
	}
	return strings.TrimSpace(string(data)), nil
}
func CustomizeLookup(domain, mode string) (lookupinfo.DomainInfo, error) {
	return CustomizeLookupContext(context.Background(), domain, mode)
}
