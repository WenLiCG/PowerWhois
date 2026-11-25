package register

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"crypto/sha256"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/utils"

	"github.com/duke-git/lancet/v2/strutil"
)

var limiterMu sync.RWMutex
var limiterList map[string]chan struct{}
var auditStore *redis.Client
var errNotSubmitted = errors.New("管理员会话已结束，域名未提交")

// Configure once before accepting registration requests. Registration fails closed without storage.
func SetAuditStore(r *redis.Client) { auditStore = r }

const (
	defaultRegisterTimeout = 10 * time.Second
)

func init() {
	SetupLimiter()
}

func SetupLimiter() {
	limiterMu.Lock()
	defer limiterMu.Unlock()
	next := make(map[string]chan struct{})
	cfg := config.GetConfig()

	for _, api := range cfg.RegisterApis {
		slots := limiterList[api.ApiName]
		if slots == nil || cap(slots) != api.ConcurrencyLimit {
			slots = make(chan struct{}, api.ConcurrencyLimit)
		}
		next[api.ApiName] = slots
	}
	limiterList = next
}

func Register(domain string, registerType string) (RegisterInfo, error) {
	return RegisterContext(context.Background(), domain, registerType)
}
func RegisterContext(ctx context.Context, domain string, registerType string) (RegisterInfo, error) {
	return RegisterForSession(ctx, domain, registerType, nil)
}

// RegisterForSession checks the authorizing session again at the registrar submission boundary.
func RegisterForSession(ctx context.Context, domain string, registerType string, maySubmit func() bool) (registerInfo RegisterInfo, resultErr error) {
	canonical, validationErr := utils.TrimAndGetMainDomain(domain)
	if validationErr != nil {
		return RegisterInfo{RegisterStatus: constant.RegisterStatusError}, validationErr
	}
	domain = canonical
	registerInfo = RegisterInfo{
		RegisterType:   registerType,
		DomainName:     domain,
		RegisterStatus: constant.RegisterStatusError,
	}
	if maySubmit != nil && (ctx.Err() != nil || !maySubmit()) {
		registerInfo.RegisterStatus = constant.RegisterStatusFailed
		registerInfo.RawResponse = errNotSubmitted.Error()
		return registerInfo, errNotSubmitted
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
	if auditStore == nil {
		return registerInfo, fmt.Errorf("registration audit storage unavailable")
	}
	// A queued request must not outlive its ten-minute duplicate guard.
	limiterMu.RLock()
	slots := limiterList[registerType]
	limiterMu.RUnlock()
	if slots == nil {
		return registerInfo, fmt.Errorf("registration provider is unavailable")
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		registerInfo.RegisterStatus = constant.RegisterStatusFailed
		registerInfo.RawResponse = errNotSubmitted.Error()
		return registerInfo, errNotSubmitted
	}
	id := uuid.NewString()
	guard := fmt.Sprintf("powerwhois:registration-guard:%x", sha256.Sum256([]byte(domain)))
	accepted, auditErr := auditStore.SetNX(ctx, guard, id, 10*time.Minute).Result()
	if auditErr != nil || !accepted {
		registerInfo.AuditID, _ = auditStore.Get(ctx, guard).Result()
		registerInfo.RegisterStatus = "outcome_unknown"
		registerInfo.RawResponse = "最近已有注册提交或审计存储不可用，请先到注册商核实，禁止直接重复提交。"
		return registerInfo, fmt.Errorf("registration blocked; verify previous submission with registrar")
	}
	registerInfo.AuditID = id
	record := map[string]string{"id": id, "domain": domain, "api": registerType, "status": "submitting", "createdAt": time.Now().UTC().Format(time.RFC3339Nano)}
	persist := func() error {
		data, _ := json.Marshal(record)
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return auditStore.Set(c, "powerwhois:registration:"+id, data, 0).Err()
	}
	if e := persist(); e != nil {
		return registerInfo, fmt.Errorf("cannot persist registration intent; request not sent")
	}
	defer func() {
		record["status"] = registerInfo.RegisterStatus
		record["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		if e := persist(); e != nil {
			log.Error("Cannot persist registration outcome; verify registrar using audit ID ", id)
			registerInfo.RegisterStatus = "outcome_unknown"
			registerInfo.RawResponse = "Cannot persist outcome; verify registrar using audit ID " + id
			resultErr = fmt.Errorf("registration outcome could not be persisted; verify registrar")
			return
		}
		if resultErr == errNotSubmitted {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			// A late cleanup may only remove this attempt's guard.
			if err := auditStore.Eval(c, `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) end`, []string{guard}, id).Err(); err != nil {
				log.Error("Cannot release the guard for a registration that was not submitted")
			}
		}
	}()

	requestCtx := ctx
	if ctx.Err() != nil || (maySubmit != nil && !maySubmit()) {
		registerInfo.RegisterStatus = constant.RegisterStatusFailed
		registerInfo.RawResponse = errNotSubmitted.Error()
		return registerInfo, errNotSubmitted
	}
	if maySubmit != nil {
		// An already authorized request must finish even if the browser disconnects.
		requestCtx = context.WithoutCancel(ctx)
	}
	response, err := sendRegisterRequest(requestCtx, apiUrl, id)

	if err != nil {
		log.Errorf("Request register api %s response error: %v", apiInfo.ApiName, err)
		registerInfo.RegisterStatus = "outcome_unknown"
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
		registerInfo.RegisterStatus = "outcome_unknown"
		return registerInfo, fmt.Errorf("%w: %s", ErrorCustomizeApiRegisterResult, domain)
	} else if strutil.ContainsAny(response, apiInfo.SuccessText) {
		log.Debugf("Request register api %s with domain %s, response contains all success text", apiInfo.ApiName, domain)
		registerInfo.RegisterStatus = constant.RegisterStatusSuccess
	} else if strutil.ContainsAny(response, apiInfo.FailText) {
		log.Debugf("Request register api %s with domain %s, response contains all fail text", apiInfo.ApiName, domain)
		registerInfo.RegisterStatus = constant.RegisterStatusFailed
	} else {
		log.Errorf("Request register api %s with domain %s, response not contains success or fail text", apiInfo.ApiName, domain)
		registerInfo.RegisterStatus = "outcome_unknown"
		// registerInfo.RawResponse = fmt.Sprintf("Request register api %s with domain %s, response not contains success or fail text", apiInfo.ApiName, domain)
		return registerInfo, fmt.Errorf("%w: %s", ErrorCustomizeApiRegisterResult, domain)
	}

	return registerInfo, nil
}

func sendRegisterRequest(ctx context.Context, apiURL string, requestID ...string) (string, error) {
	// Fresh HTTP/1 connections prevent Transport from replaying a GET transaction on a reused connection.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableKeepAlives = true
	tr.Protocols = new(http.Protocols)
	tr.Protocols.SetHTTP1(true)
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: defaultRegisterTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid registration endpoint")
	}
	if len(requestID) > 0 {
		req.Header.Set("Idempotency-Key", requestID[0])
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
