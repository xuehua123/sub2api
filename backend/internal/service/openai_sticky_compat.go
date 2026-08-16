package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
)

type openAILegacySessionHashContextKey struct{}

var openAILegacySessionHashKey = openAILegacySessionHashContextKey{}

var (
	openAIStickyLegacyReadFallbackTotal atomic.Int64
	openAIStickyLegacyReadFallbackHit   atomic.Int64
	openAIStickyLegacyDualWriteTotal    atomic.Int64
)

func openAIStickyCompatStats() (legacyReadFallbackTotal, legacyReadFallbackHit, legacyDualWriteTotal int64) {
	return openAIStickyLegacyReadFallbackTotal.Load(),
		openAIStickyLegacyReadFallbackHit.Load(),
		openAIStickyLegacyDualWriteTotal.Load()
}

// DeriveSessionHashFromSeed computes the current-format sticky-session hash
// from an arbitrary seed string.
func DeriveSessionHashFromSeed(seed string) string {
	currentHash, _ := deriveOpenAISessionHashes(seed)
	return currentHash
}

func deriveOpenAISessionHashes(sessionID string) (currentHash string, legacyHash string) {
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" {
		return "", ""
	}

	currentHash = fmt.Sprintf("%016x", xxhash.Sum64String(normalized))
	sum := sha256.Sum256([]byte(normalized))
	legacyHash = hex.EncodeToString(sum[:])
	return currentHash, legacyHash
}

func withOpenAILegacySessionHash(ctx context.Context, legacyHash string) context.Context {
	if ctx == nil {
		return nil
	}
	trimmed := strings.TrimSpace(legacyHash)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, openAILegacySessionHashKey, trimmed)
}

func openAILegacySessionHashFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(openAILegacySessionHashKey).(string)
	return strings.TrimSpace(value)
}

func attachOpenAILegacySessionHashToGin(c *gin.Context, legacyHash string) {
	if c == nil || c.Request == nil {
		return
	}
	c.Request = c.Request.WithContext(withOpenAILegacySessionHash(c.Request.Context(), legacyHash))
}

func (s *OpenAIGatewayService) openAISessionHashReadOldFallbackEnabled() bool {
	if s == nil || s.cfg == nil {
		return true
	}
	return s.cfg.Gateway.OpenAIWS.SessionHashReadOldFallback
}

func (s *OpenAIGatewayService) openAISessionHashDualWriteOldEnabled() bool {
	if s == nil || s.cfg == nil {
		return true
	}
	return s.cfg.Gateway.OpenAIWS.SessionHashDualWriteOld
}

func (s *OpenAIGatewayService) openAISessionCacheKey(sessionHash string) string {
	normalized := strings.TrimSpace(sessionHash)
	if normalized == "" {
		return ""
	}
	return "openai:" + normalized
}

func (s *OpenAIGatewayService) openAILegacySessionCacheKey(ctx context.Context, sessionHash string) string {
	legacyHash := openAILegacySessionHashFromContext(ctx)
	if legacyHash == "" {
		return ""
	}
	legacyKey := "openai:" + legacyHash
	if legacyKey == s.openAISessionCacheKey(sessionHash) {
		return ""
	}
	return legacyKey
}

// openAIPreCanonicalSessionCacheKey is a narrow migration bridge for the
// session-id priority change.  Unlike the SHA-256 compatibility key above, it
// preserves v0.1.177's previous *selection source* (for example a known
// prompt_cache_key or content seed).  The bridge is attached only by
// GenerateSessionHash for the current request, so it can name at most one
// deterministic old key and remains subject to the normal group-scoped cache
// API.
func (s *OpenAIGatewayService) openAIPreCanonicalSessionCacheKey(ctx context.Context, sessionHash string) string {
	preCanonicalHash := openAIPreCanonicalSessionHashFromContext(ctx, sessionHash)
	if preCanonicalHash == "" {
		return ""
	}
	preCanonicalKey := s.openAISessionCacheKey(preCanonicalHash)
	if preCanonicalKey == s.openAISessionCacheKey(sessionHash) {
		return ""
	}
	return preCanonicalKey
}

// openAIPreCanonicalLegacySessionCacheKey is the SHA-256 mirror that a
// v0.1.177 slot may have dual-written for the same old selected source.  It
// remains deterministic request context only; this is never a keyspace scan.
func (s *OpenAIGatewayService) openAIPreCanonicalLegacySessionCacheKey(ctx context.Context, sessionHash string) string {
	legacyHash := openAIPreCanonicalLegacySessionHashFromContext(ctx, sessionHash)
	if legacyHash == "" {
		return ""
	}
	legacyKey := s.openAISessionCacheKey(legacyHash)
	if legacyKey == s.openAISessionCacheKey(sessionHash) {
		return ""
	}
	return legacyKey
}

func (s *OpenAIGatewayService) openAIStickyLegacyTTL(ttl time.Duration) time.Duration {
	legacyTTL := ttl
	if legacyTTL <= 0 {
		legacyTTL = openaiStickySessionTTL
	}
	if legacyTTL > 10*time.Minute {
		return 10 * time.Minute
	}
	return legacyTTL
}

