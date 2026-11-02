package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	bolt "go.etcd.io/bbolt"
)

type ImportOptions struct {
	Sheet  string
	Column string
	Header bool
	Source string
}

func (s *Store) Import(ctx context.Context, r io.Reader, o ImportOptions) (Job, error) {
	var j Job
	var err error
	j = Job{ID: newID(), State: "importing", CreatedAt: time.Now().UTC(), Source: o.Source, Counts: map[string]int{}, Partial: true}
	err = s.update(func(tx *bolt.Tx) error {
		b, e := tx.Bucket(jobsKey).CreateBucket([]byte(j.ID))
		if e != nil {
			return e
		}
		for _, name := range []string{"seeds", "unique", "items", "origins", "pending", "active"} {
			if _, e = b.CreateBucket([]byte(name)); e != nil {
				return e
			}
		}
		return putJob(b, j)
	})
	if err != nil {
		return j, err
	}
	var batch []Seed

	flush := func() error {
		return s.update(func(tx *bolt.Tx) error {
			b, e := jobBucket(tx, j.ID)
			if e != nil {
				return e
			}
			stored, e := readJob(b)
			if e != nil {
				return e
			}
			unique := b.Bucket([]byte("unique"))
			for _, seed := range batch {
				stored.InputRows++
				if seed.Error != "" {
					stored.InvalidRows++
				} else {
					stored.ValidRows++
					seed.Duplicate = unique.Get([]byte(seed.Domain)) != nil
					if !seed.Duplicate {
						stored.UniqueSeeds++
						if e = unique.Put([]byte(seed.Domain), rowKey(seed.Row)); e != nil {
							return e
						}
					}
				}
				data, e := json.Marshal(seed)
				if e != nil {
					return e
				}
				if e = b.Bucket([]byte("seeds")).Put(rowKey(seed.Row), data); e != nil {
					return e
				}
			}
			j = stored
			return putJob(b, j)
		})
	}
	err = readSeedRows(ctx, r, o, func(seed Seed) error {
		batch = append(batch, seed)
		if len(batch) == 500 {
			if e := flush(); e != nil {
				return e
			}
			batch = nil
		}
		return nil
	})
	if err == nil && len(batch) > 0 {
		err = flush()
	}
	if err != nil {
		cause := err
		saveErr := s.update(func(tx *bolt.Tx) error {
			b, e := jobBucket(tx, j.ID)
			if e != nil {
				return e
			}
			j.State = "error"
			j.Error = cause.Error()
			return putJob(b, j)
		})
		if saveErr != nil {
			return j, fmt.Errorf("import: %v; persist failure: %w", cause, saveErr)
		}
		return j, cause
	}
	err = s.update(func(tx *bolt.Tx) error {
		b, e := jobBucket(tx, j.ID)
		if e != nil {
			return e
		}
		j.State = "draft"
		return putJob(b, j)
	})
	return j, err
}

