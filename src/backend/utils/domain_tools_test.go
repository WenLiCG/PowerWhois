package utils

import (
	"encoding/csv"
	"strings"
	"testing"
	"typonamer/lookup/lookupinfo"
)

func TestDomainBoundary(t *testing.T) {
	for in, want := range map[string]string{" EXAMPLE.COM. ": "example.com", "sub.example.co.uk": "example.co.uk", "bücher.de": "xn--bcher-kva.de", "foo.com.com": "com.com", "foo.com.cn.com.cn": "cn.com.cn"} {
		got, e := TrimAndGetMainDomain(in)
		if e != nil || got != want {
			t.Fatalf("%q: %q %v", in, got, e)
		}
	}
	for _, in := range []string{"hello world.com", "https://example.com/path", "example..com", "-example.com", "example-.com", "127.0.0.1", "a.com\nQUIT", "x_foo.com"} {
		if _, e := TrimAndGetMainDomain(in); e == nil {
			t.Fatalf("accepted %q", in)
		}
	}
}

func TestCSVNeutralizesUpstreamFormulas(t *testing.T) {
	data, err := ConvertQueryResultToCSV([]lookupinfo.QueryResult{{Domain: "example.com", Status: "error", Source: "=cmd()", QueryError: "\n=unsafe", NameServer: []string{"+formula"}}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil || len(rows) != 2 {
		t.Fatal(err)
	}
	for i, title := range rows[0] {
		if title == "Source" || title == "Error Info" || title == "Name Server" {
			if !strings.HasPrefix(rows[1][i], "'") {
				t.Fatalf("unescaped %s", title)
			}
		}
	}
}
