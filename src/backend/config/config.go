package config

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/idna"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"typonamer/log"
)

var mu sync.RWMutex
var configFile string
var changeHooks []func()
var defaults = Config{LogLevel: "Info", WhoisTimeout: 5, DnsTimeout: 3, RdapTimeout: 10,
	DnsConcurrency: 15, WhoisConcurrency: 15, RdapConcurrency: 4, DnsRate: 20, WhoisRate: 2, RdapRate: 4,
	WebCheckConcurrencyLimit: 15, WebCheckDomainLimit: 500, BulkCheckConcurrencyLimit: 32, RetryMax: 2, RetryInterval: 1, AuthExpireDays: 1}
var config = defaults

// RegisterChangeHook registers a callback invoked after configuration loads or updates.
func RegisterChangeHook(fn func()) {
	if fn == nil {
		return
	}
	mu.Lock()
	changeHooks = append(changeHooks, fn)
	mu.Unlock()
}

func notifyChangeHooks() {
	mu.RLock()
	hooks := slices.Clone(changeHooks)
	mu.RUnlock()
	for _, fn := range hooks {
		fn()
	}
}

type Config struct {
	LogLevel string `json:"logLevel"` //日志等级

	AuthUsername     string `json:"authUsername"`   //认证账号
	AuthPassword     string `json:"authPassword"`   //认证密码
	AuthExpireDays   int    `json:"authExpireDays"` //认证有效期
	JwtSecretKey     string `json:"jwtSecretKey"`   //JWT密钥
	authPasswordHash string // File-only override created by an explicit administrator password change.

	RdapTimeout      int `json:"rdapTimeout"`
	DnsConcurrency   int `json:"dnsConcurrency"`
	WhoisConcurrency int `json:"whoisConcurrency"`
	RdapConcurrency  int `json:"rdapConcurrency"`
	DnsRate          int `json:"dnsRate"`
	WhoisRate        int `json:"whoisRate"`
	RdapRate         int `json:"rdapRate"`
	WhoisTimeout     int `json:"whoisTimeout"` //whois超时
	DnsTimeout       int `json:"dnsTimeout"`   //DNS超时

	RetryOnTimeout bool `json:"retryOnTimeout"` //是否重试
	RetryInterval  int  `json:"retryInterval"`  //重试间隔
	RetryMax       int  `json:"retryMax"`       //最大重试次数

	GlobalProxyTlds []string `json:"globalProxyTlds"` //全局代理TLD

	MixedProxyTlds []string `json:"mixedProxyTlds"` //混合查询代理TLD
	MixedDnsTlds   []string `json:"mixedDnsTlds"`   //混合查询DNS TLDS

	SocketProxyHost     string `json:"socketProxyHost"`     //代理服务器地址
	SocketProxyPort     int    `json:"socketProxyPort"`     //代理服务器端口
	SocketProxyAuth     bool   `json:"socketProxyAuth"`     //代理服务器认证
	SocketProxyUser     string `json:"socketProxyUser"`     //代理服务器账号
	SocketProxyPassword string `json:"socketProxyPassword"` //代理服务器密码

	BulkCheckConcurrencyLimit int `json:"bulkCheckConcurrencyLimit"` //批量查询并发限制

	WebCheckConcurrencyLimit int `json:"webCheckConcurrencyLimit"` //网页查询并发限制
	WebCheckDomainLimit      int `json:"webCheckDomainLimit"`      //单次网页查询域名数量限制

	TypoDefaultCcTlds      []CcTld  `json:"typoDefaultCcTlds"`      //默认ccTLD
	TypoCustomizedReplaces []string `json:"typoCustomizedReplaces"` //自定义替换

	RegisterApis []RegisterApi `json:"registerApis"` //注册接口
	WhoisApis    []WhoisApi    `json:"whoisApis"`    //自定义whois接口
}

type CcTld struct {
	Tld        string `json:"tld"`        //TLD
	IsSelected bool   `json:"isSelected"` //是否选中
}

