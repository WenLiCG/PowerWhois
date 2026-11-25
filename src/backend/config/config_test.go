package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShippedConfigFiles(t *testing.T) {
	original, originalPath := GetConfig(), configFile
	t.Cleanup(func() { mu.Lock(); config, configFile = original, originalPath; mu.Unlock() })
	t.Setenv("POWERWHOIS_ADMIN_USER", "fixture")
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", strings.Repeat("a", 12))
	t.Setenv("POWERWHOIS_JWT_SECRET", strings.Repeat("b", 32))
	for _, path := range []string{"../config.yaml", "../../../deploy/backend/config.yaml"} {
		t.Run(path, func(t *testing.T) {
			if err := Load(path); err != nil {
				t.Fatal(err)
			}
			c := GetConfig()
			if c.WebCheckConcurrencyLimit != 15 || c.DnsConcurrency != 15 || c.WhoisConcurrency != 15 {
				t.Error("shipped config overrides the documented concurrency defaults")
			}
			if c.SocketProxyHost != "" || len(c.RegisterApis) != 0 || len(c.WhoisApis) != 0 {
				t.Error("fresh installation should have no third-party provider configured")
			}
		})
	}
}

func TestSafeConfigRoundTrip(t *testing.T) {
	original, originalPath := GetConfig(), configFile
	t.Cleanup(func() { mu.Lock(); config, configFile = original, originalPath; mu.Unlock() })
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("logLevel: Error\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POWERWHOIS_ADMIN_USER", "fixture")
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "fixture-password-strong")
	t.Setenv("POWERWHOIS_JWT_SECRET", strings.Repeat("fixture!", 8))
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	c := GetConfig()
	c.SocketProxyPassword = "quote: #\n'你好"
	c.WhoisApis = []WhoisApi{{ApiName: "fixture", ApiUrl: "https://example.com/?domain={domain}&key=a%23b", FreeText: []string{"missing"}, TakenText: []string{"exists"}, ConcurrencyLimit: 1}}
	if err := UpdateConfig(c); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), c.AuthPassword) || strings.Contains(string(raw), c.JwtSecretKey) {
		t.Fatal("environment secret persisted")
	}
	if err := Load(path); err != nil || GetConfig().SocketProxyPassword != c.SocketProxyPassword {
		t.Fatal("YAML round trip", err)
	}
	c.AuthPassword = "different-password"
	if UpdateConfig(c) == nil {
		t.Fatal("environment credential unexpectedly editable")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(raw) {
		t.Fatal("invalid update changed file")
	}
	c = GetConfig()
	c.WhoisApis[0].FreeText = []string{""}
	if Validate(c) == nil {
		t.Fatal("empty matching rule accepted")
	}
	c = GetConfig()
	public := PublicSettings()
	secretsMap, ok := public["secrets"].(map[string]interface{})
	if !ok {
		t.Fatal("missing secrets metadata")
	}
	if userMeta, ok := secretsMap["authUsername"].(map[string]bool); !ok || !userMeta["managed"] {
		t.Fatalf("expected authUsername managed metadata, got %+v", secretsMap["authUsername"])
	}
	encoded, _ := json.Marshal(public)
	for _, secret := range []string{c.AuthPassword, c.JwtSecretKey, c.SocketProxyPassword, c.WhoisApis[0].ApiUrl} {
		if secret != "" && strings.Contains(string(encoded), secret) {
			t.Fatal("public settings expose a secret")
		}
	}
	var incoming Config
	json.Unmarshal(encoded, &incoming)
	if e := UpdateConfig(incoming, map[string]string{}); e != nil {
		t.Fatal("redacted settings could not round trip", e)
	}
	kept := GetConfig()
	if kept.AuthPassword != c.AuthPassword || kept.JwtSecretKey != c.JwtSecretKey || kept.SocketProxyPassword != c.SocketProxyPassword || kept.WhoisApis[0].ApiUrl != c.WhoisApis[0].ApiUrl {
		t.Fatal("keep lost a secret")
	}
	if e := UpdateConfig(incoming, map[string]string{"socketProxyPassword": "clear"}); e != nil || GetConfig().SocketProxyPassword != "" {
		t.Fatal("explicit clear failed", e)
	}
	copy := GetConfig()
	copy.WhoisApis[0].FreeText[0] = "mutated"
	if GetConfig().WhoisApis[0].FreeText[0] == "mutated" {
		t.Fatal("configuration snapshot aliases live data")
	}
}

func TestRoutingSuffixesUseASCIIOnLoadAndSave(t *testing.T) {
	original, originalPath := GetConfig(), configFile
	t.Cleanup(func() { mu.Lock(); config, configFile = original, originalPath; mu.Unlock() })
	t.Setenv("POWERWHOIS_ADMIN_USER", "fixture")
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", strings.Repeat("a", 12))
	t.Setenv("POWERWHOIS_JWT_SECRET", strings.Repeat("b", 32))
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("globalProxyTlds: [.公司.]\nmixedProxyTlds: [XN--55QX5D]\nmixedDnsTlds: [co.uk]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	c := GetConfig()
	if c.GlobalProxyTlds[0] != "xn--55qx5d" || c.MixedProxyTlds[0] != "xn--55qx5d" || c.MixedDnsTlds[0] != "co.uk" {
		t.Fatal("startup rules do not match canonical lookup suffixes")
	}
	c.MixedDnsTlds = []string{"公司"}
	if err := UpdateConfig(c); err != nil || GetConfig().MixedDnsTlds[0] != "xn--55qx5d" {
		t.Fatal("saved DNS routing rule does not match the lookup suffix", err)
	}
}
