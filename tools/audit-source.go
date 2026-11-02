// Independently reconcile the pinned public CSV with the final seed workbook
// and disclose membership/selection of the existing small online sample.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xuri/excelize/v2"
	"typonamer/workflow"
)

func main() {
	if len(os.Args) != 6 {
		panic("usage: audit-source CSV input.xlsx live-results.json DB job-id")
	}
	f, e := os.Open(os.Args[1])
	if e != nil {
		panic(e)
	}
	h := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(f, h))
	seen := map[string]bool{}
	expected := []string{}
	lines, rejected, duplicates, last := 0, 0, 0, 0
	for scanner.Scan() {
		lines++
		if len(expected) == 10000 {
			continue
		}
		d, e := workflow.NormalizeSeed(strings.TrimSpace(scanner.Text()))
		if e != nil {
			rejected++
			continue
		}
		if seen[d] {
			duplicates++
			continue
		}
		seen[d] = true
		expected = append(expected, d)
		last = lines
	}
	if e = scanner.Err(); e != nil {
		panic(e)
	}
	f.Close()
	if len(expected) != 10000 {
		panic("insufficient public seeds")
	}
	x, e := excelize.OpenFile(os.Args[2], excelize.Options{UnzipXMLSizeLimit: 8 << 20})
	if e != nil {
		panic(e)
	}
	rows, e := x.Rows("Sheet1")
	if e != nil {
		panic(e)
	}
	index := 0
	for rows.Next() {
		values, e := rows.Columns()
		if e != nil {
			panic(e)
		}
		if index == 0 {
			index++
			continue
		}
		if len(values) < 2 || index > 10000 || values[1] != expected[index-1] {
			panic(fmt.Sprintf("source/XLSX sequence mismatch at row %d", index+1))
		}
		index++
	}
	if e = rows.Error(); e != nil {
		panic(e)
	}
	rows.Close()
	x.Close()
	if index != 10001 {
		panic("XLSX row count mismatch")
	}
	data, e := os.ReadFile(os.Args[3])
	if e != nil {
		panic(e)
	}
	var live struct {
		Items []workflow.Candidate `json:"items"`
	}
	if e = json.Unmarshal(data, &live); e != nil {
		panic(e)
	}
	membership := map[string]bool{}
	for _, c := range live.Items {
		membership[c.Domain] = false
	}
	store, e := workflow.Open(os.Args[4])
	if e != nil {
		panic(e)
	}
	defer store.Close()
	cursor := ""
	for {
		page, next, e := store.Page(os.Args[5], "items", cursor, 1000)
		if e != nil {
			panic(e)
		}
		for _, raw := range page {
			var c workflow.Candidate
			if e = json.Unmarshal(raw, &c); e != nil {
				panic(e)
			}
			if _, exists := membership[c.Domain]; exists {
				membership[c.Domain] = true
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	matches := 0
	for _, found := range membership {
		if found {
			matches++
		}
	}
	result := map[string]interface{}{"sourceCSVRows": lines, "sourceSHA256": hex.EncodeToString(h.Sum(nil)), "seedCount": len(expected), "sourceRowsUsed": last, "rejectedBeforeCutoff": rejected, "duplicatesBeforeCutoff": duplicates, "seedWorkbookSequenceMatchesPinnedCSV": true, "liveSeedPresentInFinal10000": map[string]bool{"indyweek.com": seen["indyweek.com"], "google.com": seen["google.com"], "itnews.com.au": seen["itnews.com.au"]}, "liveCandidateCount": len(live.Items), "liveCandidatesBelongingToFinalJob": matches, "liveCandidateMembership": membership, "selectionBias": "Existing live20 came from one .com seed in the earlier pre-PSL-fix workbook; they are not an unbiased sample of the final 10000-seed job and cannot estimate its registration proportions."}
	if e = json.NewEncoder(os.Stdout).Encode(result); e != nil {
		panic(e)
	}
}
