package api

import (
	"strings"
	"testing"
	"typonamer/config"
)

func TestLegacyAuthRejectsUnsetOrShortSecrets(t *testing.T) {
	for _, cfg := range []config.Config{{}, {AuthUsername: "admin", AuthPassword: "short", JwtSecretKey: strings.Repeat("x", 32)}, {AuthUsername: "admin", AuthPassword: strings.Repeat("x", 12), JwtSecretKey: "short"}} {
		if authConfigured(cfg) {
			t.Fatal("insecure legacy auth accepted")
		}
	}
	if !authConfigured(config.Config{AuthUsername: "admin", AuthPassword: strings.Repeat("x", 12), JwtSecretKey: strings.Repeat("y", 32)}) {
		t.Fatal("configured legacy auth rejected")
	}
}
