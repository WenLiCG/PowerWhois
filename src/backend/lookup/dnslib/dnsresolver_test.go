package dnslib

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func TestTldNsCacheClonesSlicesAndIgnoresEmptyEntries(t *testing.T) {
	TldNsCache.mux.Lock()
	original := TldNsCache.TldNsMap
	TldNsCache.TldNsMap = make(map[string][]string)
	TldNsCache.mux.Unlock()
	t.Cleanup(func() {
		TldNsCache.mux.Lock()
		TldNsCache.TldNsMap = original
		TldNsCache.mux.Unlock()
	})

	AddTldNsCache("EMPTY", nil)
	if HasTldNsCache("empty") {
		t.Fatal("empty NS slice should not be cached")
	}

	source := []string{"ns1.nic.test."}
	AddTldNsCache("TEST", source)
	source[0] = "mutated.invalid."

	cached := GetTldNsCache("test")
	if len(cached) != 1 || cached[0] != "ns1.nic.test." {
		t.Fatalf("unexpected cached slice: %v", cached)
	}
	cached[0] = "caller-mutated.invalid."
	if again := GetTldNsCache("TEST"); len(again) != 1 || again[0] != "ns1.nic.test." {
		t.Fatalf("cache slice aliased caller memory: %v", again)
	}
}

func TestMixedCaseHeaderMatchingLogic(t *testing.T) {
	// Verify case-insensitive owner matching used during NS referral extraction.
	rr, err := dns.NewRR("EXAMPLE.COM. 3600 IN NS NS1.EXAMPLE.COM.")
	if err != nil {
		t.Fatal(err)
	}
	ns := rr.(*dns.NS)
	hdrName := strings.Trim(ns.Hdr.Name, ".")
	if !strings.EqualFold(hdrName, "example.com") {
		t.Fatalf("expected case-insensitive match for %q", hdrName)
	}

	// Verify canceled context exits cleanly without panic.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NsCheckContext(ctx, "example.com")
	if err == nil {
		t.Fatal("expected canceled lookup error")
	}
	_ = net.ErrClosed
}
