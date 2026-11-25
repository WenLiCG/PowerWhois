package typo

import (
	"fmt"
	"sort"
	"strings"
	"typonamer/utils"
)

type Request struct {
	Domain string   `json:"domain"`
	Types  []string `json:"types"`
	TLDs   []string `json:"tlds"`
}
type Result struct {
	Domains []string            `json:"domains"`
	Groups  map[string][]string `json:"groups"`
}

func Generate(r Request) (Result, error) {
	out := Result{Domains: []string{}, Groups: map[string][]string{}}
	domain, err := utils.TrimAndGetMainDomain(r.Domain)
	if err != nil {
		return out, err
	}
	if len(r.Types) > 8 || len(r.TLDs) > 100 {
		return out, fmt.Errorf("too many typo types or suffixes")
	}
	t := Typo{Domain: domain}
	methods := map[string]func() []string{"www": t.TypeWww, "skipLetter": t.TypeSkipLetter, "doubleLetter": t.TypeDoubleLetter, "reverseLetter": t.TypeReverseLetter, "insertedLetter": t.TypeInsertedLetter, "wrongHorizontalKey": t.TypeWrongHorizontalKey, "wrongVerticalKey": t.TypeWrongVerticalKey, "customizedReplace": t.TypeCustomizedReplace}
	for _, kind := range r.Types {
		f, ok := methods[kind]
		if !ok {
			return out, fmt.Errorf("unknown typo type: %s", kind)
		}
		out.Groups[kind] = f()
	}
	if len(r.TLDs) > 0 {
		for _, suffix := range r.TLDs {
			candidate, e := utils.TrimDomain("example." + strings.TrimPrefix(suffix, "."))
			if e != nil {
				return out, fmt.Errorf("invalid suffix")
			}
			main, e := utils.GetDomainSuffixPlusOne(candidate)
			if e != nil || main != candidate {
				return out, fmt.Errorf("invalid suffix")
			}
		}
		out.Groups["wrongTlds"] = t.TypeWrongTlds(r.TLDs)
	}
	seen := map[string]bool{}
	for kind, domains := range out.Groups {
		valid := []string{}
		groupSeen := map[string]bool{}
		for _, d := range domains {
			n, e := utils.TrimAndGetMainDomain(d)
			if e != nil {
				continue
			}
			if !groupSeen[n] {
				valid = append(valid, n)
				groupSeen[n] = true
			}
			if !seen[n] {
				out.Domains = append(out.Domains, n)
				seen[n] = true
			}
		}
		sort.Strings(valid)
		out.Groups[kind] = valid
	}
	sort.Strings(out.Domains)
	return out, nil
}
