package whoislib_test

import (
	"github.com/dromara/carbon/v2"
	"os"
	"strings"
	"testing"
	"time"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/whoislib"
)

// 有效 RED 必须在无法加载 PRC 的环境执行；普通 go test 会提供 GOROOT，
// 其通过不能作为 RED 证据。仅设置无效 ZONEINFO 也不足以阻止 Go 回退。
// 在 src/backend 下构建并由沙箱外验证（不联网）：
// export GOEXPERIMENT=nojsonv2 GOCACHE=/tmp/pw8-go-build GOFLAGS=-ldflags=-checklinkname=0
// export GOPROXY=off GOSUMDB=off
// go test -c -trimpath -o /tmp/pw8-red-whoislib.test ./lookup/whoislib
// env -u GOROOT -u ZONEINFO /tmp/pw8-red-whoislib.test -test.v -test.count=1 -test.run '^TestPW8CNParse'
// 本层不嵌入 tzdata；PRC 缺失时日期应为空，但其他字段应保留。
func TestPW8CNParseApplicationDefaults(t *testing.T) {
	pw8SetApplicationDefaults()
	raw := pw8ReadSample(t)
	info := pw8Parse(t, raw)
	pw8AssertFields(t, info, raw)
	if _, err := time.LoadLocation("PRC"); err != nil {
		if info.CreationDate != "" || info.ExpiryDate != "" {
			t.Errorf("without PRC: CreationDate = %q, ExpiryDate = %q, want empty", info.CreationDate, info.ExpiryDate)
		}
	} else {
		if info.CreationDate != "2003-03-17 12:20:05" {
			t.Errorf("CreationDate = %q", info.CreationDate)
		}
		if info.ExpiryDate != "2029-03-17 12:48:36" {
			t.Errorf("ExpiryDate = %q", info.ExpiryDate)
		}
	}
}

func TestPW8CNParseMalformedDates(t *testing.T) {
	for _, tc := range []struct {
		name, old, replacement string
		creation               bool
	}{
		{"registration", "Registration Time: 2003-03-17 12:20:05", "Registration Time: not-a-real-date", true},
		{"expiration", "Expiration Time: 2029-03-17 12:48:36", "Expiration Time: not-a-real-date", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pw8SetApplicationDefaults()
			sample := pw8ReadSample(t)
			if strings.Count(sample, tc.old) != 1 {
				t.Fatal("fixture must contain exactly one date to replace")
			}
			raw := strings.Replace(sample, tc.old, tc.replacement, 1)
			info := pw8Parse(t, raw)
			pw8AssertFields(t, info, raw)
			broken, intact, wantIntact := info.ExpiryDate, info.CreationDate, "2003-03-17 12:20:05"
			if tc.creation {
				broken, intact, wantIntact = info.CreationDate, info.ExpiryDate, "2029-03-17 12:48:36"
			}
			if broken != "" {
				t.Errorf("malformed date = %q, want empty", broken)
			}
			if _, err := time.LoadLocation("PRC"); err != nil {
				wantIntact = ""
			}
			if intact != wantIntact {
				t.Errorf("intact date = %q, want %q", intact, wantIntact)
			}
		})
	}
}

// Carbon defaults are global: these tests must not run in parallel.
func pw8SetApplicationDefaults() {
	carbon.SetDefault(carbon.Default{Layout: carbon.DateTimeLayout, Timezone: carbon.PRC, WeekStartsAt: carbon.Monday, Locale: "zh-CN"})
}

func pw8ReadSample(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/baidu.cn.whois")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func pw8Parse(t *testing.T, raw string) lookupinfo.DomainInfo {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	info, err := whoislib.ParseWhoisResponse(raw, "baidu.cn", whoislib.WhoisMatchers["cn"])
	if err != nil {
		t.Fatalf("ParseWhoisResponse: %v", err)
	}
	return info
}

func pw8AssertFields(t *testing.T, info lookupinfo.DomainInfo, raw string) {
	t.Helper()
	if info.DomainName != "baidu.cn" {
		t.Errorf("DomainName = %q", info.DomainName)
	}
	if info.RawResponse != raw {
		t.Error("RawResponse does not preserve the complete input")
	}
	if info.Registrar != "互联网域名系统北京市工程研究中心有限公司" {
		t.Errorf("Registrar = %q", info.Registrar)
	}
	for _, field := range []struct {
		name      string
		got, want []string
	}{
		{"NameServer", info.NameServer, []string{"ns1.baidu.cn", "ns2.baidu.cn", "ns3.baidu.cn", "ns4.baidu.cn"}},
		{"DomainStatus", info.DomainStatus, []string{"clientDeleteProhibited", "clientTransferProhibited"}},
	} {
		for _, want := range field.want {
			found := false
			for _, got := range field.got {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s = %v, missing %q", field.name, field.got, want)
			}
		}
	}
}
