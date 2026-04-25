package repository

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// lobeHubOIDCSigningKeyProvider provides a single in-memory RSA key pair for LobeHub OIDC token signing.
type lobeHubOIDCSigningKeyProvider struct {
	mu  sync.Once
	key *rsa.PrivateKey
	kid string
	err error
}

func NewLobeHubOIDCSigningKeyProvider() service.LobeHubOIDCSigningKeyProvider {
	return &lobeHubOIDCSigningKeyProvider{}
}

func (p *lobeHubOIDCSigningKeyProvider) GetSigningKey(_ context.Context) (*rsa.PrivateKey, string, error) {
	p.mu.Do(func() {
		p.key, p.err = rsa.GenerateKey(rand.Reader, 2048)
		p.kid = "lobehub-oidc-1"
	})
	return p.key, p.kid, p.err
}
