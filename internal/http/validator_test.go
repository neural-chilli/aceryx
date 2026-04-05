package httpfw

import (
	"net/netip"
	"testing"
)

func TestURLValidatorHTTPSInProduction(t *testing.T) {
	v := NewURLValidator(true)
	v.resolveHost = func(_ string) ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil }
	if err := v.Validate("t1", "http://example.com/data"); err == nil {
		t.Fatal("expected HTTPS enforcement error")
	}
}

func TestURLValidatorDevAllowsLocalhostHTTP(t *testing.T) {
	v := NewURLValidator(false)
	v.resolveHost = func(_ string) ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil }
	if err := v.Validate("t1", "http://localhost:8080/data"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestURLValidatorBlocksPrivateRanges(t *testing.T) {
	v := NewURLValidator(true)
	cases := []string{
		"https://127.0.0.1/data",
		"https://10.0.0.5/data",
		"https://172.16.0.9/data",
		"https://192.168.1.2/data",
		"https://169.254.10.1/data",
		"https://0.0.0.1/data",
		"https://[::1]/data",
		"https://[fc00::1]/data",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if err := v.Validate("t1", raw); err == nil {
				t.Fatal("expected private IP block")
			}
		})
	}
}

func TestURLValidatorBlocksResolvedPrivateIP(t *testing.T) {
	v := NewURLValidator(true)
	v.resolveHost = func(_ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("10.0.0.2")}, nil
	}
	if err := v.Validate("t1", "https://public.example.com/data"); err == nil {
		t.Fatal("expected DNS-rebound private IP block")
	}
}

func TestURLValidatorAllowlist(t *testing.T) {
	v := NewURLValidator(true)
	v.resolveHost = func(_ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	v.SetAllowlist("t1", []string{"api.example.com"})

	if err := v.Validate("t1", "https://api.example.com/data"); err != nil {
		t.Fatalf("expected allowlisted domain to pass: %v", err)
	}
	if err := v.Validate("t1", "https://api.evil.com/data"); err == nil {
		t.Fatal("expected non-allowlisted domain to fail")
	}
}

func TestURLValidatorNoAllowlistAllowsPublicDomain(t *testing.T) {
	v := NewURLValidator(true)
	v.resolveHost = func(_ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	if err := v.Validate("t1", "https://api.anything.com/data"); err != nil {
		t.Fatalf("expected public domain to pass without allowlist: %v", err)
	}
}
