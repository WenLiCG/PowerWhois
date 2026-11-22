package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"typonamer/constant"
	"typonamer/log"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Published configurations and their slices are immutable.
var current atomic.Pointer[Config]
var saveMu sync.Mutex
var configFile string

type Config struct {
	LogLevel string `json:"logLevel" yaml:"LogLevel"`

	AuthUsername   string `json:"authUsername" yaml:"AuthUsername"`
	AuthPassword   string `json:"-" yaml:"-"`
	AuthExpireDays int    `json:"authExpireDays" yaml:"AuthExpireDays"`
	JwtSecretKey   string `json:"-" yaml:"-"`

	WhoisTimeout int `json:"whoisTimeout" yaml:"WhoisTimeout"`
	DnsTimeout   int `json:"dnsTimeout" yaml:"DnsTimeout"`

	RetryOnTimeout bool `json:"retryOnTimeout" yaml:"RetryOnTimeout"`
	RetryInterval  int  `json:"retryInterval" yaml:"RetryInterval"`
	RetryMax       int  `json:"retryMax" yaml:"RetryMax"`

	GlobalProxyTlds []string `json:"globalProxyTlds" yaml:"GlobalProxyTlds"`

	MixedProxyTlds []string `json:"mixedProxyTlds" yaml:"MixedProxyTlds"`
	MixedDnsTlds   []string `json:"mixedDnsTlds" yaml:"MixedDnsTlds"`

	SocketProxyHost     string `json:"socketProxyHost" yaml:"SocketProxyHost"`
	SocketProxyPort     int    `json:"socketProxyPort" yaml:"SocketProxyPort"`
	SocketProxyAuth     bool   `json:"socketProxyAuth" yaml:"SocketProxyAuth"`
	SocketProxyUser     string `json:"socketProxyUser" yaml:"SocketProxyUser"`
	SocketProxyPassword string `json:"socketProxyPassword" yaml:"SocketProxyPassword"`

	BulkCheckConcurrencyLimit int `json:"bulkCheckConcurrencyLimit" yaml:"BulkCheckConcurrencyLimit"`

	WebCheckConcurrencyLimit int `json:"webCheckConcurrencyLimit" yaml:"WebCheckConcurrencyLimit"`
	WebCheckDomainLimit      int `json:"webCheckDomainLimit" yaml:"WebCheckDomainLimit"`

	// PublicDomainsPerMinute limits anonymous web lookups per client IP.
	PublicDomainsPerMinute int `json:"publicDomainsPerMinute" yaml:"PublicDomainsPerMinute"`

	TypoDefaultCcTlds      []CcTld  `json:"typoDefaultCcTlds" yaml:"TypoDefaultCcTlds"`
	TypoCustomizedReplaces []string `json:"typoCustomizedReplaces" yaml:"TypoCustomizedReplaces"`

	RegisterApis []RegisterApi `json:"registerApis" yaml:"RegisterApis"`
	WhoisApis    []WhoisApi    `json:"whoisApis" yaml:"WhoisApis"`
}

type CcTld struct {
	Tld        string `json:"tld" yaml:"Tld"`
	IsSelected bool   `json:"isSelected" yaml:"IsSelected"`
}

type RegisterApi struct {
	ApiName          string   `json:"apiName" yaml:"ApiName"`
	ApiUrl           string   `json:"apiUrl" yaml:"ApiUrl"`
	SuccessText      []string `json:"successText" yaml:"SuccessText"`
	FailText         []string `json:"failText" yaml:"FailText"`
	ConcurrencyLimit int      `json:"concurrencyLimit" yaml:"ConcurrencyLimit"`
}

type WhoisApi struct {
	ApiName          string   `json:"apiName" yaml:"ApiName"`
	ApiUrl           string   `json:"apiUrl" yaml:"ApiUrl"`
	FreeText         []string `json:"freeText" yaml:"FreeText"`
	TakenText        []string `json:"takenText" yaml:"TakenText"`
	ConcurrencyLimit int      `json:"concurrencyLimit" yaml:"ConcurrencyLimit"`
}

