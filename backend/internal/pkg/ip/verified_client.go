package ip

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const (
	VerifiedClientIPSourceXFF    = "xff"
	VerifiedClientIPSourceDirect = "direct"

	defaultVerifiedClientIPMaxHops = 8
	minVerifiedClientIPMaxHops     = 2
	maxVerifiedClientIPMaxHops     = 16
)

// VerifiedClientIP is a client address whose source passed the dedicated
// connectivity-probe trust policy. Source is intentionally server-only.
type VerifiedClientIP struct {
	IP     netip.Addr
	Source string
	OK     bool
}

type VerifiedClientIPConfig struct {
	TrustedProxiesConfigured bool
	TrustedProxies           []string
	DeniedCIDRs              []string
	AllowDirect              bool
	MaxHops                  int
}

type VerifiedClientIPResolver struct {
	trustedProxies           []netip.Prefix
	deniedCIDRs              []netip.Prefix
	trustedProxiesConfigured bool
	allowDirect              bool
	maxHops                  int
}

func NewVerifiedClientIPResolver(cfg VerifiedClientIPConfig) (*VerifiedClientIPResolver, error) {
	maxHops := cfg.MaxHops
	if maxHops == 0 {
		maxHops = defaultVerifiedClientIPMaxHops
	}
	if maxHops < minVerifiedClientIPMaxHops || maxHops > maxVerifiedClientIPMaxHops {
		return nil, errors.New("verified client IP max hops must be between 2 and 16")
	}

	trustedProxies, err := parseVerifiedClientIPPrefixes(cfg.TrustedProxies)
	if err != nil {
		return nil, err
	}
	deniedCIDRs, err := parseVerifiedClientIPPrefixes(cfg.DeniedCIDRs)
	if err != nil {
		return nil, err
	}

	return &VerifiedClientIPResolver{
		trustedProxies:           trustedProxies,
		deniedCIDRs:              deniedCIDRs,
		trustedProxiesConfigured: cfg.TrustedProxiesConfigured,
		allowDirect:              cfg.AllowDirect,
		maxHops:                  maxHops,
	}, nil
}

// CanExposeClientIP reports whether the deployment supplied every safety
// boundary required before a verified address may be returned to a browser.
func (r *VerifiedClientIPResolver) CanExposeClientIP() bool {
	if r == nil || !r.trustedProxiesConfigured || len(r.deniedCIDRs) == 0 {
		return false
	}
	return len(r.trustedProxies) > 0 || r.allowDirect
}

func (r *VerifiedClientIPResolver) Resolve(req *http.Request) VerifiedClientIP {
	if r == nil || req == nil {
		return VerifiedClientIP{}
	}
	peer, ok := parseVerifiedRemoteAddr(req.RemoteAddr)
	if !ok {
		return VerifiedClientIP{}
	}

	if !r.matches(peer, r.trustedProxies) {
		if !r.allowDirect || !isVerifiedPublicAddr(peer) || r.matches(peer, r.deniedCIDRs) {
			return VerifiedClientIP{}
		}
		return VerifiedClientIP{IP: peer, Source: VerifiedClientIPSourceDirect, OK: true}
	}

	values := req.Header.Values("X-Forwarded-For")
	if len(values) != 1 {
		return VerifiedClientIP{}
	}
	tokens := strings.Split(values[0], ",")
	if len(tokens)+1 > r.maxHops {
		return VerifiedClientIP{}
	}

	chain := make([]netip.Addr, 0, len(tokens)+1)
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			return VerifiedClientIP{}
		}
		addr, err := netip.ParseAddr(token)
		if err != nil {
			return VerifiedClientIP{}
		}
		chain = append(chain, addr.Unmap())
	}
	chain = append(chain, peer)

	for i := len(chain) - 1; i >= 0; i-- {
		candidate := chain[i]
		if r.matches(candidate, r.trustedProxies) {
			continue
		}
		if !isVerifiedPublicAddr(candidate) || r.matches(candidate, r.deniedCIDRs) {
			return VerifiedClientIP{}
		}
		return VerifiedClientIP{IP: candidate, Source: VerifiedClientIPSourceXFF, OK: true}
	}
	return VerifiedClientIP{}
}

func parseVerifiedRemoteAddr(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func parseVerifiedClientIPPrefixes(values []string) ([]netip.Prefix, error) {
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("verified client IP CIDR must not be empty")
		}
		if addr, err := netip.ParseAddr(value); err == nil {
			if addr.Zone() != "" {
				return nil, errors.New("verified client IP CIDR must not contain an IPv6 zone")
			}
			addr = addr.Unmap()
			result = append(result, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, errors.New("invalid verified client IP CIDR")
		}
		if prefix.Addr().Zone() != "" {
			return nil, errors.New("verified client IP CIDR must not contain an IPv6 zone")
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}

func (r *VerifiedClientIPResolver) matches(addr netip.Addr, prefixes []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func isVerifiedPublicAddr(addr netip.Addr) bool {
	return IsPublicInternetAddr(addr)
}
