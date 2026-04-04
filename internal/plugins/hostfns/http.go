package hostfns

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/plugins"
)

var errPrivateIPBlocked = errors.New("request to private IP range blocked")

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type HTTPHost struct {
	Client         HTTPClient
	AllowDomains   map[string]struct{}
	SystemMax      time.Duration
	MaxBodyBytes   int64
	DialLookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)
}

func NewHTTPHost(client HTTPClient, allowDomains []string, maxTimeout time.Duration) *HTTPHost {
	d := map[string]struct{}{}
	for _, domain := range allowDomains {
		dd := strings.ToLower(strings.TrimSpace(domain))
		if dd == "" {
			continue
		}
		d[dd] = struct{}{}
	}
	if maxTimeout <= 0 {
		maxTimeout = 60 * time.Second
	}
	return &HTTPHost{
		Client:       client,
		AllowDomains: d,
		SystemMax:    maxTimeout,
		MaxBodyBytes: 10 << 20,
	}
}

func (h *HTTPHost) HTTPRequest(method, rawURL string, headers map[string]string, body []byte, timeoutMS int) (plugins.HTTPResponse, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return plugins.HTTPResponse{}, fmt.Errorf("invalid url: %w", err)
	}
	host := strings.ToLower(parsed.Hostname())
	if len(h.AllowDomains) > 0 {
		if _, ok := h.AllowDomains[host]; !ok {
			return plugins.HTTPResponse{}, fmt.Errorf("domain not allowed: %s", host)
		}
	}
	if err := h.blockPrivateHost(host); err != nil {
		return plugins.HTTPResponse{}, err
	}

	timeout := h.SystemMax
	if timeoutMS > 0 {
		requested := time.Duration(timeoutMS) * time.Millisecond
		if requested < timeout {
			timeout = requested
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return plugins.HTTPResponse{}, fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return plugins.HTTPResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	limit := h.MaxBodyBytes
	if limit <= 0 {
		limit = 10 << 20
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return plugins.HTTPResponse{}, fmt.Errorf("read response body: %w", err)
	}
	outHeaders := map[string]string{}
	for k, values := range resp.Header {
		outHeaders[k] = strings.Join(values, ",")
	}
	return plugins.HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    outHeaders,
		Body:       respBody,
	}, nil
}

func (h *HTTPHost) blockPrivateHost(host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return errPrivateIPBlocked
		}
		return nil
	}
	lookup := h.DialLookupFunc
	if lookup == nil {
		lookup = defaultLookup
	}
	addrs, err := lookup(context.Background(), host)
	if err != nil {
		return err
	}
	for _, a := range addrs {
		if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() {
			return errPrivateIPBlocked
		}
	}
	return nil
}

func defaultLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		if addr, ok := netip.AddrFromSlice(ip); ok {
			out = append(out, addr)
		}
	}
	return out, nil
}

func isPrivateIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast()
}