// Generate batches about 1000 raw candidates per transaction. Candidate records,
// origins, counters and the seed/rule checkpoint commit atomically. A crash can
// replay only the uncommitted bounded batch, without changing deduplication.
func (s *Store) Generate(ctx context.Context, id string, o GenerateOptions) (Job, error) {
	if err := o.defaults(); err != nil {
		return Job{}, err
	}
	var j Job
	// Admission and the state check must share one write transaction with cancel.
	err := s.update(func(tx *bolt.Tx) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		b, e := jobBucket(tx, id)
		if e != nil {
			return e
		}
		j, e = readJob(b)
		if e != nil {
			return e
		}
		if j.State != "draft" && j.State != "generating" && j.State != "ready" {
			return fmt.Errorf("%w: cannot generate %s", ErrConflict, j.State)
		}
		if j.State != "draft" {
			stored, _ := json.Marshal(j.Options)
			requested, _ := json.Marshal(o)
			if string(stored) != string(requested) {
				return fmt.Errorf("%w: generation options are immutable; import a new job to use different options", ErrConflict)
			}
		}
		if j.State == "ready" {
			return nil
		}
		j.Options = o
		j.State = "generating"
		return putJob(b, j)
	})
	if err != nil || j.State == "ready" {
		return j, err
	}
	rules := append([]string(nil), o.Rules...)
	if o.IncludeSeeds {
		rules = append([]string{"seed"}, rules...)
	}
	for j.State == "generating" {
		if err = ctx.Err(); err != nil {
			return j, err
		}
		err = s.update(func(tx *bolt.Tx) error {
			b, e := jobBucket(tx, id)
			if e != nil {
				return e
			}
			items := b.Bucket([]byte("items"))
			origins := b.Bucket([]byte("origins"))
			cursor := b.Bucket([]byte("seeds")).Cursor()
			rawInBatch := 0
			for rawInBatch < 1000 && j.State == "generating" {
				if e = ctx.Err(); e != nil {
					return e
				}
				k, v := cursor.Seek(rowKey(j.SeedCursor))
				if k == nil {
					j.State = "ready"
					break
				}
				var seed Seed
				if e = json.Unmarshal(v, &seed); e != nil {
					return e
				}
				if seed.Error != "" || j.RuleCursor >= len(rules) {
					j.SeedCursor = seed.Row + 1
					j.RuleCursor = 0
					continue
				}
				rule := rules[j.RuleCursor]
				candidates := mutations(seed.Domain, rule, o.TLDs)
				rawInBatch += len(candidates)
				for _, raw := range candidates {
					j.RawCandidates++
					d, e := NormalizeSeed(raw)
					if e != nil || (d == seed.Domain && rule != "seed") {
						continue
					}
					j.ValidCandidates++
					key := []byte(d)
					if items.Get(key) == nil {
						if j.Candidates == o.MaxCandidates {
							j.Truncated = true
							j.State = "ready"
							break
						}
						data, _ := json.Marshal(Candidate{Domain: d})
						if e = items.Put(key, data); e != nil {
							return e
						}
						if e = b.Bucket([]byte("pending")).Put(key, []byte{1}); e != nil {
							return e
						}
						j.Candidates++
					}
					originKey := []byte(d + "\x00" + strconv.Itoa(seed.Row) + "\x00" + rule)
					if origins.Get(originKey) == nil {
						data, _ := json.Marshal(Origin{Seed: seed, Candidate: d, Rule: rule})
						if e = origins.Put(originKey, data); e != nil {
							return e
						}
						j.Origins++
					}
				}
				j.RuleCursor++
			}
			return putJob(b, j)
		})
		if err != nil {
			return j, err
		}
	}
	return s.Get(id)
}

// All text is written as explicit strings, never as formulas. The leading quote
// also protects downstream applications that may convert these cells to CSV.
func safeText(s string) string {
	t := strings.TrimLeft(s, " \t\r\n")
	if t != "" && strings.ContainsAny(t[:1], "=+-@") {
		return "'" + s
	}
	return s
}