var defaults = Config{
	LogLevel: "Info", AuthUsername: "admin", AuthExpireDays: 1,
	WhoisTimeout: 6, DnsTimeout: 5, RetryInterval: 3, RetryMax: 2,
	BulkCheckConcurrencyLimit: 15, WebCheckConcurrencyLimit: 15, WebCheckDomainLimit: 500,
	PublicDomainsPerMinute: 500,
}

func Load(path string) error {
	next := defaults
	reader := viper.New()
	reader.SetConfigFile(path)
	if err := reader.ReadInConfig(); err != nil {
		return fmt.Errorf("cannot read configuration file %s", path)
	}
	if err := reader.Unmarshal(&next); err != nil {
		return errors.New("cannot decode configuration fields")
	}
	if err := applyRequiredSecretsTo(&next); err != nil {
		return err
	}
	normalize(&next)
	if err := validate(next); err != nil {
		return err
	}

	configFile = path
	current.Store(&next)
	log.SetLevel(next.LogLevel)
	return nil
}

func applyRequiredSecretsTo(cfg *Config) error {
	password := strings.TrimSpace(os.Getenv("POWERWHOIS_ADMIN_PASSWORD"))
	secret := strings.TrimSpace(os.Getenv("POWERWHOIS_JWT_SECRET"))
	var missing []string
	if password == "" {
		missing = append(missing, "POWERWHOIS_ADMIN_PASSWORD")
	}
	if secret == "" {
		missing = append(missing, "POWERWHOIS_JWT_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("refusing to start: required environment variable not set: %s", strings.Join(missing, ", "))
	}
	cfg.AuthPassword, cfg.JwtSecretKey = password, secret
	return nil
}

func GetConfig() Config {
	if cfg := current.Load(); cfg != nil {
		return *cfg
	}
	return defaults
}

func PublicSettings(cfg Config) Config {
	cfg.AuthPassword, cfg.JwtSecretKey, cfg.SocketProxyPassword = "", "", ""
	cfg.RegisterApis = slices.Clone(cfg.RegisterApis)
	for i := range cfg.RegisterApis {
		cfg.RegisterApis[i].ApiUrl = ""
	}
	cfg.WhoisApis = slices.Clone(cfg.WhoisApis)
	for i := range cfg.WhoisApis {
		cfg.WhoisApis[i].ApiUrl = ""
	}
	return cfg
}

func UpdateConfig(next Config) (Config, error) {
	saveMu.Lock()
	defer saveMu.Unlock()
	prev := GetConfig()

	next.AuthPassword, next.JwtSecretKey = prev.AuthPassword, prev.JwtSecretKey
	if next.SocketProxyPassword == "" {
		next.SocketProxyPassword = prev.SocketProxyPassword
	}
	normalize(&next)
	for i := range next.RegisterApis {
		if next.RegisterApis[i].ApiUrl == "" {
			for _, old := range prev.RegisterApis {
				if old.ApiName == next.RegisterApis[i].ApiName {
					next.RegisterApis[i].ApiUrl = old.ApiUrl
					break
				}
			}
		}
	}
	for i := range next.WhoisApis {
		if next.WhoisApis[i].ApiUrl == "" {
			for _, old := range prev.WhoisApis {
				if old.ApiName == next.WhoisApis[i].ApiName {
					next.WhoisApis[i].ApiUrl = old.ApiUrl
					break
				}
			}
		}
	}
	if err := validate(next); err != nil {
		return Config{}, err
	}
	if err := writeConfig(configFile, next); err != nil {
		log.Error("Could not save configuration file")
		return Config{}, ErrWriteConfig
	}

	current.Store(&next)
	log.SetLevel(next.LogLevel)
	return PublicSettings(next), nil
}

var ErrWriteConfig = errors.New("could not save configuration")

func normalize(c *Config) {
	if c.PublicDomainsPerMinute == 0 {
		// Older configuration files and clients do not send this field.
		c.PublicDomainsPerMinute = defaults.PublicDomainsPerMinute
	}
	c.LogLevel = strings.TrimSpace(c.LogLevel)
	c.AuthUsername = strings.TrimSpace(c.AuthUsername)
	for _, list := range [][]string{c.GlobalProxyTlds, c.MixedProxyTlds, c.MixedDnsTlds} {
		for i := range list {
			list[i] = strings.Trim(strings.ToLower(strings.TrimSpace(list[i])), ".")
		}
	}
	for i := range c.TypoDefaultCcTlds {
		tld := &c.TypoDefaultCcTlds[i].Tld
		*tld = strings.Trim(strings.ToLower(strings.TrimSpace(*tld)), ".")
	}
	for i := range c.RegisterApis {
		p := &c.RegisterApis[i]
		p.ApiName, p.ApiUrl = strings.TrimSpace(p.ApiName), strings.TrimSpace(p.ApiUrl)
		p.SuccessText, p.FailText = trimMarkers(p.SuccessText), trimMarkers(p.FailText)
	}
	for i := range c.WhoisApis {
		p := &c.WhoisApis[i]
		p.ApiName, p.ApiUrl = strings.TrimSpace(p.ApiName), strings.TrimSpace(p.ApiUrl)
		p.FreeText, p.TakenText = trimMarkers(p.FreeText), trimMarkers(p.TakenText)
	}
}

func trimMarkers(markers []string) []string {
	for i := range markers {
		markers[i] = strings.TrimSpace(markers[i])
	}
	return slices.DeleteFunc(markers, func(s string) bool { return s == "" })
}

func validate(c Config) error {
	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "error", "off":
	default:
		return errors.New("invalid logLevel")
	}
	if c.AuthUsername == "" || c.AuthExpireDays < 1 || c.AuthExpireDays > 365 {
		return errors.New("invalid administrator username or session lifetime")
	}
	if c.WhoisTimeout < 1 || c.WhoisTimeout > 60 || c.DnsTimeout < 1 || c.DnsTimeout > 60 {
		return errors.New("WHOIS and DNS timeouts must be 1–60 seconds")
	}
	if c.RetryMax < 0 || c.RetryMax > 5 || c.RetryInterval < 0 || c.RetryInterval > 30 {
		return errors.New("invalid retry count or interval")
	}
	if c.WebCheckDomainLimit < 1 || c.WebCheckDomainLimit > 5000 ||
		c.WebCheckConcurrencyLimit < 1 || c.WebCheckConcurrencyLimit > 64 ||
		c.BulkCheckConcurrencyLimit < 1 || c.BulkCheckConcurrencyLimit > 128 {
		return errors.New("invalid domain limit or worker count")
	}
	if c.PublicDomainsPerMinute < 1 || c.PublicDomainsPerMinute > 100000 {
		return errors.New("anonymous domains per minute must be 1–100000")
	}
	if len(c.TypoCustomizedReplaces) > 200 || len(c.TypoDefaultCcTlds) > 100 {
		return errors.New("too many Typo options")
	}
	if c.SocketProxyPort < 0 || c.SocketProxyPort > 65535 || (c.SocketProxyHost != "" && c.SocketProxyPort == 0) {
		return errors.New("invalid SOCKS proxy port")
	}
	registerNames := make(map[string]bool)
	for _, p := range c.RegisterApis {
		if registerNames[p.ApiName] || !validProvider(p.ApiName, p.ApiUrl, p.ConcurrencyLimit, p.SuccessText, p.FailText) {
			return errors.New("invalid or duplicate registration provider")
		}
		registerNames[p.ApiName] = true
	}
	whoisNames := map[string]bool{constant.WhoisQuery: true, constant.WhoisQueryWithProxy: true, constant.DnsQuery: true, constant.MixedQuery: true}
	for _, p := range c.WhoisApis {
		if whoisNames[p.ApiName] || !validProvider(p.ApiName, p.ApiUrl, p.ConcurrencyLimit, p.FreeText, p.TakenText) {
			return errors.New("invalid or duplicate lookup provider")
		}
		whoisNames[p.ApiName] = true
	}
	return nil
}

func validProvider(name, template string, concurrent int, positive, negative []string) bool {
	if name == "" || len(name) > 100 || !strings.Contains(template, "{domain}") ||
		concurrent < 1 || concurrent > 64 || len(positive) == 0 || len(negative) == 0 {
		return false
	}
	address, err := url.Parse(strings.ReplaceAll(template, "{domain}", "example.test"))
	return err == nil && address.Hostname() != "" && address.Fragment == "" &&
		(address.Scheme == "https" || address.Scheme == "http")
}

func writeConfig(path string, cfg Config) error {
	if path == "" {
		return ErrWriteConfig
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".powerwhois-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
