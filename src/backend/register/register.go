package register

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"

	"github.com/duke-git/lancet/v2/strutil"
	"github.com/zh-five/golimit"
)

var limiterMu sync.RWMutex
var limiterList = map[string]*golimit.GoLimit{}

const (
	defaultRegisterTimeout = 10 * time.Second
)

func init() {
	SetupLimiter()
}

func SetupLimiter() {
	limiterMu.Lock()
	defer limiterMu.Unlock()
	limiterList = map[string]*golimit.GoLimit{}
	cfg := config.GetConfig()

	for _, api := range cfg.RegisterApis {
		limiterList[api.ApiName] = golimit.NewGoLimit(uint(api.ConcurrencyLimit))
	}
}

func Register(domain string, registerType string) (RegisterInfo, error) {
	return RegisterContext(context.Background(), domain, registerType)
}
func RegisterContext(ctx context.Context, domain string, registerType string) (RegisterInfo, error) {
	registerInfo := RegisterInfo{
		RegisterType: registerType,
		DomainName:   domain,
	}
	cfg := config.GetConfig()

	var apiInfo config.RegisterApi
	for _, api := range cfg.RegisterApis {
		if api.ApiName == registerType {
			apiInfo = api
			break
		}
	}

	if apiInfo.ApiName == "" {
		log.Debugf("Invalid register type: %s, no register api found", registerType)
		registerInfo.RegisterStatus = constant.RegisterStatusError
		registerInfo.RawResponse = fmt.Sprintf("Invalid register type: %s, no register api found", registerType)
		return registerInfo, fmt.Errorf("%w: %s", ErrorInvalidRegisterType, registerType)
	}

	apiUrl := strutil.TemplateReplace(apiInfo.ApiUrl, map[string]string{
		"domain": domain,
	})

	var response string
	var err error

	limiterMu.RLock()
	limiter, hasLimiter := limiterList[apiInfo.ApiName]
	limiterMu.RUnlock()
	if hasLimiter {

		var wg sync.WaitGroup
		wg.Add(1)

		limiter.Do(func() {
			defer wg.Done()
			response, err = sendRegisterRequest(ctx, apiUrl)
		})

		wg.Wait()
	} else {
		response, err = sendRegisterRequest(ctx, apiUrl)
	}

	if err != nil {
		log.Errorf("Request register api %s response error: %v", apiInfo.ApiName, err)
		registerInfo.RegisterStatus = constant.RegisterStatusError
		if response != "" {
			registerInfo.RawResponse = response
		} else {
			registerInfo.RawResponse = err.Error()
		}
		return registerInfo, err
	}

	registerInfo.RawResponse = response

	if strutil.ContainsAny(response, apiInfo.SuccessText) && strutil.ContainsAny(response, apiInfo.FailText) {
		log.Errorf("Request register api %s with domain %s, response both contains success and fail text", apiInfo.ApiName, domain)
		registerInfo.RegisterStatus = constant.RegisterStatusError
		return registerInfo, fmt.Errorf("%w: %s", ErrorCustomizeApiRegisterResult, domain)
	} else if strutil.ContainsAny(response, apiInfo.SuccessText) {
		log.Debugf("Request register api %s with domain %s, response contains all success text", apiInfo.ApiName, domain)
		registerInfo.RegisterStatus = constant.RegisterStatusSuccess
	} else if strutil.ContainsAny(response, apiInfo.FailText) {
		log.Debugf("Request register api %s with domain %s, response contains all fail text", apiInfo.ApiName, domain)
		registerInfo.RegisterStatus = constant.RegisterStatusFailed
	} else {
		log.Errorf("Request register api %s with domain %s, response not contains success or fail text", apiInfo.ApiName, domain)
		registerInfo.RegisterStatus = constant.RegisterStatusError
		// registerInfo.RawResponse = fmt.Sprintf("Request register api %s with domain %s, response not contains success or fail text", apiInfo.ApiName, domain)
		return registerInfo, fmt.Errorf("%w: %s", ErrorCustomizeApiRegisterResult, domain)
	}

	return registerInfo, nil
}

func sendRegisterRequest(ctx context.Context, apiURL string) (string, error) {
	client := &http.Client{Timeout: defaultRegisterTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid registration endpoint")
	}
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("registration request failed; outcome unknown, verify with registrar before retrying")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("registration service returned HTTP %d; verify outcome before retrying", response.StatusCode)
	}
	data, err := io.ReadAll(http.MaxBytesReader(nil, response.Body, 2*1024*1024))
	if err != nil {
		return "", fmt.Errorf("cannot read registration result; verify outcome before retrying")
	}
	return string(data), nil
}
