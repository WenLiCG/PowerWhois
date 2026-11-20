package whoislib_test

import (
	"github.com/dromara/carbon/v2"
	"regexp"
	"testing"
	"typonamer/lookup/whoislib"
)

// Invalid defaults exercise the error path even on hosts with tzdata.
func TestPW8ParseCarbonErrorPaths(t *testing.T) {
	defer pw8SetApplicationDefaults()
	raw := pw8ReadSample(t)
	for _, mode := range []string{"auto", "layout", "field layouts"} {
		t.Run(mode, func(t *testing.T) {
			carbon.SetDefault(carbon.Default{Layout: carbon.DateTimeLayout, Timezone: "PW8/InvalidTimezone", WeekStartsAt: carbon.Monday, Locale: "zh-CN"})
			matcher := whoislib.WhoisMatchers["cn"]
			matcher.DateTimeLayout = ""
			switch mode {
			case "layout":
				matcher.DateTimeLayout = carbon.DateTimeLayout
			case "field layouts":
				matcher.DateTimeLayoutForCreationDate = carbon.DateTimeLayout
				matcher.DateTimeLayoutForExpiryDate = carbon.DateTimeLayout
			}
			info, err := whoislib.ParseWhoisResponse(raw, "baidu.cn", matcher)
			if err != nil {
				t.Fatal(err)
			}
			pw8AssertFields(t, info, raw)
			if info.CreationDate != "" || info.ExpiryDate != "" {
				t.Fatalf("Carbon error must leave dates empty: %q / %q", info.CreationDate, info.ExpiryDate)
			}
		})
	}
}

func TestPW8ParseEntryRecovers(t *testing.T) {
	raw := pw8ReadSample(t)
	matcher := whoislib.WhoisMatchers["cn"]
	// A status matcher without a capture triggers an internal indexing panic.
	matcher.ReDomainStatus = regexp.MustCompile(`Domain Status: .*`)
	info, err := whoislib.ParseWhoisResponse(raw, "baidu.cn", matcher)
	if err == nil {
		t.Fatal("want explicit parse failure")
	}
	if info.DomainName != "baidu.cn" || info.RawResponse != raw || info.Registrar != "互联网域名系统北京市工程研究中心有限公司" {
		t.Fatalf("recovery lost previously parsed fields: %+v", info)
	}
}
