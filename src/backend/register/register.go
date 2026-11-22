package register

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/network"
	"typonamer/utils"

	"github.com/duke-git/lancet/v2/strutil"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const requestTimeout = 10 * time.Second
const submitting = "submitting"

var ErrReconcile = errors.New("没有可核对的注册操作，或者操作仍在进行")

var registrarClient = func() *http.Client {
	transport := network.DirectTransport()
	// These legacy GETs may charge money. Avoid transport retries on reused connections.
	transport.DisableKeepAlives = true
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	return &http.Client{Transport: transport, CheckRedirect: network.NoRedirect, Timeout: requestTimeout}
}()

type record struct {
	RegisterInfo
	Started int64 `json:"started"`
}

func CanonicalTarget(input string) (string, error) {
	host, err := utils.TrimDomain(input)
	if err != nil {
		return "", err
	}
	name, err := utils.GetDomainSuffixPlusOne(host)
	if err != nil || name != host {
		return "", errors.New("注册只接受完整的可注册域名")
	}
	return name, nil
}

func ValidOperationID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil && parsed.String() == id
}

const keyPrefix = "powerwhois:registration:"

func key(domain string) string { return keyPrefix + domain }

var claim = redis.NewScript(`
local result = redis.call('HGET', KEYS[1], ARGV[1])
if result then return result end
local pending = redis.call('HGET', KEYS[1], '_pending')
if pending then return redis.call('HGET', KEYS[1], pending) end
local recent = redis.call('HGET', KEYS[1], '_recent')
if recent then
	local prior = redis.call('HGET', KEYS[1], recent)
	if prior and tonumber(ARGV[3]) - cjson.decode(prior).started < 900 then return prior end
end
redis.call('HSET', KEYS[1], ARGV[1], ARGV[2], '_pending', ARGV[1])
return ''
`)

var finish = redis.NewScript(`
if redis.call('HGET', KEYS[1], '_pending') ~= ARGV[1] then return 0 end
local previous = redis.call('HGET', KEYS[1], ARGV[1])
if not previous or previous ~= ARGV[2] then return 0 end
redis.call('HSET', KEYS[1], ARGV[1], ARGV[3])
if ARGV[4] == '1' then redis.call('HDEL', KEYS[1], '_pending') end
if ARGV[5] == 'success' then redis.call('HSET', KEYS[1], '_recent', ARGV[1]) end
return 1
`)

// notSubmitted explains a domain skipped because the administrator session ended.
const notSubmitted = "管理员会话已结束，此域名未提交"

// Register submits one domain. mayStart, when not nil, is consulted before the
// domain is claimed and again immediately before the request is sent; if it
// returns false nothing is sent. Once a request has been sent, its outcome is
// collected and stored even if the caller's context ends.
func Register(ctx context.Context, store *redis.Client, domain, operationID string, provider config.RegisterApi, mayStart func() bool) RegisterInfo {
	allowed := func() bool { return mayStart == nil || mayStart() }
	info := RegisterInfo{DomainName: domain, RegisterType: provider.ApiName, OperationID: operationID, RegisterStatus: constant.RegisterStatusError}
	name, err := CanonicalTarget(domain)
	if err != nil || !ValidOperationID(operationID) || store == nil {
		info.RawResponse = "注册参数无效或注册记录存储不可用"
		return info
	}
	info.DomainName = name
	address := strings.ReplaceAll(provider.ApiUrl, "{domain}", url.QueryEscape(name))
	if _, err := http.NewRequest(http.MethodGet, address, nil); err != nil {
		info.RawResponse = "注册接口地址无效"
		return info
	}
	release, err := network.Acquire(ctx, "register:"+provider.ApiName, provider.ConcurrencyLimit)
	if err != nil {
		info.RawResponse = "注册在提交前中断"
		return info
	}
	defer release()
	if !allowed() {
		// Not claimed and not sent, so there is nothing to record or reconcile.
		info.RawResponse = notSubmitted
		return info
	}

	entry := record{RegisterInfo: info, Started: time.Now().Unix()}
	entry.RegisterStatus = submitting
	initial, _ := json.Marshal(entry)
	claimCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	prev, err := claim.Run(claimCtx, store, []string{key(name)}, operationID, initial, entry.Started).Text()
	cancel()
	if err != nil {
		info.RegisterStatus, info.RawResponse = constant.RegisterStatusUnknown, "无法确认操作记录，提交已停止，请核对后重试"
		return info
	}
	if prev != "" {
		if err := json.Unmarshal([]byte(prev), &entry); err != nil {
			info.RegisterStatus, info.RawResponse = constant.RegisterStatusUnknown, "注册记录不可读，请先核对注册商"
			return info
		}
		if entry.RegisterStatus == submitting {
			entry.RegisterStatus, entry.RawResponse = constant.RegisterStatusUnknown, "已有注册操作，请先核对注册商"
		}
		return entry.RegisterInfo
	}

	switch {
	case ctx.Err() != nil:
		entry.RegisterStatus, entry.RawResponse = constant.RegisterStatusFailed, "注册在首次提交前中断"
	case !allowed():
		// Claimed but never sent: record a definite failure, which releases the domain.
		entry.RegisterStatus, entry.RawResponse = constant.RegisterStatusFailed, notSubmitted
	default:
		// Once accepted, collect and save the outcome even if the browser disconnects.
		callCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
		req, _ := http.NewRequestWithContext(callCtx, http.MethodGet, address, nil)
		req.Header.Set("Idempotency-Key", operationID+":"+name)
		body, status, err := network.ReadResponse(registrarClient, req)
		stop()
		entry.RegisterStatus, entry.RawResponse = classify(body, status, err, provider)
	}

	final, _ := json.Marshal(entry)
	finishCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	unlock := 0
	if entry.RegisterStatus != constant.RegisterStatusUnknown {
		unlock = 1
	}
	saved, err := finish.Run(finishCtx, store, []string{key(name)}, operationID, initial, final, unlock, entry.RegisterStatus).Int()
	if err != nil || saved != 1 {
		entry.RegisterStatus, entry.RawResponse = constant.RegisterStatusUnknown, "无法保存结果，请先用操作编号核对注册商"
	}
	log.Infof("Registration operation %s for %s: %s", operationID, name, entry.RegisterStatus)
	return entry.RegisterInfo
}

