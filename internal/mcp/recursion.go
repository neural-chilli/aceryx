package mcp

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

func (m *Manager) CheckSelfInvocation(serverURL string) error {
	u, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}
	targetHost := normalizeHostPort(u.Host)
	if targetHost == "" {
		return fmt.Errorf("invalid server URL host")
	}
	selfHosts := map[string]struct{}{}
	selfPorts := map[string]struct{}{}
	for _, selfURL := range m.selfURLs {
		su, serr := url.Parse(strings.TrimSpace(selfURL))
		if serr == nil {
			if host := normalizeHostPort(su.Host); host != "" {
				selfHosts[host] = struct{}{}
				_, port := splitHostPort(host)
				if port != "" {
					selfPorts[port] = struct{}{}
				}
			}
		}
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		for port := range selfPorts {
			selfHosts[normalizeHostPort(net.JoinHostPort(host, port))] = struct{}{}
		}
	}
	for port := range selfPorts {
		selfHosts[normalizeHostPort(net.JoinHostPort("localhost", port))] = struct{}{}
		selfHosts[normalizeHostPort(net.JoinHostPort("127.0.0.1", port))] = struct{}{}
		selfHosts[normalizeHostPort(net.JoinHostPort("0.0.0.0", port))] = struct{}{}
	}
	hostOnly, port := splitHostPort(targetHost)
	if _, ok := selfHosts[targetHost]; ok {
		return fmt.Errorf("recursive MCP invocation blocked: target is this Aceryx instance")
	}
	if hostOnly != "" && port != "" {
		for self := range selfHosts {
			shost, sport := splitHostPort(self)
			if sport == port && hostsEquivalent(hostOnly, shost) {
				return fmt.Errorf("recursive MCP invocation blocked: target is this Aceryx instance")
			}
		}
	}
	return nil
}

func normalizeHostPort(hostport string) string {
	h, p := splitHostPort(hostport)
	h = strings.Trim(strings.ToLower(strings.TrimSpace(h)), "[]")
	p = strings.TrimSpace(p)
	if h == "" {
		return ""
	}
	if p == "" {
		return h
	}
	return net.JoinHostPort(h, p)
}

func splitHostPort(hostport string) (host, port string) {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return "", ""
	}
	h, p, err := net.SplitHostPort(hostport)
	if err == nil {
		return h, p
	}
	if strings.Count(hostport, ":") == 0 {
		return hostport, ""
	}
	parts := strings.Split(hostport, ":")
	if len(parts) >= 2 {
		return strings.Join(parts[:len(parts)-1], ":"), parts[len(parts)-1]
	}
	return hostport, ""
}

func hostsEquivalent(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(strings.Trim(a, "[]")))
	b = strings.ToLower(strings.TrimSpace(strings.Trim(b, "[]")))
	if a == b {
		return true
	}
	if (a == "localhost" || a == "127.0.0.1" || a == "0.0.0.0") && (b == "localhost" || b == "127.0.0.1" || b == "0.0.0.0") {
		return true
	}
	return false
}
