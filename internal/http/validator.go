package httpfw

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync"
)

var privateRanges = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
}

type URLValidator struct {
	mu               sync.RWMutex
	productionMode   bool
	tenantAllowlists map[string]map[string]struct{}
	resolveHost      func(host string) ([]netip.Addr, error)
}

func NewURLValidator(productionMode bool) *URLValidator {
	return &URLValidator{
		productionMode:   productionMode,
		tenantAllowlists: make(map[string]map[string]struct{}),
		resolveHost:      defaultResolveHost,
	}
}

// Validate checks a URL against all security rules.
func (v *URLValidator) Validate(tenantID, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("invalid URL: missing host")
	}

	host := strings.ToLower(parsed.Hostname())
	scheme := strings.ToLower(parsed.Scheme)

	if scheme != "https" {
		if v.isProductionMode() {
			return fmt.Errorf("HTTPS required in production mode")
		}
		if !isDevHTTPOK(host) && !v.isHostAllowlisted(tenantID, host) {
			return fmt.Errorf("HTTP only allowed for localhost and configured test domains")
		}
	}

	if err := v.validateAllowlist(tenantID, host); err != nil {
		return err
	}

	addrs, err := v.resolve(host)
	if err != nil {
		return fmt.Errorf("resolve host %s: %w", host, err)
	}
	for _, addr := range addrs {
		if isPrivateOrBlockedAddr(addr) {
			return fmt.Errorf("request to private IP range blocked")
		}
	}

	return nil
}

// SetAllowlist configures a tenant domain allowlist.
func (v *URLValidator) SetAllowlist(tenantID string, domains []string) {
	if v == nil {
		return
	}
	set := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		d := strings.ToLower(strings.TrimSpace(domain))
		if d == "" {
			continue
		}
		set[d] = struct{}{}
	}
	v.mu.Lock()
	if len(set) == 0 {
		delete(v.tenantAllowlists, tenantID)
	} else {
		v.tenantAllowlists[tenantID] = set
	}
	v.mu.Unlock()
}

func (v *URLValidator) isProductionMode() bool {
	if v == nil {
		return true
	}
	v.mu.RLock()
	prod := v.productionMode
	v.mu.RUnlock()
	return prod
}

func (v *URLValidator) validateAllowlist(tenantID, host string) error {
	if v == nil {
		return nil
	}
	v.mu.RLock()
	list := v.tenantAllowlists[tenantID]
	v.mu.RUnlock()
	if len(list) == 0 {
		return nil
	}
	if _, ok := list[host]; !ok {
		return fmt.Errorf("domain not in allowlist: %s", host)
	}
	return nil
}

func (v *URLValidator) isHostAllowlisted(tenantID, host string) bool {
	if v == nil {
		return false
	}
	v.mu.RLock()
	list := v.tenantAllowlists[tenantID]
	v.mu.RUnlock()
	_, ok := list[host]
	return ok
}

func (v *URLValidator) resolve(host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	if v == nil || v.resolveHost == nil {
		return defaultResolveHost(host)
	}
	return v.resolveHost(host)
}

func defaultResolveHost(host string) ([]netip.Addr, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		if addr, ok := netip.AddrFromSlice(ip); ok {
			out = append(out, addr.Unmap())
		}
	}
	return out, nil
}

func isPrivateOrBlockedAddr(addr netip.Addr) bool {
	unmapped := addr.Unmap()
	for _, prefix := range privateRanges {
		if prefix.Contains(unmapped) {
			return true
		}
	}
	return false
}

func isDevHTTPOK(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" {
		return true
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		return ip.IsLoopback()
	}
	return false
}
