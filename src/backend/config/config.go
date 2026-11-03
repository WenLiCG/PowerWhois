package config

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"typonamer/log"
)

var mu sync.RWMutex
var configFile string
var config = Config{LogLevel: "Info", WhoisTimeout: 5, DnsTimeout: 3, RdapTimeout: 10,
	DnsConcurrency: 16, WhoisConcurrency: 4, RdapConcurrency: 4, DnsRate: 20, WhoisRate: 2, RdapRate: 4,
	WebCheckConcurrencyLimit: 10, WebCheckDomainLimit: 500, BulkCheckConcurrencyLimit: 32, RetryMax: 2, RetryInterval: 1, AuthExpireDays: 1}

type Config struct {
	LogLevel string `json:"logLevel"` //日志等级

	AuthUsername   string `json:"authUsername"`   //认证账号
	AuthPassword   string `json:"authPassword"`   //认证密码
	AuthExpireDays int    `json:"authExpireDays"` //认证有效期
	JwtSecretKey   string `json:"jwtSecretKey"`   //JWT密钥

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
	next := GetConfig()
	if err := v.Unmarshal(&next); err != nil {
		return err
	}
	for key, dst := range map[string]*string{"POWERWHOIS_ADMIN_USER": &next.AuthUsername, "POWERWHOIS_ADMIN_PASSWORD": &next.AuthPassword, "POWERWHOIS_JWT_SECRET": &next.JwtSecretKey} {
		if value := os.Getenv(key); value != "" {
			*dst = value
		}
	}
	if err := Validate(next); err != nil {
		return err
	}
	mu.Lock()
	configFile = path
	config = next
	mu.Unlock()
	log.SetLevel(next.LogLevel)
	return nil
}
func GetConfig() Config { mu.RLock(); defer mu.RUnlock(); return config }
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
	if c.AuthUsername == "" || len(c.AuthPassword) < 12 || len(c.JwtSecretKey) < 32 {
		return fmt.Errorf("configure administrator credentials (password >=12, JWT secret >=32 characters)")
	}
	if c.AuthExpireDays < 1 || c.AuthExpireDays > 30 {
		return fmt.Errorf("auth expiry must be 1..30 days")
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
func UpdateConfig(next Config) error {
	mu.Lock()
	defer mu.Unlock()
	for env, value := range map[string]string{"POWERWHOIS_ADMIN_USER": next.AuthUsername, "POWERWHOIS_ADMIN_PASSWORD": next.AuthPassword, "POWERWHOIS_JWT_SECRET": next.JwtSecretKey} {
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
	next.GlobalProxyTlds = trimTlds(next.GlobalProxyTlds)
	next.MixedProxyTlds = trimTlds(next.MixedProxyTlds)
	next.MixedDnsTlds = trimTlds(next.MixedDnsTlds)
	if err := Validate(next); err != nil {
		return err
	}
	if err := WriteConfig(next); err != nil {
		return err
	}
	config = next
	log.SetLevel(next.LogLevel)
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
func trimTlds(tlds []string) []string {
	out := make([]string, 0, len(tlds))
	for _, tld := range tlds {
		if tld = strings.Trim(strings.ToLower(strings.TrimSpace(tld)), "."); tld != "" {
			out = append(out, tld)
		}
	}
	return out
}
