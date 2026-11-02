// Offline verification and provenance attachment for the tiny final-corpus live
// sample. This program never sends network requests or starts scan jobs.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/xuri/excelize/v2"
	"typonamer/workflow"
)

func main() {
	if len(os.Args) != 7 {
		panic("usage: verify-final-sample sample.json status.json results.json export.xlsx deliverable.xlsx report.json")
	}
	read := func(path string, target any) {
		b, e := os.ReadFile(path)
		must(e)
		must(json.Unmarshal(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}), target))
	}
	var sample struct {
		Bias      string       `json:"bias"`
		Corpus    string       `json:"candidateCorpusSHA256"`
		SourceJob workflow.Job `json:"sourceJob"`
		Selected  []struct {
			workflow.Origin
			Score  string `json:"scoreSHA256"`
			Member bool   `json:"memberOfFinalCandidates"`
		} `json:"selected"`
	}
	var job workflow.Job
	var page struct {
		Items []workflow.Candidate `json:"items"`
		Next  string               `json:"next"`
	}
	read(os.Args[1], &sample)
	read(os.Args[2], &job)
	read(os.Args[3], &page)
	if len(sample.Selected) != 20 || len(page.Items) != 20 || page.Next != "" || job.Completed != 20 || job.Candidates != 20 || job.Partial || job.Truncated || job.State != "completed" {
		panic("incomplete or unexpected sample")
	}
	items := map[string]*workflow.Result{}
	counts := map[string]int{}
	attempts := []workflow.Attempt{}
	for _, c := range page.Items {
		if c.Result == nil || items[c.Domain] != nil || len(c.Result.Attempts) == 0 {
			panic("missing result/attempt or duplicate domain")
		}
		items[c.Domain] = c.Result
		counts[c.Result.Status]++
		attempts = append(attempts, c.Result.Attempts...)
	}
	seeds, rules := map[string]bool{}, map[string]int{}
	for _, s := range sample.Selected {
		if !s.Member || items[s.Candidate] == nil || seeds[s.Domain] {
			panic("sample membership or distinct-seed mismatch")
		}
		seeds[s.Domain] = true
		rules[s.Rule]++
	}
	if rules["omission"] != 7 || rules["repetition"] != 7 || rules["transposition"] != 6 {
		panic("stratum mismatch")
	}
	sort.Slice(attempts, func(i, j int) bool { return attempts[i].CheckedAt.Before(attempts[j].CheckedAt) })
	minGap := time.Hour
	for i := 1; i < len(attempts); i++ {
		gap := attempts[i].CheckedAt.Sub(attempts[i-1].CheckedAt)
		if gap < minGap {
			minGap = gap
		}
	}
	if minGap < 2*time.Second {
		panic("provider dispatch spacing below 2 seconds")
	}
	f, e := excelize.OpenFile(os.Args[4])
	must(e)
	defer f.Close()
	rows, e := f.GetRows("Results")
	must(e)
	if len(rows) != 21 {
		panic("workbook result count mismatch")
	}
	for _, row := range rows[1:] {
		if len(row) < 2 || items[row[0]] == nil || items[row[0]].Status != row[1] {
			panic("workbook result mismatch")
		}
	}
	_, e = f.NewSheet("SelectionProvenance")
	must(e)
	head := []string{"Candidate", "Original seed", "Original sheet", "Original row", "Original typo rule", "Selection SHA256", "Final corpus SHA256", "Source job", "Corpus membership", "Status", "HTTPStatus", "CheckedAt UTC"}
	for col, value := range head {
		cell, e := excelize.CoordinatesToCellName(col+1, 1)
		must(e)
		must(f.SetCellStr("SelectionProvenance", cell, value))
	}
	for i, s := range sample.Selected {
		r := items[s.Candidate]
		values := []string{s.Candidate, s.Domain, s.Sheet, strconv.Itoa(s.Row), s.Rule, s.Score, sample.Corpus, sample.SourceJob.ID, "true", r.Status, strconv.Itoa(r.HTTPStatus), r.CheckedAt.Format(time.RFC3339Nano)}
		for col, value := range values {
			cell, e := excelize.CoordinatesToCellName(col+1, i+2)
			must(e)
			must(f.SetCellStr("SelectionProvenance", cell, value))
		}
	}
	must(f.SetColWidth("SelectionProvenance", "A", "L", 28))
	must(f.SetPanes("SelectionProvenance", &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2"}))
	must(f.SetCellStr("SelectionProvenance", "A23", "Sampling limitation"))
	must(f.SetCellStr("SelectionProvenance", "B23", sample.Bias))
	if _, e = os.Stat(os.Args[5]); !os.IsNotExist(e) {
		panic("deliverable already exists or cannot be checked")
	}
	must(f.SaveAs(os.Args[5]))
	b, e := os.ReadFile(os.Args[5])
	must(e)
	sum := sha256.Sum256(b)
	report := map[string]any{"sampleJob": job.ID, "sourceJob": sample.SourceJob.ID, "candidateCorpusSHA256": sample.Corpus, "sampleMembersOfFinalCorpus": 20, "distinctSeeds": len(seeds), "ruleCounts": rules, "resultCounts": counts, "completed": job.Completed, "attempts": len(attempts), "minimumAttemptStartIntervalMs": float64(minGap) / float64(time.Millisecond), "startedAtUTC": attempts[0].CheckedAt, "lastAttemptUTC": attempts[len(attempts)-1].CheckedAt, "completedAtUTC": job.UpdatedAt, "networkRequestsByVerifier": 0, "bias": sample.Bias, "workbookResultsComparedWithAPI": 20, "workbookProvenanceRows": 20, "deliverable": os.Args[5], "deliverableBytes": len(b), "deliverableSHA256": hex.EncodeToString(sum[:]), "notFoundMeaning": "Registry RDAP found no record; does not guarantee purchasability."}
	out, e := json.MarshalIndent(report, "", "  ")
	must(e)
	must(os.WriteFile(os.Args[6], append(out, '\n'), 0600))
	fmt.Println(string(out))
}

func must(e error) {
	if e != nil {
		panic(e)
	}
}