func (s *OpenAIGatewayService) getStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, nil
	}

	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return 0, nil
	}

	accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), primaryKey)
	if err == nil && accountID > 0 {
		return accountID, nil
	}
	if !s.openAISessionHashReadOldFallbackEnabled() {
		return accountID, err
	}

	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey != "" {
		openAIStickyLegacyReadFallbackTotal.Add(1)
		legacyAccountID, legacyErr := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), legacyKey)
		if legacyErr == nil && legacyAccountID > 0 {
			openAIStickyLegacyReadFallbackHit.Add(1)
			return legacyAccountID, nil
		}
	}

	// v0.1.177 predates the session-id scheduling priority.  If this exact
	// request deterministically supplies its old selected hash, make one
	// group-scoped read for it after the established SHA-256 fallback.  Do not
	// infer or enumerate additional historical keys on a miss.
	preCanonicalKey := s.openAIPreCanonicalSessionCacheKey(ctx, sessionHash)
	if preCanonicalKey == "" || preCanonicalKey == legacyKey {
		return accountID, err
	}
	preCanonicalAccountID, preCanonicalErr := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), preCanonicalKey)
	if preCanonicalErr == nil && preCanonicalAccountID > 0 {
		return preCanonicalAccountID, nil
	}
	preCanonicalLegacyKey := s.openAIPreCanonicalLegacySessionCacheKey(ctx, sessionHash)
	if preCanonicalLegacyKey == "" || preCanonicalLegacyKey == legacyKey || preCanonicalLegacyKey == preCanonicalKey {
		return accountID, err
	}
	preCanonicalLegacyAccountID, preCanonicalLegacyErr := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), preCanonicalLegacyKey)
	if preCanonicalLegacyErr == nil && preCanonicalLegacyAccountID > 0 {
		return preCanonicalLegacyAccountID, nil
	}
	return accountID, err
}

func (s *OpenAIGatewayService) setStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if s == nil || s.cache == nil || accountID <= 0 {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}

	if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), primaryKey, accountID, ttl); err != nil {
		return err
	}

	if !s.openAISessionHashDualWriteOldEnabled() {
		return nil
	}
	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey != "" {
		if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), legacyKey, accountID, s.openAIStickyLegacyTTL(ttl)); err != nil {
			return err
		}
		openAIStickyLegacyDualWriteTotal.Add(1)
	}

	// Keep a short-lived mirror for a draining v0.1.177 slot, but only when
	// the current request supplied the exact old-precedence key.  This is the
	// same group-scoped GatewayCache operation as the primary write.
	preCanonicalKey := s.openAIPreCanonicalSessionCacheKey(ctx, sessionHash)
	if preCanonicalKey == "" || preCanonicalKey == legacyKey {
		return nil
	}
	if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), preCanonicalKey, accountID, s.openAIStickyLegacyTTL(ttl)); err != nil {
		return err
	}
	preCanonicalLegacyKey := s.openAIPreCanonicalLegacySessionCacheKey(ctx, sessionHash)
	if preCanonicalLegacyKey == "" || preCanonicalLegacyKey == legacyKey || preCanonicalLegacyKey == preCanonicalKey {
		return nil
	}
	if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), preCanonicalLegacyKey, accountID, s.openAIStickyLegacyTTL(ttl)); err != nil {
		return err
	}
	return nil
}

func (s *OpenAIGatewayService) refreshStickySessionTTL(ctx context.Context, groupID *int64, sessionHash string, ttl time.Duration) error {
	if s == nil || s.cache == nil {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}

	err := s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), primaryKey, ttl)
	if !s.openAISessionHashReadOldFallbackEnabled() && !s.openAISessionHashDualWriteOldEnabled() {
		return err
	}

	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey != "" {
		_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), legacyKey, s.openAIStickyLegacyTTL(ttl))
	}
	preCanonicalKey := s.openAIPreCanonicalSessionCacheKey(ctx, sessionHash)
	if preCanonicalKey != "" && preCanonicalKey != legacyKey {
		_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), preCanonicalKey, s.openAIStickyLegacyTTL(ttl))
	}
	preCanonicalLegacyKey := s.openAIPreCanonicalLegacySessionCacheKey(ctx, sessionHash)
	if preCanonicalLegacyKey != "" && preCanonicalLegacyKey != legacyKey && preCanonicalLegacyKey != preCanonicalKey {
		_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), preCanonicalLegacyKey, s.openAIStickyLegacyTTL(ttl))
	}
	return err
}

func (s *OpenAIGatewayService) deleteStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string) error {
	if s == nil || s.cache == nil {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}

	err := s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), primaryKey)
	if !s.openAISessionHashReadOldFallbackEnabled() && !s.openAISessionHashDualWriteOldEnabled() {
		return err
	}

	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey != "" {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), legacyKey)
	}
	preCanonicalKey := s.openAIPreCanonicalSessionCacheKey(ctx, sessionHash)
	if preCanonicalKey != "" && preCanonicalKey != legacyKey {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), preCanonicalKey)
	}
	preCanonicalLegacyKey := s.openAIPreCanonicalLegacySessionCacheKey(ctx, sessionHash)
	if preCanonicalLegacyKey != "" && preCanonicalLegacyKey != legacyKey && preCanonicalLegacyKey != preCanonicalKey {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), preCanonicalLegacyKey)
	}
	return err
}
