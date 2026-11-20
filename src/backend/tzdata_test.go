package main

import (
	"os"
	"testing"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/whoislib"
)

// 有效 RED 必须在无法加载 PRC 的环境执行；普通 go test 会提供 GOROOT，
// 其通过不能作为 RED 证据。仅设置无效 ZONEINFO 也不足以阻止 Go 回退。
// 在 src/backend 下构建并由沙箱外验证（不联网）：
// export GOEXPERIMENT=nojsonv2 GOCACHE=/tmp/pw8-go-build GOFLAGS=-ldflags=-checklinkname=0
// export GOPROXY=off GOSUMDB=off
// go test -c -trimpath -o /tmp/pw8-red-main.test .
// env -u GOROOT -u ZONEINFO /tmp/pw8-red-main.test -test.v -test.count=1 -test.run '^TestPW8CNParseMainTimezoneData$'
// 依赖真实 main.init 设置 carbon PRC 默认值，不在测试中嵌入 time/tzdata。
// RED 在缺少 PRC 时因产品 panic 失败；GREEN 嵌入 tzdata 后须保留正确日期。
func TestPW8CNParseMainTimezoneData(t *testing.T) {
	raw, err := os.ReadFile("lookup/whoislib/testdata/baidu.cn.whois")
	if err != nil {
		t.Fatal(err)
	}
	info := pw8Parse(t, string(raw))
	pw8AssertFields(t, info, string(raw))
	if info.CreationDate != "2003-03-17 12:20:05" {
		t.Errorf("CreationDate = %q", info.CreationDate)
	}
	if info.ExpiryDate != "2029-03-17 12:48:36" {
		t.Errorf("ExpiryDate = %q", info.ExpiryDate)
	}
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