func classify(body string, status int, err error, provider config.RegisterApi) (string, string) {
	if err != nil || status < 200 || status >= 300 {
		return constant.RegisterStatusUnknown, "注册接口未返回可确认的结果，请先核对注册商"
	}
	ok := strutil.ContainsAny(body, provider.SuccessText)
	failed := strutil.ContainsAny(body, provider.FailText)
	switch {
	case ok && !failed:
		return constant.RegisterStatusSuccess, "注册商返回了配置的成功标记"
	case failed && !ok:
		return constant.RegisterStatusFailed, "注册商返回了配置的失败标记"
	default:
		return constant.RegisterStatusUnknown, "注册接口未返回唯一状态，请先核对注册商"
	}
}

func Reconcile(ctx context.Context, store *redis.Client, domain, operationID, outcome string) (RegisterInfo, error) {
	name, err := CanonicalTarget(domain)
	if err != nil || !ValidOperationID(operationID) || store == nil ||
		(outcome != constant.RegisterStatusSuccess && outcome != constant.RegisterStatusFailed) {
		return RegisterInfo{}, ErrReconcile
	}
	previous, err := store.HGet(ctx, key(name), operationID).Bytes()
	if errors.Is(err, redis.Nil) {
		return RegisterInfo{}, ErrReconcile
	}
	if err != nil {
		return RegisterInfo{}, err
	}
	var entry record
	if err := json.Unmarshal(previous, &entry); err != nil {
		return RegisterInfo{}, err
	}
	if entry.RegisterStatus != constant.RegisterStatusUnknown &&
		(entry.RegisterStatus != submitting || time.Since(time.Unix(entry.Started, 0)) < 30*time.Second) {
		return RegisterInfo{}, ErrReconcile
	}
	entry.RegisterStatus, entry.RawResponse = outcome, "管理员已向注册商核对结果"
	entry.Started = time.Now().Unix()
	final, _ := json.Marshal(entry)
	ok, err := finish.Run(ctx, store, []string{key(name)}, operationID, previous, final, 1, entry.RegisterStatus).Int()
	if err != nil {
		return RegisterInfo{}, err
	}
	if ok != 1 {
		return RegisterInfo{}, ErrReconcile
	}
	log.Infof("Registration operation %s for %s reconciled: %s", operationID, name, outcome)
	return entry.RegisterInfo, nil
}

// PendingRegistration is an operation that still blocks new submissions for
// its domain: either still submitting or waiting for administrator review.
type PendingRegistration struct {
	RegisterInfo
	StartedAt int64 `json:"startedAt"`
}

const maxPending = 500

// Pending lists operations that hold a domain's registration lock, oldest
// first, so an administrator can reconcile them after a page reload.
func Pending(ctx context.Context, store *redis.Client) ([]PendingRegistration, error) {
	if store == nil {
		return nil, errors.New("registration store unavailable")
	}
	items := []PendingRegistration{}
	iter := store.Scan(ctx, 0, keyPrefix+"*", 200).Iterator()
	for iter.Next(ctx) && len(items) < maxPending {
		hash := iter.Val()
		operation, err := store.HGet(ctx, hash, "_pending").Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		raw, err := store.HGet(ctx, hash, operation).Bytes()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var entry record
		if json.Unmarshal(raw, &entry) != nil {
			entry = record{RegisterInfo: RegisterInfo{
				OperationID: operation, DomainName: strings.TrimPrefix(hash, keyPrefix),
				RegisterStatus: constant.RegisterStatusUnknown, RawResponse: "注册记录不可读，请先核对注册商",
			}}
		}
		items = append(items, PendingRegistration{RegisterInfo: entry.RegisterInfo, StartedAt: entry.Started})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt < items[j].StartedAt })
	return items, nil
}

func FindProvider(cfg config.Config, name string) (config.RegisterApi, error) {
	for _, provider := range cfg.RegisterApis {
		if provider.ApiName == name {
			return provider, nil
		}
	}
	return config.RegisterApi{}, errors.New("未找到已配置的注册接口")
}
