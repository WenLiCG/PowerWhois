package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWhoisSelectionOffersProxyAfterWhois(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(backendDir(t), "..", "frontend", "src", "components", "modules", "WhoisSelection.vue"))
	if err != nil {
		t.Fatal(err)
	}
	radios := regexp.MustCompile(`<q-radio\b[^>]*>`).FindAllString(string(body), -1)
	if len(radios) < 2 || !strings.Contains(radios[0], `val="whoisQuery"`) || !strings.Contains(radios[1], `val="whoisQueryWithProxy"`) || !strings.Contains(radios[1], `label="Whois + Proxy"`) {
		t.Error("WhoisSelection.vue must offer Whois + Proxy immediately after Whois")
	}
}

func TestCheckPagesDefaultToWhoisQuery(t *testing.T) {
	declaration := regexp.MustCompile(`\bconst\s+queryType\s*=\s*ref\(\s*"whoisQuery"\s*\)`)
	for _, name := range []string{"WebCheck.vue", "TypoCheck.vue", "BulkCheck.vue"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(backendDir(t), "..", "frontend", "src", "components", name)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !declaration.Match(body) {
				t.Error("default queryType must remain the literal \"whoisQuery\"")
			}
		})
	}
}

func TestDeployFrontendOffersWhoisProxy(t *testing.T) {
	root := filepath.Join(backendDir(t), "..", "..", "deploy", "frontend")
	jsFiles := 0
	found := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".js" {
			return nil
		}
		jsFiles++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), `val:"whoisQueryWithProxy",label:"Whois + Proxy"`) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("deploy/frontend must contain the compiled Whois + Proxy radio option")
	}
	if jsFiles == 0 {
		t.Fatal("deploy/frontend contains no built JavaScript to check")
	}
}
