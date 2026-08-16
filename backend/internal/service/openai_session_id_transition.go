package service

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// openAIPreCanonicalSessionHashBridge carries exactly one deterministic
// pre-session-id sticky hash for the lifetime of a request.  v0.1.177's
// scheduler did not consider the Codex session-id header, so a request with
// that header used the next old-priority signal instead.  Keeping the single
// derived hash lets the new scheduler and WS state recovery probe that known
// old key without enumerating cache keys or treating an arbitrary hash as a
// fallback.
//
// primaryHash is retained with the bridge to prevent a later call that derives
// a different current hash from accidentally reusing a stale fallback stored
// on the same gin request.
type openAIPreCanonicalSessionHashBridge struct {
	primaryHash        string
	fallbackHash       string
	fallbackLegacyHash string
}

type openAIPreCanonicalSessionHashContextKey struct{}

var openAIPreCanonicalSessionHashKey = openAIPreCanonicalSessionHashContextKey{}

func openAIPreCanonicalSessionHashFromContext(ctx context.Context, primaryHash string) string {
	bridge, ok := openAIPreCanonicalSessionHashBridgeFromContext(ctx, primaryHash)
	if !ok {
		return ""
	}
	fallbackHash := strings.TrimSpace(bridge.fallbackHash)
	if fallbackHash == "" || fallbackHash == strings.TrimSpace(primaryHash) {
		return ""
	}
	return fallbackHash
}

func openAIPreCanonicalLegacySessionHashFromContext(ctx context.Context, primaryHash string) string {
	bridge, ok := openAIPreCanonicalSessionHashBridgeFromContext(ctx, primaryHash)
	if !ok {
		return ""
	}
	return strings.TrimSpace(bridge.fallbackLegacyHash)
}

func openAIPreCanonicalSessionHashBridgeFromContext(ctx context.Context, primaryHash string) (openAIPreCanonicalSessionHashBridge, bool) {
	if ctx == nil {
		return openAIPreCanonicalSessionHashBridge{}, false
	}
	bridge, ok := ctx.Value(openAIPreCanonicalSessionHashKey).(openAIPreCanonicalSessionHashBridge)
	if !ok || strings.TrimSpace(bridge.primaryHash) != strings.TrimSpace(primaryHash) {
		return openAIPreCanonicalSessionHashBridge{}, false
	}
	return bridge, true
}

func attachOpenAIPreCanonicalSessionHashToGin(c *gin.Context, primaryHash, fallbackHash, fallbackLegacyHash string) {
	if c == nil || c.Request == nil {
		return
	}
	primaryHash = strings.TrimSpace(primaryHash)
	fallbackHash = strings.TrimSpace(fallbackHash)
	fallbackLegacyHash = strings.TrimSpace(fallbackLegacyHash)
	if primaryHash == "" || fallbackHash == "" || primaryHash == fallbackHash {
		return
	}
	c.Request = c.Request.WithContext(context.WithValue(
		c.Request.Context(),
		openAIPreCanonicalSessionHashKey,
		openAIPreCanonicalSessionHashBridge{
			primaryHash:        primaryHash,
			fallbackHash:       fallbackHash,
			fallbackLegacyHash: fallbackLegacyHash,
		},
	))
}

// preCanonicalOpenAIHeaderSessionID reproduces the explicit-header precedence
// from the last version before session-id became the canonical source.  The
// fixed allowlist is deliberately retained: this transition must never turn a
// caller-provided arbitrary header into a cache lookup key.
func preCanonicalOpenAIHeaderSessionID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	for _, header := range explicitOpenAIHeaderSessionNames {
		if strings.EqualFold(header, "session-id") {
			continue
		}
		if sessionID := strings.TrimSpace(getHeaderRaw(c.Request.Header, header)); sessionID != "" {
			return sessionID
		}
	}
	return ""
}

func canonicalOpenAIHyphenatedSessionID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return strings.TrimSpace(getHeaderRaw(c.Request.Header, "session-id"))
}

