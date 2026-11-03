package typo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"typonamer/utils"
)

// The workbook is extracted as data only; formula cells are null and never executed.
func TestAttachmentTypoReference(t *testing.T) {
	path := os.Getenv("POWERWHOIS_TYPO_REFERENCE")
	if path == "" {
		t.Skip("set POWERWHOIS_TYPO_REFERENCE for workbook comparison")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var rows [][]any
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	kinds := map[string]string{"漏点1": "www", "缺字": "skipLetter", "自定义中间": "doubleLetter", "互换1": "reverseLetter", "互换2": "reverseLetter", "错插": "insertedLetter", "横插": "wrongHorizontalKey", "竖插": "wrongVerticalKey"}
	cache := map[string]Result{}
	checked, formulas, invalid := 0, 0, 0
	misses := []map[string]any{}
	for i, row := range rows {
		domain := row[0].(string)
		label := row[2].(string)
		kind, ok := kinds[label]
		if !ok {
			t.Fatal("unknown reference label", label)
		}
		if row[1] == nil {
			formulas++
			continue
		}
		target, e := utils.TrimAndGetMainDomain(row[1].(string))
		if e != nil {
			invalid++
			continue
		}
		generated, ok := cache[domain]
		if !ok {
			generated, e = Generate(Request{Domain: domain, Types: []string{"www", "skipLetter", "doubleLetter", "reverseLetter", "insertedLetter", "wrongHorizontalKey", "wrongVerticalKey"}})
			if e != nil {
				t.Fatal(e)
			}
			cache[domain] = generated
		}
		found := false
		for _, v := range generated.Groups[kind] {
			if v == target {
				found = true
				break
			}
		}
		checked++
		if !found {
			misses = append(misses, map[string]any{"row": i + 1, "domain": domain, "expected": target, "label": label, "generator": kind})
		}
	}
	output := os.Getenv("POWERWHOIS_ATTACHMENT_EVIDENCE")
	if output != "" {
		b, _ := json.MarshalIndent(map[string]any{"sourceRows": len(rows), "checked": checked, "formulaCellsSkipped": formulas, "invalidReferenceCandidates": invalid, "baseDomains": len(cache), "missing": misses, "generated": cache}, "", "  ")
		os.WriteFile(filepath.Join(output, "typo-comparison.json"), b, 0600)
	}
	t.Logf("rows=%d checked=%d formulas=%d invalid=%d bases=%d missing=%d", len(rows), checked, formulas, invalid, len(cache), len(misses))
	if len(misses) > 0 {
		t.Fatalf("missing reference candidates: %v", misses[:min(10, len(misses))])
	}
}
