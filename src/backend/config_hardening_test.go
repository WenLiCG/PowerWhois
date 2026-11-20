package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPW6ExampleSocketProxyPasswordPlaceholder(t *testing.T) {
	root := filepath.Join(backendDir(t), "..", "..")
	for _, relative := range []string{"src/backend/config.yaml", "deploy/backend/config.yaml"} {
		t.Run(relative, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(root, relative))
			if err != nil {
				t.Fatal(err)
			}
			var example struct {
				SocketProxyPassword string `yaml:"SocketProxyPassword"`
			}
			if err := yaml.Unmarshal(body, &example); err != nil {
				t.Fatal(err)
			}
			const placeholder = "__PW_EXAMPLE_SOCKET_PROXY_PASSWORD__"
			if example.SocketProxyPassword != placeholder {
				t.Errorf("SocketProxyPassword=%q, want %q", example.SocketProxyPassword, placeholder)
			}
			if example.SocketProxyPassword == "123456" || example.SocketProxyPassword == "1234567890" {
				t.Error("SocketProxyPassword still uses a plaintext sample password")
			}
		})
	}
}

func TestPW6WebsocketSourceOmitsTokenLog(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(backendDir(t), "api", "websocket.go"))
	if err != nil {
		t.Fatal(err)
	}
	forbidden := "Admin auth " + "token is"
	if strings.Contains(string(body), forbidden) {
		t.Error("websocket source still contains forbidden admin token log phrase")
	}
}
