package repository

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	lobeHubOIDCWebSessionKeyPrefix        = "lobehub:oidc:web_session:"
	lobeHubOIDCAuthorizationCodeKeyPrefix = "lobehub:oidc:authorization_code:"
	lobeHubOIDCAccessTokenKeyPrefix       = "lobehub:oidc:access_token:"
	lobeHubOIDCResumeTokenKeyPrefix       = "lobehub:oidc:resume_token:"
	lobeHubBootstrapTicketKeyPrefix       = "lobehub:bootstrap:ticket:"
)

type lobeHubOIDCStateStore struct {
	rdb *redis.Client
}

func NewLobeHubOIDCStateStore(rdb *redis.Client) service.LobeHubOIDCStateStore {
	return &lobeHubOIDCStateStore{rdb: rdb}
}

func (s *lobeHubOIDCStateStore) CreateWebSession(ctx context.Context, session *service.LobeHubOIDCWebSession, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("marshal lobehub oidc web session: %w", err)
	}
	sessionID, err := newOpaqueLobeHubOIDCToken(24)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, lobeHubOIDCWebSessionKey(sessionID), payload, ttl).Err(); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *lobeHubOIDCStateStore) GetWebSession(ctx context.Context, sessionID string) (*service.LobeHubOIDCWebSession, error) {
	payload, err := s.rdb.Get(ctx, lobeHubOIDCWebSessionKey(sessionID)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrLobeHubOIDCSessionNotFound
		}
		return nil, err
	}

	var session service.LobeHubOIDCWebSession
	if err := json.Unmarshal([]byte(payload), &session); err != nil {
		return nil, fmt.Errorf("unmarshal lobehub oidc web session: %w", err)
	}
	return &session, nil
}

func (s *lobeHubOIDCStateStore) DeleteWebSession(ctx context.Context, sessionID string) error {
	return s.rdb.Del(ctx, lobeHubOIDCWebSessionKey(sessionID)).Err()
}

func (s *lobeHubOIDCStateStore) CreateAuthorizationCode(ctx context.Context, code *service.LobeHubOIDCAuthorizationCode, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(code)
	if err != nil {
		return "", fmt.Errorf("marshal lobehub oidc authorization code: %w", err)
	}
	codeID, err := newOpaqueLobeHubOIDCToken(32)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, lobeHubOIDCAuthorizationCodeKey(codeID), payload, ttl).Err(); err != nil {
		return "", err
	}
	return codeID, nil
}

func (s *lobeHubOIDCStateStore) ConsumeAuthorizationCode(ctx context.Context, code string) (*service.LobeHubOIDCAuthorizationCode, error) {
	payload, err := s.rdb.GetDel(ctx, lobeHubOIDCAuthorizationCodeKey(code)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrLobeHubOIDCInvalidGrant
		}
		return nil, err
	}

	var authorizationCode service.LobeHubOIDCAuthorizationCode
	if err := json.Unmarshal([]byte(payload), &authorizationCode); err != nil {
		return nil, fmt.Errorf("unmarshal lobehub oidc authorization code: %w", err)
	}
	return &authorizationCode, nil
}

func (s *lobeHubOIDCStateStore) CreateAccessToken(ctx context.Context, token *service.LobeHubOIDCAccessToken, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(token)
	if err != nil {
		return "", fmt.Errorf("marshal lobehub oidc access token: %w", err)
	}
	tokenID, err := newOpaqueLobeHubOIDCToken(32)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, lobeHubOIDCAccessTokenKey(tokenID), payload, ttl).Err(); err != nil {
		return "", err
	}
	return tokenID, nil
}

func (s *lobeHubOIDCStateStore) GetAccessToken(ctx context.Context, token string) (*service.LobeHubOIDCAccessToken, error) {
	payload, err := s.rdb.Get(ctx, lobeHubOIDCAccessTokenKey(token)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrLobeHubOIDCInvalidToken
		}
		return nil, err
	}

	var accessToken service.LobeHubOIDCAccessToken
	if err := json.Unmarshal([]byte(payload), &accessToken); err != nil {
		return nil, fmt.Errorf("unmarshal lobehub oidc access token: %w", err)
	}
	return &accessToken, nil
}

func (s *lobeHubOIDCStateStore) CreateResumeToken(ctx context.Context, resume *service.LobeHubOIDCResumeToken, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(resume)
	if err != nil {
		return "", fmt.Errorf("marshal lobehub oidc resume token: %w", err)
	}
	resumeID, err := newOpaqueLobeHubOIDCToken(24)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, lobeHubOIDCResumeTokenKey(resumeID), payload, ttl).Err(); err != nil {
		return "", err
	}
	return resumeID, nil
}

func (s *lobeHubOIDCStateStore) GetResumeToken(ctx context.Context, resumeID string) (*service.LobeHubOIDCResumeToken, error) {
	payload, err := s.rdb.Get(ctx, lobeHubOIDCResumeTokenKey(resumeID)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrLobeHubOIDCResumeTokenNotFound
		}
		return nil, err
	}

	var resume service.LobeHubOIDCResumeToken
	if err := json.Unmarshal([]byte(payload), &resume); err != nil {
		return nil, fmt.Errorf("unmarshal lobehub oidc resume token: %w", err)
	}
	return &resume, nil
}

func (s *lobeHubOIDCStateStore) DeleteResumeToken(ctx context.Context, resumeID string) error {
	return s.rdb.Del(ctx, lobeHubOIDCResumeTokenKey(resumeID)).Err()
}

func (s *lobeHubOIDCStateStore) CreateBootstrapTicket(ctx context.Context, ticket *service.LobeHubBootstrapTicket, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(ticket)
	if err != nil {
		return "", fmt.Errorf("marshal lobehub bootstrap ticket: %w", err)
	}
	ticketID, err := newOpaqueLobeHubOIDCToken(24)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, lobeHubBootstrapTicketKey(ticketID), payload, ttl).Err(); err != nil {
		return "", err
	}
	return ticketID, nil
}

func (s *lobeHubOIDCStateStore) ConsumeBootstrapTicket(ctx context.Context, ticketID string) (*service.LobeHubBootstrapTicket, error) {
	payload, err := s.rdb.GetDel(ctx, lobeHubBootstrapTicketKey(ticketID)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrLobeHubBootstrapTicketNotFound
		}
		return nil, err
	}

	var ticket service.LobeHubBootstrapTicket
	if err := json.Unmarshal([]byte(payload), &ticket); err != nil {
		return nil, fmt.Errorf("unmarshal lobehub bootstrap ticket: %w", err)
	}
	return &ticket, nil
}

func lobeHubOIDCWebSessionKey(sessionID string) string {
	return lobeHubOIDCWebSessionKeyPrefix + sessionID
}

func lobeHubOIDCAuthorizationCodeKey(code string) string {
	return lobeHubOIDCAuthorizationCodeKeyPrefix + code
}

func lobeHubOIDCAccessTokenKey(token string) string {
	return lobeHubOIDCAccessTokenKeyPrefix + token
}

func lobeHubOIDCResumeTokenKey(resumeID string) string {
	return lobeHubOIDCResumeTokenKeyPrefix + resumeID
}

func lobeHubBootstrapTicketKey(ticketID string) string {
	return lobeHubBootstrapTicketKeyPrefix + ticketID
}

func newOpaqueLobeHubOIDCToken(byteLen int) (string, error) {
	if byteLen <= 0 {
		byteLen = 24
	}
	raw := make([]byte, byteLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate lobehub oidc token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
