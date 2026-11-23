package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"sort"
	"strconv"
	"typonamer/constant"
	"typonamer/lookup/lookupinfo"
	"typonamer/utils"
)

// ImportLegacy copies the old singleton batch once. Original keys remain untouched for rollback.
// Legacy Free/DNS cannot prove availability and is deliberately imported as unknown.
func (s *Store) ImportLegacy(ctx context.Context) error {
	marker := s.Prefix + "legacy-imported"
	if n, e := s.Redis.Exists(ctx, marker).Result(); e != nil {
		return e
	} else if n > 0 {
		return nil
	}
	pending, err := s.Redis.HGetAll(ctx, constant.BulkCheckUniqueDomainsRedisKey).Result()
	if err != nil {
		return err
	}
	type oldInput struct {
		Domain string
		Order  int
	}
	inputs := map[string]int{}
	previous := map[string]lookupinfo.QueryResult{}
	for _, raw := range pending {
		var d oldInput
		if err = json.Unmarshal([]byte(raw), &d); err != nil {
			return fmt.Errorf("invalid legacy pending data")
		}
		if _, err = utils.TrimAndGetMainDomain(d.Domain); err != nil {
			return fmt.Errorf("invalid legacy domain; retain backup and repair before migration")
		}
		inputs[d.Domain] = d.Order
	}
	for _, key := range []string{
		constant.BulkCheckTakenResultRedisKey,
		constant.BulkCheckFreeResultRedisKey,
		constant.BulkCheckErrorResultRedisKey,
		constant.BulkCheckErrorResultRedisKey + ":kept",
	} {
		for offset := int64(0); ; offset += 1000 {
			rows, e := s.Redis.LRange(ctx, key, offset, offset+999).Result()
			if e != nil {
				return e
			}
			for _, raw := range rows {
				var r lookupinfo.QueryResult
				if e = json.Unmarshal([]byte(raw), &r); e != nil {
					return e
				}
				inputs[r.Domain] = r.Order
				r.Source = "legacy-import"
				r.QueriedAt = ""
				switch r.RegisterStatus {
				case "Taken":
					r.Status = "registered"
				case "Free", "NotFound":
					r.Status = "not_found"
					r.RegisterStatus = "NotFound"
					if r.LookupType == "dns" {
						r.Status = "unknown"
						r.RegisterStatus = "Unknown"
						r.QueryError = "legacy DNS absence does not establish registration status"
					}
				case "Unknown", "Reserved":
					r.Status = "unknown"
				default:
					r.Status = "error"
				}
				previous[r.Domain] = r
			}
			if len(rows) < 1000 {
				break
			}
		}
	}
	if len(inputs) == 0 {
		return nil
	}
	names := make([]string, 0, len(inputs))
	for d := range inputs {
		names = append(names, d)
	}
	sort.Slice(names, func(i, j int) bool {
		if inputs[names[i]] == inputs[names[j]] {
			return names[i] < names[j]
		}
		return inputs[names[i]] < inputs[names[j]]
	})
	mode, e := s.Redis.Get(ctx, constant.BulkCheckQueryTypeRedisKey).Result()
	if e != nil && e != redis.Nil {
		return e
	}
	if mode == "" {
		mode = "whoisQuery"
	}
	t, _, err := s.Create(ctx, Request{Domains: names, Mode: mode}, "legacy-import-v1")
	if err != nil {
		return err
	}
	t.State = "paused"
	if err = s.save(ctx, t); err != nil {
		return err
	}
	for i, d := range names {
		r, ok := previous[d]
		if !ok {
			continue
		}
		r.Order = i
		raw, _ := json.Marshal(r)
		_, err = s.Redis.Eval(ctx, resultScript, []string{s.Prefix + "worker", s.key(t.ID, "results"), s.key(t.ID, "counts")}, s.owner, strconv.Itoa(i), raw, r.Status).Result()
		if err != nil {
			return err
		}
	}
	t, err = s.Get(ctx, t.ID)
	if err != nil {
		return err
	}
	if t.Completed == t.Total {
		t.State = "completed"
	}
	if err = s.save(ctx, t); err != nil {
		return err
	}
	return s.Redis.Eval(ctx, writeGuard+`return redis.call('SET',KEYS[2],ARGV[2])`, []string{s.Prefix + "worker", marker}, s.owner, t.ID).Err()
}
