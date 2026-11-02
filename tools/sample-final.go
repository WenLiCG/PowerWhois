// Select a reproducible, rule-stratified .com sample from an actual completed
// candidate-generation job. This tool is offline and performs no domain queries.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
	"typonamer/workflow"
)

type selection struct {
	workflow.Origin
	Score  string `json:"scoreSHA256"`
	Member bool   `json:"memberOfFinalCandidates"`
}

func main() {
	if len(os.Args) != 4 {
		panic("usage: sample-final closed-db job-id output-prefix")
	}
	store, e := workflow.Open(os.Args[1])
	if e != nil {
		panic(e)
	}
	defer store.Close()
	job, e := store.Get(os.Args[2])
	if e != nil {
		panic(e)
	}
	quota := map[string]int{"omission": 7, "repetition": 7, "transposition": 6}
	best := map[string]map[string]selection{}
	for rule := range quota {
		best[rule] = map[string]selection{}
	}
	cursor := ""
	for {
		page, next, e := store.Page(job.ID, "origins", cursor, 1000)
		if e != nil {
			panic(e)
		}
		for _, raw := range page {
			var o workflow.Origin
			if e = json.Unmarshal(raw, &o); e != nil {
				panic(e)
			}
			if _, ok := quota[o.Rule]; !ok || !strings.HasSuffix(o.Candidate, ".com") {
				continue
			}
			sum := sha256.Sum256([]byte(fmt.Sprintf("PowerWhoisFinal10kSample/v1\x00%s\x00%s\x00%s\x00%d\x00%s", o.Candidate, o.Domain, o.Rule, o.Row, o.Sheet)))
			s := selection{Origin: o, Score: hex.EncodeToString(sum[:])}
			old, exists := best[o.Rule][o.Domain]
			if !exists || s.Score < old.Score {
				best[o.Rule][o.Domain] = s
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	chosen := []selection{}
	usedSeed, usedDomain := map[string]bool{}, map[string]bool{}
	for _, rule := range []string{"omission", "repetition", "transposition"} {
		pool := []selection{}
		for _, s := range best[rule] {
			pool = append(pool, s)
		}
		sort.Slice(pool, func(i, j int) bool { return pool[i].Score < pool[j].Score })
		n := 0
		for _, s := range pool {
			if usedSeed[s.Domain] || usedDomain[s.Candidate] {
				continue
			}
			chosen = append(chosen, s)
			usedSeed[s.Domain] = true
			usedDomain[s.Candidate] = true
			n++
			if n == quota[rule] {
				break
			}
		}
		if n != quota[rule] {
			panic("insufficient distinct seeds for stratum")
		}
	}
	corpusHash := sha256.New()
	members := map[string]bool{}
	cursor = ""
	for {
		page, next, e := store.Page(job.ID, "items", cursor, 1000)
		if e != nil {
			panic(e)
		}
		for _, raw := range page {
			var c workflow.Candidate
			if e = json.Unmarshal(raw, &c); e != nil {
				panic(e)
			}
			_, _ = corpusHash.Write([]byte(c.Domain + "\n"))
			if usedDomain[c.Domain] {
				members[c.Domain] = true
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(members) != 20 {
		panic("selected domains are not all candidate members")
	}
	for i := range chosen {
		chosen[i].Member = true
	}
	x := excelize.NewFile()
	defer x.Close()
	headers := []interface{}{"Original seed", "Domain to scan", "Original typo rule", "Source sheet", "Source row", "Corpus job", "Score SHA256"}
	if e = x.SetSheetRow("Sheet1", "A1", &headers); e != nil {
		panic(e)
	}
	for i, s := range chosen {
		values := []interface{}{s.Domain, s.Candidate, s.Rule, s.Sheet, s.Row, job.ID, s.Score}
		if e = x.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+2), &values); e != nil {
			panic(e)
		}
	}
	if e = x.SaveAs(os.Args[3] + ".xlsx"); e != nil {
		panic(e)
	}
	report := map[string]interface{}{"sourceJob": job, "selectionAlgorithm": "v1; restrict final candidate origins to .com; minimum SHA256 per seed/rule; sort scores; select7 omission,7 repetition,6 transposition in that order, skipping previously used seed/domain", "distinctSeeds": len(usedSeed), "selected": chosen, "ruleQuotas": quota, "candidateMembershipConfirmed": 20, "candidateCorpusSHA256": hex.EncodeToString(corpusHash.Sum(nil)), "networkRequestsByThisTool": 0, "bias": "Rule-balanced, distinct-seed, .com-only deterministic design; not a uniform all-candidate or all-TLD sample; cannot estimate overall purchasability/registration prevalence."}
	data, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		panic(e)
	}
	if e = os.WriteFile(os.Args[3]+".json", data, 0600); e != nil {
		panic(e)
	}
	fmt.Printf("selected=%d distinctSeeds=%d confirmedMembers=%d\n", len(chosen), len(usedSeed), len(members))
}
