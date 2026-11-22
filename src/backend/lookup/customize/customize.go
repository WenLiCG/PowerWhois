package customize

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"

	"github.com/duke-git/lancet/v2/strutil"
)

// CustomizeLookup queries an administrator-configured provider. The timeout
// starts once a slot for the provider is held.
func CustomizeLookup(parent context.Context, domain string, provider config.WhoisApi, timeout time.Duration) (lookupinfo.DomainInfo, error) {
	info := lookupinfo.DomainInfo{LookupType: provider.ApiName, DomainName: domain}
	slotKey := "custom:" + provider.ApiName
	release, err := network.Acquire(parent, slotKey, provider.ConcurrencyLimit)
	if err != nil {
		return info, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	address := strings.ReplaceAll(provider.ApiUrl, "{domain}", url.QueryEscape(domain))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return info, lookuperror.ErrorCustomizeApiServerResponse
	}
	body, code, header, err := network.ReadResponseHeader(network.PrivateLookupClient, req)
	switch {
	case parent.Err() != nil:
		return info, parent.Err()
	case ctx.Err() != nil:
		return info, ctx.Err()
	case code == http.StatusTooManyRequests:
		network.Cooldown(slotKey, network.RetryAfter(header, time.Now()))
		return info, lookuperror.ErrorRateLimited
	case err != nil || code < 200 || code >= 300:
		return info, lookuperror.ErrorCustomizeApiServerResponse
	}

	free, taken := strutil.ContainsAny(body, provider.FreeText), strutil.ContainsAny(body, provider.TakenText)
	switch {
	case free && !taken:
		info.CustomizedResult = constant.DomainRegisterStatusFree
	case taken && !free:
		info.CustomizedResult = constant.DomainRegisterStatusTaken
	default:
		return info, lookuperror.ErrorCustomizeApiWhoisResult
	}
	info.RawResponse = "自定义查询接口返回了已配置的状态标记。"
	return info, nil
}
