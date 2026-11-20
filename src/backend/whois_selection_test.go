package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWhoisSelectionDoesNotOfferProxy(t *testing.T) {
	path := filepath.Join(backendDir(t), "..", "frontend", "src", "components", "modules", "WhoisSelection.vue")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `val="whoisQueryWithProxy"`) {
		t.Error("WhoisSelection.vue still contains val=\"whoisQueryWithProxy\"")
	}
	for _, radio := range regexp.MustCompile(`<q-radio\b[^>]*>`).FindAllString(string(body), -1) {
		if strings.Contains(radio, `label="Whois + Proxy"`) {
			t.Error("WhoisSelection.vue still offers a q-radio labelled Whois + Proxy")
		}
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

func TestDeployFrontendDoesNotOfferWhoisProxy(t *testing.T) {
	root := filepath.Join(backendDir(t), "..", "..", "deploy", "frontend")
	jsFiles := 0
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
			t.Errorf("%s still contains the compiled Whois + Proxy radio option", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if jsFiles == 0 {
		t.Fatal("deploy/frontend contains no built JavaScript to check")
	}
}
