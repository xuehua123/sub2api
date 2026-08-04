package ip

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifiedClientIPResolverTrustedChain(t *testing.T) {
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  8,
	})
	require.NoError(t, err)
	require.True(t, resolver.CanExposeClientIP())

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.8.8, 203.0.113.20")

	verified := resolver.Resolve(req)
	require.True(t, verified.OK)
	require.Equal(t, "8.8.8.8", verified.IP.String())
	require.Equal(t, VerifiedClientIPSourceXFF, verified.Source)
}

func TestVerifiedClientIPResolverRejectsUntrustedForwardingHeaders(t *testing.T) {
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  8,
	})
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.1.1.1:443"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")

	require.False(t, resolver.Resolve(req).OK)
}

func TestVerifiedClientIPResolverDirectModeIgnoresForwardingHeaders(t *testing.T) {
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		AllowDirect:              true,
		MaxHops:                  8,
	})
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.1.1.1:443"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")

	verified := resolver.Resolve(req)
	require.True(t, verified.OK)
	require.Equal(t, "1.1.1.1", verified.IP.String())
	require.Equal(t, VerifiedClientIPSourceDirect, verified.Source)
}

func TestVerifiedClientIPResolverRejectsAmbiguousOrUnsafeChains(t *testing.T) {
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  3,
	})
	require.NoError(t, err)

	tests := []struct {
		name   string
		values []string
	}{
		{name: "missing header"},
		{name: "repeated header", values: []string{"8.8.8.8", "1.1.1.1"}},
		{name: "empty token", values: []string{"8.8.8.8, , 203.0.113.20"}},
		{name: "invalid token", values: []string{"not-an-ip, 203.0.113.20"}},
		{name: "too many hops", values: []string{"8.8.8.8, 1.1.1.1, 203.0.113.20"}},
		{name: "private candidate", values: []string{"10.0.0.8, 203.0.113.20"}},
		{name: "denied node candidate", values: []string{"9.9.9.9, 203.0.113.20"}},
		{name: "all trusted", values: []string{"203.0.113.20"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "203.0.113.10:443"
			for _, value := range tt.values {
				req.Header.Add("X-Forwarded-For", value)
			}
			require.False(t, resolver.Resolve(req).OK)
		})
	}
}

func TestVerifiedClientIPResolverUnmapsIPv4MappedIPv6(t *testing.T) {
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  8,
	})
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "::ffff:8.8.8.8")

	verified := resolver.Resolve(req)
	require.True(t, verified.OK)
	require.Equal(t, "8.8.8.8", verified.IP.String())
}

func TestVerifiedClientIPResolverRejectsSpecialUseAndScopedIPv6(t *testing.T) {
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  8,
	})
	require.NoError(t, err)

	for _, value := range []string{
		"64:ff9b::a00:1",
		"2002:0808:0808::1",
		"3fff::1",
		"5f00::1",
		"2606:4700:4700::1111%eth0",
	} {
		t.Run(value, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "203.0.113.10:443"
			req.Header.Set("X-Forwarded-For", value)
			require.False(t, resolver.Resolve(req).OK)
		})
	}
}

func TestVerifiedClientIPResolverRejectsScopedPrefixConfiguration(t *testing.T) {
	_, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"2606:4700:4700::1111%eth0"},
		MaxHops:                  8,
	})
	require.Error(t, err)
}

func TestVerifiedClientIPResolverRequiresExplicitSafetyBoundary(t *testing.T) {
	// No trusted proxies configured: exit IP must never be exposed.
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: false,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  8,
	})
	require.NoError(t, err)
	require.False(t, resolver.CanExposeClientIP())

	// Denied CIDRs missing: fail closed.
	resolver, err = NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{},
		MaxHops:                  8,
	})
	require.NoError(t, err)
	require.False(t, resolver.CanExposeClientIP())

	// Neither trusted proxies nor allowDirect: fail closed.
	resolver, err = NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		AllowDirect:              false,
		MaxHops:                  8,
	})
	require.NoError(t, err)
	require.False(t, resolver.CanExposeClientIP())
}

func TestVerifiedClientIPResolverCloudflareChain(t *testing.T) {
	// Cloudflare edge CIDR is trusted; the real client is behind it.
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"173.245.48.0/20"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  8,
	})
	require.NoError(t, err)
	require.True(t, resolver.CanExposeClientIP())

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "173.245.49.5:443"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")

	verified := resolver.Resolve(req)
	require.True(t, verified.OK)
	require.Equal(t, "8.8.8.8", verified.IP.String())
}

func TestVerifiedClientIPResolverWireGuardRelayChain(t *testing.T) {
	// A WireGuard relay is a trusted hop; the real client is further upstream.
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"198.51.100.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  8,
	})
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.7:51820"
	req.Header.Set("X-Forwarded-For", "8.8.8.8, 198.51.100.10")

	verified := resolver.Resolve(req)
	require.True(t, verified.OK)
	require.Equal(t, "8.8.8.8", verified.IP.String())
}

func TestVerifiedClientIPResolverIgnoresForgedForwardingHeaders(t *testing.T) {
	// A direct (non-proxy) peer must not be able to forge any forwarding header
	// when allowDirect is disabled.
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  8,
	})
	require.NoError(t, err)

	for _, header := range []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP"} {
		t.Run(header, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "1.1.1.1:443"
			req.Header.Set(header, "8.8.8.8")
			require.False(t, resolver.Resolve(req).OK)
		})
	}
}

func TestVerifiedClientIPResolverMaxHopsBoundary(t *testing.T) {
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  3,
	})
	require.NoError(t, err)

	// Exactly maxHops addresses (2 XFF + peer) is allowed.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.8.8, 203.0.113.20")
	require.True(t, resolver.Resolve(req).OK)

	// One more XFF entry pushes the chain past maxHops.
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 8.8.8.8, 203.0.113.20")
	require.False(t, resolver.Resolve(req).OK)
}

func TestVerifiedClientIPResolverDirectDeniedCIDR(t *testing.T) {
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"203.0.113.0/24"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		AllowDirect:              true,
		MaxHops:                  8,
	})
	require.NoError(t, err)

	// Direct peer in the denied CIDR must be rejected even with allowDirect.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "9.9.9.9:443"
	require.False(t, resolver.Resolve(req).OK)
}

func TestVerifiedClientIPResolverIPv6ThroughTrustedProxy(t *testing.T) {
	resolver, err := NewVerifiedClientIPResolver(VerifiedClientIPConfig{
		TrustedProxiesConfigured: true,
		TrustedProxies:           []string{"2001:db8::/32"},
		DeniedCIDRs:              []string{"9.9.9.0/24"},
		MaxHops:                  8,
	})
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[2001:db8::10]:443"
	req.Header.Set("X-Forwarded-For", "2606:4700:4700::1111")

	verified := resolver.Resolve(req)
	require.True(t, verified.OK)
	require.Equal(t, "2606:4700:4700::1111", verified.IP.String())
	require.Equal(t, VerifiedClientIPSourceXFF, verified.Source)
}
