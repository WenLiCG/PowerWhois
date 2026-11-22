package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsRoundTripKeepsPrivateFields(t *testing.T) {
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "local-fixture-admin")
	t.Setenv("POWERWHOIS_JWT_SECRET", "local-fixture-jwt")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("AuthUsername: admin\nSocketProxyPassword: local-fixture-proxy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	next := GetConfig()
	next.WhoisApis = []WhoisApi{{
		ApiName: "Fixture", ApiUrl: "https://api.example.test/?key=local-fixture-key&domain={domain}",
		FreeText: []string{"message: available # yes"}, TakenText: []string{"reserved"}, ConcurrencyLimit: 1,
	}}
	safe, err := UpdateConfig(next)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(safe)
	if err != nil || strings.Contains(string(body), "local-fixture-") {
		t.Fatalf("settings should not include credential values: %v", err)
	}
	var edit Config
	if err := json.Unmarshal(body, &edit); err != nil {
		t.Fatal(err)
	}
	edit.WhoisTimeout = 7
	if _, err := UpdateConfig(edit); err != nil {
		t.Fatal(err)
	}
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	saved := GetConfig()
	if saved.WhoisTimeout != 7 || saved.SocketProxyPassword != "local-fixture-proxy" || saved.WhoisApis[0].ApiUrl != next.WhoisApis[0].ApiUrl ||
		saved.WhoisApis[0].FreeText[0] != next.WhoisApis[0].FreeText[0] {
		t.Fatal("save/load lost configuration")
	}
	if _, err := UpdateConfig(Config{}); err == nil {
		t.Fatal("accepted invalid settings")
	}
	if GetConfig().WhoisTimeout != 7 {
		t.Fatal("invalid settings changed active configuration")
	}
}
