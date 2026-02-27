package internalapi

import (
	"log/slog"
	"net"
	"net/http"
	"strings"

	"audistro-provider/internal/config"
)

type ipAllowlist struct {
	allowedCIDRs      []*net.IPNet
	trustProxyHeaders bool
	trustedProxyCIDRs []*net.IPNet
}

func NewIPAllowlistMiddleware(cfg config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	allowlist := &ipAllowlist{
		trustProxyHeaders: cfg.TrustProxyHeaders,
	}

	var err error
	allowlist.allowedCIDRs, err = parseCIDRList(cfg.InternalAllowedCIDRs)
	if err != nil {
		if logger != nil {
			logger.Error("invalid internal allowlist CIDRs", slog.String("error", err.Error()))
		}
		allowlist.allowedCIDRs = nil
	}
	allowlist.trustedProxyCIDRs, err = parseCIDRList(cfg.TrustedProxyCIDRs)
	if err != nil {
		if logger != nil {
			logger.Error("invalid trusted proxy CIDRs", slog.String("error", err.Error()))
		}
		allowlist.trustedProxyCIDRs = nil
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := allowlist.clientIP(r)
			if clientIP == nil || !ipInCIDRs(clientIP, allowlist.allowedCIDRs) {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseCIDRList(value string) ([]*net.IPNet, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	out := make([]*net.IPNet, 0, len(parts))
	for _, part := range parts {
		cidr := strings.TrimSpace(part)
		if cidr == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		out = append(out, ipnet)
	}
	return out, nil
}

func (a *ipAllowlist) clientIP(r *http.Request) net.IP {
	remote := parseRemoteIP(r.RemoteAddr)
	if remote == nil {
		return nil
	}

	if !a.trustProxyHeaders || !ipInCIDRs(remote, a.trustedProxyCIDRs) {
		return remote
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remote
	}

	first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	if first == "" {
		return remote
	}
	forwarded := net.ParseIP(first)
	if forwarded == nil {
		return remote
	}
	return forwarded
}

func parseRemoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return net.ParseIP(strings.TrimSpace(remoteAddr))
	}
	return net.ParseIP(strings.TrimSpace(host))
}

func ipInCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	for _, cidr := range cidrs {
		if cidr != nil && cidr.Contains(ip) {
			return true
		}
	}
	return false
}