// preCanonicalOpenAIRequestSessionID reproduces v0.1.177's full non-content
// precedence, but only when the new canonical session-id header is present.
// It is intentionally used solely as a transition bridge; normal scheduling
// continues to use explicitOpenAIRequestSessionID.
func preCanonicalOpenAIRequestSessionID(c *gin.Context, body []byte) string {
	if canonicalOpenAIHyphenatedSessionID(c) == "" {
		return ""
	}

	sessionID := preCanonicalOpenAIHeaderSessionID(c)
	if sessionID == "" && isGrokRequestContext(c) && c != nil && c.Request != nil {
		sessionID = strings.TrimSpace(getHeaderRaw(c.Request.Header, grokConversationIDHeader))
	}
	if sessionID == "" && len(body) > 0 {
		sessionID = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	}
	if sessionID == "" && isGrokRequestContext(c) && len(body) > 0 {
		sessionID = grokPreviousResponseSessionSeed(body)
	}
	return sessionID
}

// preCanonicalOpenAISessionHashes returns both cache-key forms that v0.1.177
// could have written for its old selected source: xxhash64 as primary and the
// short-lived SHA-256 compatibility mirror. Content fallback is opt-in because
// GenerateExplicitSessionHash historically did not use it.
func preCanonicalOpenAISessionHashes(c *gin.Context, body []byte, includeContentFallback bool) (currentHash, legacyHash string) {
	sessionID := preCanonicalOpenAIRequestSessionID(c, body)
	if sessionID == "" && includeContentFallback && canonicalOpenAIHyphenatedSessionID(c) != "" && len(body) > 0 {
		sessionID = deriveOpenAIContentSessionSeed(body)
	}
	if sessionID == "" {
		return "", ""
	}
	if isGrokRequestContext(c) {
		sessionID = grokStickyAffinitySeed(sessionID, body)
	}
	return deriveOpenAISessionHashes(sessionID)
}

func attachOpenAIPreCanonicalSessionHashBridge(c *gin.Context, primaryHash string, body []byte, includeContentFallback bool) {
	fallbackHash, fallbackLegacyHash := preCanonicalOpenAISessionHashes(c, body, includeContentFallback)
	attachOpenAIPreCanonicalSessionHashToGin(
		c,
		primaryHash,
		fallbackHash,
		fallbackLegacyHash,
	)
}

// attachOpenAIPreCanonicalSessionHashBridgeFromFallbackSeed covers callers
// which historically supplied an endpoint-specific fallback after ordinary
// session selection failed (for example Alpha Search's request id).  A new
// hyphenated session-id may now make normal selection succeed, so recreate
// that fallback only when v0.1.177 would have exhausted every fixed old
// header/body source.  This retains old header priority and makes exactly one
// deterministic compatibility key available; it never searches cache keys.
func attachOpenAIPreCanonicalSessionHashBridgeFromFallbackSeed(c *gin.Context, primaryHash string, body []byte, fallbackSeed string, includeContentFallback bool) {
	if canonicalOpenAIHyphenatedSessionID(c) == "" || strings.TrimSpace(primaryHash) == "" {
		return
	}
	oldHash, _ := preCanonicalOpenAISessionHashes(c, body, includeContentFallback)
	if oldHash != "" {
		return
	}
	fallbackHash, fallbackLegacyHash := deriveOpenAISessionHashes(fallbackSeed)
	attachOpenAIPreCanonicalSessionHashToGin(c, primaryHash, fallbackHash, fallbackLegacyHash)
}

// attachOpenAIPreCanonicalSessionHashBridgeFromWSFallback covers the one WSv2
// path that historically called GenerateSessionHash with no body and then
// explicitly fell back to its already-parsed prompt_cache_key.  It only runs
// after the old header/Grok precedence has produced no bridge, so this retains
// the exact old ordering rather than overwriting an explicit old header.
func attachOpenAIPreCanonicalSessionHashBridgeFromWSFallback(c *gin.Context, primaryHash, fallbackSeed string) {
	attachOpenAIPreCanonicalSessionHashBridgeFromFallbackSeed(c, primaryHash, nil, fallbackSeed, false)
}
