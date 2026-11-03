package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
}
