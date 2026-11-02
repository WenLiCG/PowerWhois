// Verify bounded API pages and, optionally, the exported XLSX independently.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/xuri/excelize/v2"
	"io"
	"net/http"
	"net/url"
	"os"
	"typonamer/workflow"
)

func main() {
	if len(os.Args) != 4 && len(os.Args) != 5 {
		panic("usage: verify-api base-url job-id expected-domain-sha256 [export.xlsx]")
	}
	base := os.Args[1] + "/api/v1/jobs/" + os.Args[2]
	cursor := ""
	count := 0
	hash := sha256.New()
	for {
		resp, e := http.Get(base + "/items?limit=1000&after=" + url.QueryEscape(cursor))
		if e != nil {
			panic(e)
		}
		if resp.StatusCode != 200 {
			panic(resp.Status)
		}
		var page struct {
			Items []workflow.Candidate `json:"items"`
			Next  string               `json:"next"`
		}
		e = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if e != nil {
			panic(e)
		}
		for _, c := range page.Items {
			if normalized, e := workflow.NormalizeSeed(c.Domain); e != nil || normalized != c.Domain {
				panic("invalid registry candidate")
			}
			_, _ = io.WriteString(hash, c.Domain+"\n")
			count++
		}
		if page.Next == "" {
			break
		}
		cursor = page.Next
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != os.Args[3] {
		panic(fmt.Sprintf("domain digest mismatch: %s", got))
	}
	result := map[string]interface{}{"count": count, "uniqueDomainSHA256": got, "matchesRecoveredDirectServiceJob": true}
	if len(os.Args) == 5 {
		response, e := http.Get(base)
		if e != nil {
			panic(e)
		}
		var job workflow.Job
		e = json.NewDecoder(response.Body).Decode(&job)
		response.Body.Close()
		if e != nil {
			panic(e)
		}
		x, e := excelize.OpenFile(os.Args[4], excelize.Options{UnzipXMLSizeLimit: 8 << 20})
		if e != nil {
			panic(e)
		}
		defer x.Close()
		counts := map[string]int{}
		for _, sheet := range []string{"Results", "Sources", "InvalidInputs"} {
			rows, e := x.Rows(sheet)
			if e != nil {
				panic(e)
			}
			n := 0
			for rows.Next() {
				n++
			}
			if e = rows.Error(); e != nil {
				panic(e)
			}
			rows.Close()
			counts[sheet] = n - 1
		}
		if counts["Results"] != count || counts["Sources"] != job.Origins || counts["InvalidInputs"] != job.InvalidRows {
			panic("CLI workbook reconciliation failed")
		}
		result["exportSheetDataRows"] = counts
	}
	if e := json.NewEncoder(os.Stdout).Encode(result); e != nil {
		panic(e)
	}
}