type RegisterApi struct {
	ApiName          string   `json:"apiName"`          //接口名称
	ApiUrl           string   `json:"apiUrl"`           //接口地址
	SuccessText      []string `json:"successText"`      //成功标识
	FailText         []string `json:"failText"`         //失败标识
	ConcurrencyLimit int      `json:"concurrencyLimit"` //并发限制
}

type WhoisApi struct {
	ApiName          string   `json:"apiName"`          //接口名称
	ApiUrl           string   `json:"apiUrl"`           //接口地址
	FreeText         []string `json:"freeText"`         //Free标识
	TakenText        []string `json:"takenText"`        //Taken标识
	ConcurrencyLimit int      `json:"concurrencyLimit"` //并发限制
}

// Load is explicit so importing helpers never opens configuration files or exits.
func Load(path string) error {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return err
	}
	next := defaults
	if err := v.Unmarshal(&next); err != nil {
		return err
	}
	for key, dst := range map[string]*string{"POWERWHOIS_ADMIN_USER": &next.AuthUsername, "POWERWHOIS_JWT_SECRET": &next.JwtSecretKey} {
		if value := os.Getenv(key); value != "" {
			*dst = value
		}
	}
	next.authPasswordHash = v.GetString("authPasswordHash")
	if next.authPasswordHash != "" {
		if _, err := bcrypt.Cost([]byte(next.authPasswordHash)); err != nil {
			return fmt.Errorf("invalid stored administrator password hash")
		}
		next.AuthPassword = ""
	} else if seed := os.Getenv("POWERWHOIS_ADMIN_PASSWORD"); seed != "" {
		next.AuthPassword = seed
	} else if strings.TrimSpace(next.AuthPassword) == "123456" {
		return fmt.Errorf("replace the historical configuration template password or set POWERWHOIS_ADMIN_PASSWORD")
	}
	next.normalizeProxy()
	next.normalizeRoutingTLDs()
	if err := Validate(next); err != nil {
		return err
	}
	mu.Lock()
	configFile = path
	config = clone(next)
	mu.Unlock()
	log.SetLevel(next.LogLevel)
	notifyChangeHooks()
	return nil
}
func GetConfig() Config { mu.RLock(); defer mu.RUnlock(); return clone(config) }

func (c Config) MatchesPassword(password string) bool {
	digest := sha256.Sum256([]byte(password))
	if c.authPasswordHash != "" {
		return bcrypt.CompareHashAndPassword([]byte(c.authPasswordHash), digest[:]) == nil
	}
	legacy := sha256.Sum256([]byte(c.AuthPassword))
	return c.AuthPassword != "" && subtle.ConstantTimeCompare(digest[:], legacy[:]) == 1
}

// AdminJWTKey preserves existing sessions until a password change publishes a new salted hash.
func (c Config) AdminJWTKey() []byte {
	key := []byte(c.JwtSecretKey)
	if c.authPasswordHash == "" {
		return key
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(c.authPasswordHash))
	return mac.Sum(nil)
}

func clone(c Config) Config {
	c.GlobalProxyTlds = slices.Clone(c.GlobalProxyTlds)
	c.MixedDnsTlds = slices.Clone(c.MixedDnsTlds)
	c.MixedProxyTlds = slices.Clone(c.MixedProxyTlds)
	c.TypoDefaultCcTlds = slices.Clone(c.TypoDefaultCcTlds)
	c.TypoCustomizedReplaces = slices.Clone(c.TypoCustomizedReplaces)
	c.WhoisApis = slices.Clone(c.WhoisApis)
	c.RegisterApis = slices.Clone(c.RegisterApis)
	for i := range c.WhoisApis {
		c.WhoisApis[i].FreeText = slices.Clone(c.WhoisApis[i].FreeText)
		c.WhoisApis[i].TakenText = slices.Clone(c.WhoisApis[i].TakenText)
	}
	for i := range c.RegisterApis {
		c.RegisterApis[i].SuccessText = slices.Clone(c.RegisterApis[i].SuccessText)
		c.RegisterApis[i].FailText = slices.Clone(c.RegisterApis[i].FailText)
	}
	return c
}