func (s *Store) Export(id, denominator string, w io.Writer) error {
	if denominator != "all" && denominator != "known" {
		return fmt.Errorf("denominator must be all or known")
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Summary"); err != nil {
		return err
	}
	for _, name := range []string{"Results", "Sources", "InvalidInputs"} {
		if _, err := f.NewSheet(name); err != nil {
			return err
		}
	}
	streams := map[string]*excelize.StreamWriter{}
	line := map[string]int{}
	for _, name := range []string{"Results", "Sources", "InvalidInputs"} {
		sw, err := f.NewStreamWriter(name)
		if err != nil {
			return err
		}
		streams[name] = sw
		line[name] = 1
		if err = sw.SetColWidth(1, 16, 24); err != nil {
			return err
		}
		if err = sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2"}); err != nil {
			return err
		}
	}
	write := func(sheet string, values ...interface{}) error {
		n := line[sheet]
		if n > 1048576 {
			return fmt.Errorf("%s exceeds Excel row limit; use paginated API for remaining records", sheet)
		}
		for i, v := range values {
			if text, ok := v.(string); ok {
				values[i] = safeText(text)
			}
		}
		err := streams[sheet].SetRow("A"+strconv.Itoa(n), values)
		line[sheet]++
		return err
	}
	if err := write("Results", "Domain", "Status", "CheckedAt UTC", "Provider", "HTTPStatus", "Error", "Attempts JSON", "Evidence", "EvidenceTruncated", "JobID", "Partial"); err != nil {
		return err
	}
	if err := write("Sources", "Domain", "Sheet", "Row", "Raw input", "Seed", "Rule"); err != nil {
		return err
	}
	if err := write("InvalidInputs", "Sheet", "Row", "Raw input", "Error"); err != nil {
		return err
	}
	// A single read transaction gives the workbook a consistent snapshot while
	// workers continue to commit. StreamWriter spills large worksheets to disk.
	err := s.db.View(func(tx *bolt.Tx) error {
		b, e := jobBucket(tx, id)
		if e != nil {
			return e
		}
		j, e := readJob(b)
		if e != nil {
			return e
		}
		divisor := j.Candidates
		if denominator == "known" {
			divisor = j.Counts["registered"] + j.Counts["not_found"]
		}
		rate := interface{}("undefined: denominator is zero")
		if divisor > 0 {
			rate = float64(j.Counts["not_found"]) / float64(divisor)
		}
		values := [][]interface{}{{"PowerWhois query report", j.ID}, {"State", j.State}, {"Partial results", j.Partial}, {"Generated candidate cap reached", j.Truncated}, {"Input nonempty rows", j.InputRows}, {"Valid input rows", j.ValidRows}, {"Unique seeds", j.UniqueSeeds}, {"Invalid inputs", j.InvalidRows}, {"Raw candidates attempted", j.RawCandidates}, {"Legal non-identity candidates", j.ValidCandidates}, {"Unique scan domains", j.Candidates}, {"Source associations", j.Origins}, {"Checked domains", j.Completed}, {"Registered", j.Counts["registered"]}, {"Not found", j.Counts["not_found"]}, {"Unknown", j.Counts["unknown"]}, {"Error", j.Counts["error"]}, {"Ratio denominator option", denominator}, {"Ratio denominator count", divisor}, {"Not-found ratio", rate}, {"Meaning", "not_found does not establish that a domain is purchasable"}, {"Source", j.Source}, {"Exported at UTC", time.Now().UTC().Format(time.RFC3339)}, {"Pending", j.Candidates - j.Completed}}
		for i, row := range values {
			for k, v := range row {
				if text, ok := v.(string); ok {
					row[k] = safeText(text)
				}
			}
			if e = f.SetSheetRow("Summary", "A"+strconv.Itoa(i+1), &row); e != nil {
				return e
			}
		}
		if e = f.SetColWidth("Summary", "A", "A", 36); e != nil {
			return e
		}
		if e = f.SetColWidth("Summary", "B", "B", 85); e != nil {
			return e
		}
		if e = b.Bucket([]byte("items")).ForEach(func(k, v []byte) error {
			var c Candidate
			if e := json.Unmarshal(v, &c); e != nil {
				return e
			}
			r := c.Result
			if r == nil {
				return write("Results", c.Domain, "pending", "", "", 0, "", "", "", false, j.ID, j.Partial)
			}
			a, _ := json.Marshal(r.Attempts)
			evidence := r.Evidence
			truncated := false
			if runes := []rune(evidence); len(runes) > 32000 {
				evidence = string(runes[:32000]) + "\n[truncated; full evidence in results API]"
				truncated = true
			}
			return write("Results", c.Domain, r.Status, r.CheckedAt.Format(time.RFC3339Nano), r.Provider, r.HTTPStatus, r.Error, string(a), evidence, truncated, j.ID, j.Partial)
		}); e != nil {
			return e
		}
		if e = b.Bucket([]byte("origins")).ForEach(func(k, v []byte) error {
			var o Origin
			if e := json.Unmarshal(v, &o); e != nil {
				return e
			}
			domain, _, _ := keyParts(string(k))
			return write("Sources", domain, o.Sheet, o.Row, o.Raw, o.Domain, o.Rule)
		}); e != nil {
			return e
		}
		return b.Bucket([]byte("seeds")).ForEach(func(k, v []byte) error {
			var seed Seed
			if e := json.Unmarshal(v, &seed); e != nil {
				return e
			}
			if seed.Error != "" {
				return write("InvalidInputs", seed.Sheet, seed.Row, seed.Raw, seed.Error)
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	for _, name := range []string{"Results", "Sources", "InvalidInputs"} {
		if err = streams[name].Flush(); err != nil {
			return err
		}
	}
	return writeWorkbookStream(f, w)
}
