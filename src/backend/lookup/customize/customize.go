package customize

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"

	"github.com/duke-git/lancet/v2/strutil"
)

func CustomizeLookup(ctx context.Context, domain string, provider config.WhoisApi) (lookupinfo.DomainInfo, error) {
	info := lookupinfo.DomainInfo{LookupType: provider.ApiName, DomainName: domain}
	release, err := network.Acquire(ctx, "custom:"+provider.ApiName, provider.ConcurrencyLimit)
	if err != nil {
		return info, err
	}
	defer release()
	address := strings.ReplaceAll(provider.ApiUrl, "{domain}", url.QueryEscape(domain))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return info, lookuperror.ErrorCustomizeApiServerResponse
	}
	body, code, err := network.ReadResponse(network.PrivateLookupClient, req)
	switch {
	case ctx.Err() != nil:
		return info, ctx.Err()
	case code == http.StatusTooManyRequests:
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
