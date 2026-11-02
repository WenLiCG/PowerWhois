// Package workflow owns durable query jobs shared by the HTTP API, CLI and Web UI.
// It deliberately has no dependency on the legacy registration or Redis packages.
package workflow

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

type Seed struct {
	Row       int    `json:"row"`
	Sheet     string `json:"sheet"`
	Raw       string `json:"raw"`
	Domain    string `json:"domain,omitempty"`
	Error     string `json:"error,omitempty"`
	Duplicate bool   `json:"duplicate"`
}
type Origin struct {
	Seed
	Candidate string `json:"candidate"`
	Rule      string `json:"rule"`
}
type Attempt struct {
	CheckedAt  time.Time `json:"checkedAt"`
	Provider   string    `json:"provider"`
	HTTPStatus int       `json:"httpStatus"`
	Error      string    `json:"error,omitempty"`
	DurationMS int64     `json:"durationMs"`
}
type Result struct {
	Status     string    `json:"status"` // registered, not_found, unknown, error; never a purchase claim
	CheckedAt  time.Time `json:"checkedAt"`
	Provider   string    `json:"provider"`
	HTTPStatus int       `json:"httpStatus"`
	Error      string    `json:"error,omitempty"`
	Evidence   string    `json:"evidence,omitempty"`
	Attempts   []Attempt `json:"attempts"`
}
type Candidate struct {
	Domain string  `json:"domain"`
	Result *Result `json:"result,omitempty"`
}
type GenerateOptions struct {
	Rules         []string `json:"rules"`
	TLDs          []string `json:"tlds"`
	MaxCandidates int      `json:"maxCandidates"`
	IncludeSeeds  bool     `json:"includeSeeds"`
}
type Job struct {
	ID              string          `json:"id"`
	State           string          `json:"state"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	Source          string          `json:"source"`
	InputRows       int             `json:"inputRows"`
	ValidRows       int             `json:"validRows"`
	UniqueSeeds     int             `json:"uniqueSeeds"`
	InvalidRows     int             `json:"invalidRows"`
	RawCandidates   int             `json:"rawCandidates"`
	ValidCandidates int             `json:"validCandidates"`
	Candidates      int             `json:"candidates"`
	Origins         int             `json:"origins"`
	Completed       int             `json:"completed"`
	Counts          map[string]int  `json:"counts"`
	Truncated       bool            `json:"truncated"`
	Partial         bool            `json:"partial"`
	Error           string          `json:"error,omitempty"`
	Options         GenerateOptions `json:"options"`
	SeedCursor      int             `json:"seedCursor"`
	RuleCursor      int             `json:"ruleCursor"`
}

var labelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Normalize only applies DNS-equivalent case/IDNA normalization. It never removes
// punctuation, hyphens, internal whitespace, URL prefixes or subdomain labels.
func Normalize(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("empty domain")
	}
	if strings.ContainsAny(s, "/\\:@=+?# \t\r\n") {
		return "", fmt.Errorf("expected a domain, not a URL, formula or expression")
	}
	d, err := idna.Lookup.ToASCII(strings.ToLower(s))
	if err != nil {
		return "", fmt.Errorf("invalid IDNA domain: %w", err)
	}
	if len(d) > 253 || !strings.Contains(d, ".") {
		return "", fmt.Errorf("invalid domain length or suffix")
	}
	for _, label := range strings.Split(d, ".") {
		if !labelPattern.MatchString(label) {
			return "", fmt.Errorf("invalid DNS label")
		}
	}
	tld := d[strings.LastIndex(d, ".")+1:]
	if len(tld) < 2 || strings.Trim(tld, "0123456789") == "" {
		return "", fmt.Errorf("invalid TLD")
	}
	if _, err = publicRoot(d); err != nil {
		return "", fmt.Errorf("public suffix without domain")
	}
	return d, nil
}

// The PSL also lists privately operated namespaces such as blogspot.com.
// Only its ICANN section defines registry boundaries for registration queries.
func publicRoot(domain string) (string, error) {
	parts := strings.Split(domain, ".")
	suffix, icann := publicsuffix.PublicSuffix(domain)
	if !icann {
		suffix = parts[len(parts)-1]
		for i := 1; i < len(parts); i++ {
			candidate, known := publicsuffix.PublicSuffix(strings.Join(parts[i:], "."))
			if known {
				suffix = candidate
				break
			}
		}
	}
	n := strings.Count(suffix, ".") + 2
	if len(parts) < n {
		return "", fmt.Errorf("public suffix without domain")
	}
	return strings.Join(parts[len(parts)-n:], "."), nil
}

func NormalizeSeed(raw string) (string, error) {
	d, err := Normalize(raw)
	if err != nil {
		return "", err
	}
	root, err := publicRoot(d)
	if err != nil || root != d {
		return "", fmt.Errorf("seed must be a registrable domain; subdomain identity is not changed")
	}
	return d, nil
}

var Rules = []string{"omission", "repetition", "transposition", "insertion", "horizontal", "vertical", "www", "tld"}
var horizontal = map[byte]string{'q': "w", 'w': "qe", 'e': "wr", 'r': "et", 't': "ry", 'y': "tu", 'u': "yi", 'i': "uo", 'o': "ip", 'p': "o", 'a': "s", 's': "ad", 'd': "sf", 'f': "dg", 'g': "fh", 'h': "gj", 'j': "hk", 'k': "jl", 'l': "k", 'z': "x", 'x': "zc", 'c': "xv", 'v': "cb", 'b': "vn", 'n': "bm", 'm': "n"}
var vertical = map[byte]string{'q': "1a", 'w': "2s", 'e': "3d", 'r': "4f", 't': "5g", 'y': "6h", 'u': "7j", 'i': "8k", 'o': "9l", 'p': "0", 'a': "qz", 's': "wx", 'd': "ec", 'f': "rv", 'g': "tb", 'h': "yn", 'j': "um", 'k': "i", 'l': "o", 'z': "a", 'x': "s", 'c': "d", 'v': "f", 'b': "g", 'n': "h", 'm': "j"}

func (o *GenerateOptions) defaults() error {
	if len(o.Rules) == 0 {
		o.Rules = append([]string(nil), Rules...)
	}
	if len(o.TLDs) == 0 {
		o.TLDs = []string{"com", "net", "org", "co"}
	}
	if o.MaxCandidates == 0 {
		o.MaxCandidates = 500000
	}
	if o.MaxCandidates < 1 || o.MaxCandidates > 2000000 {
		return fmt.Errorf("maxCandidates must be 1..2000000")
	}
	seen := map[string]bool{}
	for _, r := range o.Rules {
		found := false
		for _, allowed := range Rules {
			if r == allowed {
				found = true
			}
		}
		if !found || seen[r] {
			return fmt.Errorf("unknown or duplicate rule %q", r)
		}
		seen[r] = true
	}
	for _, tld := range o.TLDs {
		if d, err := NormalizeSeed("example." + tld); err != nil || d != "example."+tld {
			return fmt.Errorf("invalid TLD %q", tld)
		}
	}
	return nil
}

// One rule's output is bounded by the DNS label length, independent of job size.
func mutations(domain, rule string, tlds []string) []string {
	root, err := publicRoot(domain)
	if err != nil || root != domain {
		return nil
	}
	parts := strings.SplitN(domain, ".", 2)
	name, s := parts[0], parts[1]
	var out []string
	add := func(n string) { out = append(out, n+"."+s) }
	switch rule {
	case "seed":
		out = append(out, domain)
	case "www":
		add("www" + name)
	case "tld":
		for _, tld := range tlds {
			out = append(out, name+"."+tld)
		}
	case "insertion":
		for i := 0; i <= len(name); i++ {
			for _, ch := range "abcdefghijklmnopqrstuvwxyz0123456789" {
				add(name[:i] + string(ch) + name[i:])
			}
		}
	default:
		for i := 0; i < len(name); i++ {
			switch rule {
			case "omission":
				add(name[:i] + name[i+1:])
			case "repetition":
				add(name[:i] + string(name[i]) + name[i:])
			case "transposition":
				if i+1 < len(name) {
					add(name[:i] + string(name[i+1]) + string(name[i]) + name[i+2:])
				}
			case "horizontal", "vertical":
				keys := horizontal[name[i]]
				if rule == "vertical" {
					keys = vertical[name[i]]
				}
				for _, ch := range keys {
					add(name[:i] + string(ch) + name[i+1:])
				}
			}
		}
	}
	return out
}
