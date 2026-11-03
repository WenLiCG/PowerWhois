package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"typonamer/log"

	"github.com/duke-git/lancet/v2/strutil"
	"github.com/spf13/viper"
)

var config Config
var configFile string

type Config struct {
	LogLevel string `json:"logLevel"` //日志等级

	AuthUsername   string `json:"authUsername"`   //认证账号
	AuthPassword   string `json:"authPassword"`   //认证密码
	AuthExpireDays int    `json:"authExpireDays"` //认证有效期
	JwtSecretKey   string `json:"jwtSecretKey"`   //JWT密钥

	WhoisTimeout int `json:"whoisTimeout"` //whois超时
	DnsTimeout   int `json:"dnsTimeout"`   //DNS超时

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

const (
	// configFileName is the name of the configuration file.
	// The file is located in the same directory as the executable.
	configFileName = "config"

	// configFileType is the type of the configuration file.
	// Currently, only YAML is supported.
	configFileType = "yaml"
)

func init() {
	// Get the path of the executable file.
	exePath, err := os.Executable()
	if err != nil {
		log.Error("Error getting executable file path: ", err)
		os.Exit(1)
	}

	// Get the directory of the executable file.
	execDir := filepath.Dir(exePath)

	// Set the configuration file path.
	// The configuration file is named config.yaml and located in the same directory as the executable.
	configFile = filepath.Join(execDir, fmt.Sprintf("%s.%s", configFileName, configFileType))

	// Set the configuration file name, type and path.
	viper.SetConfigName(configFileName)
	viper.SetConfigType(configFileType)
	viper.AddConfigPath(execDir)
	// viper.AddConfigPath(".")

	// Read the configuration file.
	err = viper.ReadInConfig()
	if err != nil {
		log.Error("Error reading config file: ", err)
		os.Exit(1)
	}

	// Unmarshal the configuration into the config variable.
	viper.Unmarshal(&config)

	// Print the configuration to the debug log at startup.
	log.Debugf("Read config: %+v", config)
}

func GetConfig() Config {
	return config
}

func UpdateConfig(newConfig Config) error {
	// Update the log level if it has changed.
	// The log level is special cased because it needs to be updated immediately.
	currentLogLevel := strutil.UpperSnakeCase(config.LogLevel)
	newLogLevel := strutil.UpperSnakeCase(newConfig.LogLevel)
	if currentLogLevel != newLogLevel {
		log.Debugf("Log level changed from %s to %s", currentLogLevel, newLogLevel)

		// Update the log level immediately.
		log.SetLevel(newLogLevel)
	}

	// Trim the TLDs to remove any whitespace.
	newConfig.GlobalProxyTlds = trimTlds(newConfig.GlobalProxyTlds)
	newConfig.MixedProxyTlds = trimTlds(newConfig.MixedProxyTlds)
	newConfig.MixedDnsTlds = trimTlds(newConfig.MixedDnsTlds)

	for i, tld := range newConfig.TypoDefaultCcTlds {
		newConfig.TypoDefaultCcTlds[i].Tld = strutil.Trim(tld.Tld, ".")
	}

	if len(newConfig.RegisterApis) > 0 {
		for i, api := range newConfig.RegisterApis {
			newConfig.RegisterApis[i].ApiName = strutil.Trim(api.ApiName)
			newConfig.RegisterApis[i].ApiUrl = strutil.RemoveWhiteSpace(api.ApiUrl, true)
			for j, successText := range api.SuccessText {
				newConfig.RegisterApis[i].SuccessText[j] = strutil.Trim(successText)
			}
			for k, failText := range api.FailText {
				newConfig.RegisterApis[i].FailText[k] = strutil.Trim(failText)
			}
			if newConfig.RegisterApis[i].ConcurrencyLimit <= 0 {
				newConfig.RegisterApis[i].ConcurrencyLimit = 1
			}
		}
	}

	if len(newConfig.WhoisApis) > 0 {
		for i, api := range newConfig.WhoisApis {
			newConfig.WhoisApis[i].ApiName = strutil.Trim(api.ApiName)
			newConfig.WhoisApis[i].ApiUrl = strutil.RemoveWhiteSpace(api.ApiUrl, true)
			for j, freeText := range api.FreeText {
				newConfig.WhoisApis[i].FreeText[j] = strutil.Trim(freeText)
			}
			for k, takenText := range api.TakenText {
				newConfig.WhoisApis[i].TakenText[k] = strutil.Trim(takenText)
			}
			if newConfig.WhoisApis[i].ConcurrencyLimit <= 0 {
				newConfig.WhoisApis[i].ConcurrencyLimit = 1
			}
		}
	}

	// Write the new configuration to the file specified by the configFile variable.
	// If the file does not exist, it will be created.
	// If the file cannot be written, the program will exit with code 1.
	// The configuration is also printed to the debug log at startup.
	err := WriteConfig(newConfig)
	if err != nil {
		log.Error("Error writing config file: ", err)
		return err
	}

	// Update the configuration variable.
	// The configuration is not reloaded from the file until the program is restarted.
	config = newConfig

	return nil
}

func WriteConfig(newConfig Config) error {
	configTemplate := `
# Setting the log level, available values are: Error, Warn, Info, Debug, Off
LogLevel: {{ .LogLevel }}

# Setting authentication information
AuthUsername: {{ .AuthUsername }}
AuthPassword: {{ .AuthPassword }}
AuthExpireDays: {{ .AuthExpireDays }}
JwtSecretKey: {{ .JwtSecretKey }}

# ------ Common settings ------
## Setting whois parameters
WhoisTimeout: {{ .WhoisTimeout }}

## Setting DNS parameters
DnsTimeout: {{ .DnsTimeout }}

## Setting retry parameters
RetryOnTimeout: {{ .RetryOnTimeout }}
RetryInterval: {{ .RetryInterval }}
RetryMax: {{ .RetryMax }}

## The TLDs forced to go through proxy
GlobalProxyTlds:
{{- range .GlobalProxyTlds }}
    - {{.}}
{{- end}}

## The TLDs forced to go through proxy in mixed query
MixedProxyTlds:
{{- range .MixedProxyTlds }}
    - {{.}}
{{- end}}

## The TLDs forced to go through DNS check in mixed query
MixedDnsTlds:
{{- range .MixedDnsTlds }}
    - {{.}}
{{- end}}

## Setting proxy information
SocketProxyHost: {{ .SocketProxyHost }}
SocketProxyPort: {{ .SocketProxyPort }}
SocketProxyAuth: {{ .SocketProxyAuth }}
SocketProxyUser: {{ .SocketProxyUser }}
SocketProxyPassword: {{ .SocketProxyPassword }}

# ------ Bulk check settings ------
BulkCheckConcurrencyLimit: {{ .BulkCheckConcurrencyLimit }}

# ------ Web check settings ------
WebCheckConcurrencyLimit: {{ .WebCheckConcurrencyLimit }}
WebCheckDomainLimit: {{ .WebCheckDomainLimit }}

# ------ Typo check settings ------
TypoDefaultCcTlds:
{{- range .TypoDefaultCcTlds }}
    - Tld: {{.Tld}}
      IsSelected: {{.IsSelected}}
{{- end}}

TypoCustomizedReplaces:
{{- range .TypoCustomizedReplaces }}
    - {{.}}
{{- end}}

# ------ Register APIs ------
RegisterApis:
{{- range .RegisterApis }}
    - ApiName: {{.ApiName}}
      ApiUrl: {{.ApiUrl}}
      SuccessText: 
{{- range .SuccessText }}
          - {{.}}
{{- end}}
      FailText: 
{{- range .FailText }}
          - {{.}}
{{- end}}
      ConcurrencyLimit: {{.ConcurrencyLimit}}
{{- end}}

# ------ Whois APIs ------
WhoisApis:
{{- range .WhoisApis }}
    - ApiName: {{.ApiName}}
      ApiUrl: {{.ApiUrl}}
      FreeText: 
{{- range .FreeText }}
          - {{.}}
{{- end}}
      TakenText: 
{{- range .TakenText }}
          - {{.}}
{{- end}}
      ConcurrencyLimit: {{.ConcurrencyLimit}}
{{- end}}
`

	// Create a new template for the configuration file.
	// The template is parsed from the configTemplate string.
	// The template is used to generate the new configuration content.
	configBody, err := template.New("config").Parse(configTemplate)
	if err != nil {
		// Error parsing the config template.
		log.Error("Error parsing config template: ", err)
		return err
	}

	// Create a new buffer to store the new configuration content.
	// The buffer is used to write the new configuration content to the file.
	var newConfigContent bytes.Buffer

	// Execute the template with the new configuration and write the result to the buffer.
	// The new configuration content is generated using the template and the newConfig variable.
	// The result is written to the newConfigContent buffer.
	err = configBody.Execute(&newConfigContent, newConfig)
	if err != nil {
		// Error executing the config template.
		log.Error("Error executing config template: ", err)
		return err
	}

	// Print the new configuration content to the debug log.
	// The new configuration content is printed to the debug log for debugging purposes.
	log.Debugf("New config content: %s", newConfigContent.String())

	// Write the new configuration content to the file specified by the configFile variable.
	// The file is created if it does not exist.
	// The new configuration content is written to the file in the specified order.
	// The file is closed after the write operation is complete.
	return os.WriteFile(configFile, newConfigContent.Bytes(), 0644)
}

func trimTlds(tlds []string) []string {
	// Trim leading and trailing dots from a slice of TLDs.
	// The function takes a slice of strings as input and returns a new slice of strings.
	// The function iterates over the input slice and trims leading and trailing dots from each string.
	// The trimmed strings are appended to a new slice which is returned.
	//
	// Example: trimTlds([]string{".com.", "net."}) returns []string{"com", "net"}
	//
	// The function is used to trim leading and trailing dots from the TLDs configuration.

	var trimmedTlds []string
	for _, tld := range tlds {
		tldStr := strutil.Trim(tld, ".")
		if tldStr == "" {
			continue
		}
		trimmedTlds = append(trimmedTlds, tldStr)
	}
	return trimmedTlds
}
