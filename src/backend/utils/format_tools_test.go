package utils

import (
	"context"
	"testing"

	"typonamer/constant"
	"typonamer/lookup/lookuperror"
)

func TestDomainHumanStatus(t *testing.T) {
	for _, tc := range []struct {
		status []string
		want   string
	}{
		{nil, constant.DomainStatusUnknown},
		{[]string{"active"}, constant.DomainStatusActive},
		{[]string{"client hold"}, constant.DomainStatusHold},
		{[]string{"serverHold", "clientTransferProhibited"}, constant.DomainStatusHold},
		{[]string{"registrarHold"}, constant.DomainStatusExpired},
		{[]string{"redemption period", "serverHold"}, constant.DomainStatusRedemptionPeriod},
		{[]string{"pendingDelete", "redemptionPeriod"}, constant.DomainStatusPendingDelete},
	} {
		if got := GetDomainHumanStatus(tc.status); got != tc.want {
			t.Errorf("%v = %s, want %s", tc.status, got, tc.want)
		}
	}
}

func TestTimeoutMessagesNameTheirSource(t *testing.T) {
	if GetDomainHumanError(lookuperror.ErrorWhoisTimeout) != "Whois查询超时" ||
		GetDomainHumanError(lookuperror.ErrorDnsTimeout) != "DNS查询超时" ||
		GetDomainHumanError(context.DeadlineExceeded) != "查询超时" {
		t.Fatal("timeout messages do not match the lookup that timed out")
	}
}