// PublicSettings never returns reusable credentials, including secrets embedded in API URLs.
func PublicSettings() map[string]interface{} {
	c := GetConfig()
	data, _ := json.Marshal(c)
	out := map[string]interface{}{}
	json.Unmarshal(data, &out)
	status := map[string]interface{}{
		"authUsername": map[string]bool{"configured": c.AuthUsername != "", "managed": os.Getenv("POWERWHOIS_ADMIN_USER") != ""},
	}
	for key, configured := range map[string]bool{"authPassword": c.AuthPassword != "" || c.authPasswordHash != "", "jwtSecretKey": c.JwtSecretKey != "", "socketProxyPassword": c.SocketProxyPassword != ""} {
		status[key] = map[string]bool{"configured": configured, "managed": key == "jwtSecretKey" && os.Getenv("POWERWHOIS_JWT_SECRET") != ""}
		out[key] = ""
	}
	for _, key := range []string{"whoisApis", "registerApis"} {
		if list, ok := out[key].([]interface{}); ok {
			for _, item := range list {
				a := item.(map[string]interface{})
				a["urlConfigured"] = a["apiUrl"] != ""
				a["apiUrl"] = ""
			}
		}
	}
	out["secrets"] = status
	return out
}
func Validate(c Config) error {
	if c.WhoisTimeout < 1 || c.WhoisTimeout > 120 || c.DnsTimeout < 1 || c.DnsTimeout > 120 || c.RdapTimeout < 1 || c.RdapTimeout > 120 {
		return fmt.Errorf("timeouts must be 1..120 seconds")
	}
	if c.RetryMax < 0 || c.RetryMax > 5 || c.RetryInterval < 0 || c.RetryInterval > 60 {
		return fmt.Errorf("invalid retry limits")
	}
	for _, n := range []int{c.WebCheckConcurrencyLimit, c.BulkCheckConcurrencyLimit, c.DnsConcurrency, c.WhoisConcurrency, c.RdapConcurrency, c.DnsRate, c.WhoisRate, c.RdapRate} {
		if n < 1 || n > 256 {
			return fmt.Errorf("concurrency and rates must be 1..256")
		}
	}
	if c.WebCheckDomainLimit < 1 || c.WebCheckDomainLimit > 10000 {
		return fmt.Errorf("web domain limit must be 1..10000")
	}
	if c.AuthUsername == "" || (strings.TrimSpace(c.AuthPassword) == "" && c.authPasswordHash == "") || len(c.JwtSecretKey) < 32 {
		return fmt.Errorf("configure an administrator username, a nonblank password and a JWT secret of at least 32 characters")
	}
	if c.AuthExpireDays < 1 || c.AuthExpireDays > 30 {
		return fmt.Errorf("auth expiry must be 1..30 days")
	}
	if err := ValidateProxy(c); err != nil {
		return err
	}
	names := map[string]bool{"dnsQuery": true, "whoisQuery": true, "mixedQuery": true, "whoisQueryWithProxy": true, "dns": true, "whois": true, "mixed": true, "proxy": true}
	for _, a := range c.WhoisApis {
		if a.ApiName == "" || a.ConcurrencyLimit < 1 || a.ConcurrencyLimit > 256 {
			return fmt.Errorf("invalid custom query API")
		}
		if names[a.ApiName] || !validAPI(a.ApiUrl, a.FreeText, a.TakenText) {
			return fmt.Errorf("custom API needs a unique name, HTTP(S) URL and nonempty, distinct response markers")
		}
		names[a.ApiName] = true
	}
	names = map[string]bool{}
	for _, a := range c.RegisterApis {
		if a.ApiName == "" || a.ConcurrencyLimit < 1 || a.ConcurrencyLimit > 256 {
			return fmt.Errorf("invalid registration API")
		}
		if names[a.ApiName] || !validAPI(a.ApiUrl, a.SuccessText, a.FailText) {
			return fmt.Errorf("registration API needs a unique name, HTTP(S) URL and nonempty, distinct response markers")
		}
		names[a.ApiName] = true
	}
	return nil
}
func validAPI(address string, positive, negative []string) bool {
	u, err := url.Parse(strings.ReplaceAll(address, "{domain}", "example.com"))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || len(positive) == 0 || len(negative) == 0 {
		return false
	}
	for _, markers := range [][]string{positive, negative} {
		for _, marker := range markers {
			if strings.TrimSpace(marker) == "" {
				return false
			}
		}
	}
	for _, p := range positive {
		for _, n := range negative {
			if p == n {
				return false
			}
		}
	}
	return true
}
func UpdateConfig(next Config, secretActions ...map[string]string) error {
	mu.Lock()
	defer mu.Unlock()
	next.authPasswordHash = config.authPasswordHash
	setPassword := false
	if len(secretActions) > 0 {
		for key, pair := range map[string][2]*string{"authPassword": {&next.AuthPassword, &config.AuthPassword}, "jwtSecretKey": {&next.JwtSecretKey, &config.JwtSecretKey}, "socketProxyPassword": {&next.SocketProxyPassword, &config.SocketProxyPassword}} {
			switch secretActions[0][key] {
			case "", "keep":
				*pair[0] = *pair[1]
			case "set":
				if *pair[0] == "" {
					return fmt.Errorf("%s replacement is empty", key)
				}
				if key == "authPassword" {
					setPassword = true
				}
			case "clear":
				if key == "authPassword" {
					return fmt.Errorf("administrator password cannot be cleared")
				}
				*pair[0] = ""
			default:
				return fmt.Errorf("invalid secret action for %s", key)
			}
		}
		for i := range next.WhoisApis {
			if next.WhoisApis[i].ApiUrl == "" {
				for _, old := range config.WhoisApis {
					if old.ApiName == next.WhoisApis[i].ApiName {
						next.WhoisApis[i].ApiUrl = old.ApiUrl
					}
				}
			}
		}
		for i := range next.RegisterApis {
			if next.RegisterApis[i].ApiUrl == "" {
				for _, old := range config.RegisterApis {
					if old.ApiName == next.RegisterApis[i].ApiName {
						next.RegisterApis[i].ApiUrl = old.ApiUrl
					}
				}
			}
		}
	}
	if setPassword || next.AuthPassword != config.AuthPassword {
		if strings.TrimSpace(next.AuthPassword) == "" {
			return fmt.Errorf("administrator password must not be blank")
		}
		// A fixed-size input retains every byte of long passwords beyond bcrypt's 72-byte limit.
		digest := sha256.Sum256([]byte(next.AuthPassword))
		hash, err := bcrypt.GenerateFromPassword(digest[:], bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		next.authPasswordHash = string(hash)
		next.AuthPassword = ""
	}
	for env, value := range map[string]string{"POWERWHOIS_ADMIN_USER": next.AuthUsername, "POWERWHOIS_JWT_SECRET": next.JwtSecretKey} {
		if managed := os.Getenv(env); managed != "" && value != managed {
			return fmt.Errorf("%s is managed by the service environment; change it there and restart", env)
		}
	}
	// Older Web clients omit protocol controls.
	if next.RdapTimeout == 0 {
		next.RdapTimeout = config.RdapTimeout
	}
	if next.DnsConcurrency == 0 {
		next.DnsConcurrency = config.DnsConcurrency
	}
	if next.WhoisConcurrency == 0 {
		next.WhoisConcurrency = config.WhoisConcurrency
	}
	if next.RdapConcurrency == 0 {
		next.RdapConcurrency = config.RdapConcurrency
	}
	if next.DnsRate == 0 {
		next.DnsRate = config.DnsRate
	}
	if next.WhoisRate == 0 {
		next.WhoisRate = config.WhoisRate
	}
	if next.RdapRate == 0 {
		next.RdapRate = config.RdapRate
	}
	next.normalizeProxy()
	next.normalizeRoutingTLDs()
	if err := Validate(next); err != nil {
		return err
	}
	if err := WriteConfig(next); err != nil {
		return err
	}
	config = clone(next)
	log.SetLevel(next.LogLevel)
	go notifyChangeHooks()
	return nil
}
func WriteConfig(next Config) error {
	// Encode instead of interpolating: passwords and API URLs may contain YAML syntax.
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	var fields map[string]interface{}
	if err = json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for env, key := range map[string]string{"POWERWHOIS_ADMIN_USER": "authUsername", "POWERWHOIS_ADMIN_PASSWORD": "authPassword", "POWERWHOIS_JWT_SECRET": "jwtSecretKey"} {
		if os.Getenv(env) != "" {
			fields[key] = ""
		}
	}
	if next.authPasswordHash != "" {
		fields["authPasswordHash"] = next.authPasswordHash
	}
	data, err = yaml.Marshal(fields)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(configFile), ".powerwhois-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), configFile)
}
func (c *Config) normalizeProxy() {
	c.SocketProxyHost = NormalizeProxyHost(c.SocketProxyHost)
	c.SocketProxyUser = strings.TrimSpace(c.SocketProxyUser)
	if c.SocketProxyHost == "" {
		c.SocketProxyPort = 0
		c.SocketProxyAuth = false
	}
}

// NormalizeProxyHost trims whitespace and strips optional socks5(h):// prefixes.
func NormalizeProxyHost(host string) string {
	host = strings.TrimSpace(host)
	lower := strings.ToLower(host)
	if strings.HasPrefix(lower, "socks5h://") {
		host = strings.TrimSpace(host[len("socks5h://"):])
	} else if strings.HasPrefix(lower, "socks5://") {
		host = strings.TrimSpace(host[len("socks5://"):])
	}
	return strings.TrimSuffix(host, "/")
}

// ValidateProxy checks SOCKS5 proxy fields when a proxy host is configured.
func ValidateProxy(c Config) error {
	host := NormalizeProxyHost(c.SocketProxyHost)
	if host == "" {
		return nil
	}
	lower := strings.ToLower(host)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("SOCKS5 proxy host must be a hostname or IP without http(s):// (e.g. brd.superproxy.io)")
	}
	if strings.ContainsAny(host, " \t\r\n/@?#") {
		return fmt.Errorf("invalid SOCKS5 proxy host: provide hostname or IP only")
	}
	if c.SocketProxyPort < 1 || c.SocketProxyPort > 65535 {
		return fmt.Errorf("SOCKS5 proxy port must be 1..65535")
	}
	if c.SocketProxyAuth && strings.TrimSpace(c.SocketProxyUser) == "" {
		return fmt.Errorf("SOCKS5 proxy username is required when proxy authentication is enabled")
	}
	return nil
}

func (c *Config) normalizeRoutingTLDs() {
	c.GlobalProxyTlds = trimTlds(c.GlobalProxyTlds)
	c.MixedProxyTlds = trimTlds(c.MixedProxyTlds)
	c.MixedDnsTlds = trimTlds(c.MixedDnsTlds)
}

func trimTlds(tlds []string) []string {
	if len(tlds) == 0 {
		return tlds
	}
	out := make([]string, 0, len(tlds))
	for _, tld := range tlds {
		if tld = strings.Trim(strings.ToLower(strings.TrimSpace(tld)), "."); tld != "" {
			if ascii, err := idna.Lookup.ToASCII(tld); err == nil {
				tld = strings.Trim(ascii, ".")
			}
			out = append(out, tld)
		}
	}
	return out
}
