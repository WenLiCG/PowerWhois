package whoislib

import (
	"errors"
	"testing"
	"typonamer/lookup/lookuperror"
)

func TestRegistryReservedAndDates(t *testing.T) {
	if _, err := ParseWhoisResponse("Reserved by QDR", "example.qa", WhoisMatchers["qa"]); !errors.Is(err, lookuperror.ErrorReservedDomain) {
		t.Fatalf("reserved response = %v", err)
	}
	parsed, err := ParseWhoisResponse("Sponsoring Registrar: Fixture\nRegistration Time: 2020-05-08 12:00:00\n", "example.cn", WhoisMatchers["cn"])
	if err != nil || parsed.CreationDate != "2020-05-08 12:00:00" ||
		dateOrdinal.ReplaceAllString("21st August 2020", "$1") != "21 August 2020" {
		t.Fatalf("date = %q, err = %v", parsed.CreationDate, err)
	}
}
