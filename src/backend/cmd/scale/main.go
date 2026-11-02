// Local-only real-data capacity/recovery validation. It never performs DNS,
// WHOIS or RDAP calls and never invokes scripts supplied by the data source.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xuri/excelize/v2"

	"typonamer/workflow"
)

func check(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func hashFile(path string) string {
	f, e := os.Open(path)
	check(e)
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	check(e)
	return hex.EncodeToString(h.Sum(nil))
}
func save(path string, value interface{}) {
	data, e := json.MarshalIndent(value, "", "  ")
	check(e)
	check(os.WriteFile(path, data, 0600))
}
func main() {
	source := flag.String("source", "", "one-domain-per-line input CSV")
	output := flag.String("output", "validation/real-10k", "output directory")
	resume := flag.Bool("resume", false, "resume interrupted generation")
	interrupt := flag.Int("interrupt-at", 0, "exit process 75 after at least this many durable candidates")
	flag.Parse()
	check(os.MkdirAll(*output, 0700))
	started := time.Now()
	var maxHeap atomic.Uint64
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				for old := maxHeap.Load(); m.HeapAlloc > old; old = maxHeap.Load() {
					if maxHeap.CompareAndSwap(old, m.HeapAlloc) {
						break
					}
				}
			}
		}
	}()
	defer close(done)
	opts := workflow.GenerateOptions{Rules: []string{"omission", "repetition", "transposition"}, TLDs: []string{"com", "net", "org", "co"}, MaxCandidates: 500000}
	store, e := workflow.Open(filepath.Join(*output, "jobs.db"))
	check(e)
	var j workflow.Job
	sourceSHA := ""
	if *source != "" {
		sourceSHA = hashFile(*source)
	}
	if !*resume {
		if *source == "" {
			check(fmt.Errorf("--source is required"))
		}
		input, e := os.Open(*source)
		check(e)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		f := excelize.NewFile()
		writer, e := f.NewStreamWriter("Sheet1")
		check(e)
		check(writer.SetRow("A1", []interface{}{"Rank", "Domain"}))
		seen := map[string]bool{}
		accepted, rejected, duplicates, lines := 0, 0, 0, 0
		for scanner.Scan() {
			lines++
			raw := strings.TrimSpace(scanner.Text())
			d, e := workflow.NormalizeSeed(raw)

			if e != nil {
				rejected++
				continue
			}
			if seen[d] {
				duplicates++
				continue
			}
			seen[d] = true
			accepted++
			check(writer.SetRow("A"+strconv.Itoa(accepted+1), []interface{}{accepted, d}))
			if accepted == 10000 {
				break
			}
		}
		check(scanner.Err())
		input.Close()
		if accepted != 10000 {
			check(fmt.Errorf("only %d valid unique registrable seeds; rejected=%d duplicates=%d; use the pinned 100k source instead", accepted, rejected, duplicates))
		}
		check(writer.Flush())
		xlsx := filepath.Join(*output, "seeds-10000.xlsx")
		check(f.SaveAs(xlsx))
		f.Close()
		in, e := os.Open(xlsx)
		check(e)
		j, e = store.Import(context.Background(), in, workflow.ImportOptions{Sheet: "Sheet1", Column: "B", Header: true, Source: fmt.Sprintf("WebsiteLaunches/top-100-million-domains; %s; SHA256=%s; first 10000 valid unique registrable domains in source order", filepath.Base(*source), sourceSHA)})
		in.Close()
		check(e)
		save(filepath.Join(*output, "job-id.json"), j.ID)
		save(filepath.Join(*output, "sampling.json"), map[string]interface{}{"source": *source, "sourceSHA256": sourceSHA, "linesRead": lines, "accepted": accepted, "rejected": rejected, "duplicates": duplicates, "xlsxSHA256": hashFile(xlsx)})
	} else {
		b, e := os.ReadFile(filepath.Join(*output, "job-id.json"))
		check(e)
		var id string
		check(json.Unmarshal(b, &id))
		j, e = store.Get(id)
		check(e)
		save(filepath.Join(*output, "recovered-checkpoint.json"), j)
	}
	if *interrupt > 0 {
		interruptJobID := j.ID
		go func() {
			for {
				time.Sleep(5 * time.Millisecond)
				current, e := store.Get(interruptJobID)
				if e == nil && current.Candidates >= *interrupt {
					save(filepath.Join(*output, "interrupted-checkpoint.json"), current)
					fmt.Println("abrupt exit after durable checkpoint", current.Candidates)
					os.Exit(75)
				}
			}
		}()
	}
	genStarted := time.Now()
	j, e = store.Generate(context.Background(), j.ID, opts)
	check(e)
	genDuration := time.Since(genStarted)
	check(store.Close())
	store, e = workflow.Open(filepath.Join(*output, "jobs.db"))
	check(e)
	defer store.Close()
	after, e := store.Get(j.ID)
	check(e)
	if after.Candidates != j.Candidates || after.Origins != j.Origins {
		check(fmt.Errorf("restart reconciliation failed"))
	}
	hash := sha256.New()
	count := 0
	cursor := ""
	for {
		items, next, e := store.Page(j.ID, "items", cursor, 1000)
		check(e)
		for _, raw := range items {
			var c workflow.Candidate
			check(json.Unmarshal(raw, &c))
			if normalized, e := workflow.NormalizeSeed(c.Domain); e != nil || normalized != c.Domain {
				check(fmt.Errorf("invalid generated domain"))
			}
			_, _ = io.WriteString(hash, c.Domain+"\n")
			count++
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if count != j.Candidates {
		check(fmt.Errorf("pagination count mismatch"))
	}
	exportStarted := time.Now()
	report := filepath.Join(*output, "real-10k-results.xlsx")
	out, e := os.Create(report)
	check(e)
	check(store.Export(j.ID, "all", out))
	check(out.Close())
	exportDuration := time.Since(exportStarted)
	x, e := excelize.OpenFile(report, excelize.Options{UnzipXMLSizeLimit: 8 << 20})
	check(e)
	sheetCounts := map[string]int{}
	for _, sheet := range []string{"Results", "Sources", "InvalidInputs"} {
		rows, e := x.Rows(sheet)
		check(e)
		n := 0
		for rows.Next() {
			n++
		}
		check(rows.Error())
		rows.Close()
		sheetCounts[sheet] = n - 1
	}
	x.Close()
	if sheetCounts["Results"] != j.Candidates || sheetCounts["Sources"] != j.Origins || sheetCounts["InvalidInputs"] != j.InvalidRows {
		check(fmt.Errorf("export reconciliation failed: %v", sheetCounts))
	}
	info, e := os.Stat(report)
	check(e)
	result := map[string]interface{}{"job": j, "inputRows": j.InputRows, "sourceSHA256": sourceSHA, "uniqueDomainSHA256": hex.EncodeToString(hash.Sum(nil)), "sheetDataRows": sheetCounts, "generationSecondsThisProcess": genDuration.Seconds(), "exportSeconds": exportDuration.Seconds(), "totalSecondsThisProcess": time.Since(started).Seconds(), "peakGoHeapBytesSampled": maxHeap.Load(), "exportBytes": info.Size(), "exportSHA256": hashFile(report), "goVersion": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "logicalCPUs": runtime.NumCPU(), "onlineScanned": 0, "partial": true, "note": "Real public seeds and real generated typos. No online scans or fabricated scan results in this capacity report."}
	save(filepath.Join(*output, "validation-report.json"), result)
	check(json.NewEncoder(os.Stdout).Encode(result))
}
