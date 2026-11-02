// Synthetic checked-evidence capacity fixture. Reserved .test names and a fake
// provider make it impossible to confuse this report with a real network scan.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/xuri/excelize/v2"
	"typonamer/workflow"
)

type fakeEvidence struct{ bytes int }

func (p fakeEvidence) Lookup(_ context.Context, d string) workflow.Result {
	raw := make([]byte, 0, p.bytes+32)
	for i := 0; len(raw) < p.bytes; i++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d", d, i)))
		raw = append(raw, sum[:]...)
	}
	return workflow.Result{Status: "registered", CheckedAt: time.Now().UTC(), Provider: "SYNTHETIC-NO-NETWORK", Evidence: base64.StdEncoding.EncodeToString(raw[:p.bytes]), Attempts: []workflow.Attempt{{Provider: "SYNTHETIC-NO-NETWORK", HTTPStatus: 200}}}
}
func check(e error) {
	if e != nil {
		panic(e)
	}
}
func main() {
	output := flag.String("output", "validation/review/checked-evidence", "output directory")
	rows := flag.Int("rows", 10000, "synthetic rows")
	size := flag.Int("evidence-bytes", 8192, "incompressible raw evidence bytes per row before base64")
	flag.Parse()
	if *rows < 1 || *rows > 20000 || *size < 1 || *size > 16000 {
		panic("fixture limits: rows1..20000, evidence1..16000")
	}
	check(os.MkdirAll(*output, 0700))
	f := excelize.NewFile()
	sw, e := f.NewStreamWriter("Sheet1")
	check(e)
	check(sw.SetRow("A1", []interface{}{"Index", "Domain"}))
	for i := 0; i < *rows; i++ {
		check(sw.SetRow(fmt.Sprintf("A%d", i+2), []interface{}{i, fmt.Sprintf("evidence%05d.test", i)}))
	}
	check(sw.Flush())
	input, e := f.WriteToBuffer()
	check(e)
	f.Close()
	store, e := workflow.Open(filepath.Join(*output, "fixture.db"))
	check(e)
	defer store.Close()
	j, e := store.Import(context.Background(), input, workflow.ImportOptions{Column: "B", Header: true, Source: "SYNTHETIC checked-evidence capacity fixture; not real domains or scan results"})
	check(e)
	j, e = store.Generate(context.Background(), j.ID, workflow.GenerateOptions{Rules: []string{"tld"}, TLDs: []string{"test"}, IncludeSeeds: true, MaxCandidates: *rows})
	check(e)
	svc, e := workflow.NewService(store, fakeEvidence{*size}, 10)
	check(e)
	defer svc.Close()
	_, e = svc.Action(j.ID, "start")
	check(e)
	started := time.Now()
	for {
		j, e = store.Get(j.ID)
		check(e)
		check(svc.Health())
		if j.State == "completed" {
			break
		}
		if time.Since(started) > 5*time.Minute {
			panic("synthetic fixture timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var peak atomic.Uint64
	done := make(chan struct{})
	runtime.GC()
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				for old := peak.Load(); m.HeapAlloc > old; old = peak.Load() {
					if peak.CompareAndSwap(old, m.HeapAlloc) {
						break
					}
				}
			}
		}
	}()
	outPath := filepath.Join(*output, "synthetic-checked-evidence.xlsx")
	out, e := os.Create(outPath)
	check(e)
	exportStarted := time.Now()
	check(store.Export(j.ID, "known", out))
	check(out.Close())
	close(done)
	info, e := os.Stat(outPath)
	check(e)
	x, e := excelize.OpenFile(outPath, excelize.Options{UnzipXMLSizeLimit: 8 << 20})
	check(e)
	it, e := x.Rows("Results")
	check(e)
	n := 0
	for it.Next() {
		n++
	}
	check(it.Error())
	it.Close()
	x.Close()
	if n != *rows+1 {
		panic("synthetic XLSX row reconciliation failed")
	}
	report := map[string]interface{}{"synthetic": true, "networkRequests": 0, "domains": *rows, "rawEvidenceBytesPerDomain": *size, "storedEvidenceCharactersPerDomain": base64.StdEncoding.EncodedLen(*size), "checked": j.Completed, "exportBytes": info.Size(), "exportAndRoundtripSeconds": time.Since(exportStarted).Seconds(), "peakExportGoHeapBytesSampled": peak.Load(), "totalFakeScanSeconds": time.Since(started).Seconds(), "note": "All .test records and statuses are synthetic; this tests large checked-evidence export only."}
	data, e := json.MarshalIndent(report, "", "  ")
	check(e)
	check(os.WriteFile(filepath.Join(*output, "report.json"), data, 0600))
	fmt.Println(string(data))
}
