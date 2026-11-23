package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAttachmentWebInputs(t *testing.T) {
	path := os.Getenv("POWERWHOIS_WEB_CASES")
	if path == "" {
		t.Skip("set POWERWHOIS_WEB_CASES for spreadsheet input tests")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Name    string   `json:"name"`
		Domains []string `json:"domains"`
	}
	if e = json.Unmarshal(raw, &cases); e != nil {
		t.Fatal(e)
	}
	report := []map[string]any{}
	for _, c := range cases {
		valid := []string{}
		rejected := []map[string]any{}
		seen := map[string]bool{}
		for i, d := range c.Domains {
			n, e := TrimAndGetMainDomain(d)
			if e != nil {
				rejected = append(rejected, map[string]any{"index": i, "input": d, "error": e.Error()})
				continue
			}
			if !seen[n] {
				seen[n] = true
				valid = append(valid, n)
			}
		}
		report = append(report, map[string]any{"name": c.Name, "raw": c.Domains, "rawCount": len(c.Domains), "validUnique": valid, "rejected": rejected})
		t.Logf("%s: input=%d validUnique=%d rejected=%d", c.Name, len(c.Domains), len(valid), len(rejected))
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	if e = os.WriteFile(filepath.Join(os.Getenv("POWERWHOIS_ATTACHMENT_EVIDENCE"), "web-cases.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
}
