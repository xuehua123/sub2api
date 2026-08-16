package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newSessionIDTransitionTestContext(t *testing.T, apiKeyID, groupID int64) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "canonical-codex-session")
	if apiKeyID > 0 {
		group := groupID
		c.Set("api_key", &APIKey{ID: apiKeyID, GroupID: &group})
	}
	return c
}

func TestPreCanonicalSessionIDBridge_ReproducesV01177Priority(t *testing.T) {
	const promptBody = `{"model":"gpt-5.4","prompt_cache_key":"old-prompt-key","input":"fallback input"}`
	contentBody := []byte(`{"model":"gpt-5.4","input":"first user turn"}`)

	tests := []struct {
		name         string
		headers      map[string]string
		body         []byte
		expectedSeed string
	}{
		{
			name:         "underscore session wins",
			headers:      map[string]string{"session_id": "old-underscore", "conversation_id": "old-conversation"},
			body:         []byte(promptBody),
			expectedSeed: "old-underscore",
		},
		{
			name:         "conversation wins after underscore absent",
			headers:      map[string]string{"conversation_id": "old-conversation", openCodeSessionAffinityHeader: "old-affinity"},
			body:         []byte(promptBody),
			expectedSeed: "old-conversation",
		},
		{
			name:         "opencode affinity wins",
			headers:      map[string]string{openCodeSessionAffinityHeader: "old-affinity", openCodeSessionIDHeader: "old-opencode-id"},
			body:         []byte(promptBody),
			expectedSeed: "old-affinity",
		},
		{
			name:         "opencode id wins",
			headers:      map[string]string{openCodeSessionIDHeader: "old-opencode-id", openCodeNativeSessionHeader: "old-native"},
			body:         []byte(promptBody),
			expectedSeed: "old-opencode-id",
		},
		{
			name:         "opencode native wins",
			headers:      map[string]string{openCodeNativeSessionHeader: "old-native", codeBuddyConversationHeader: "old-codebuddy"},
			body:         []byte(promptBody),
			expectedSeed: "old-native",
		},
		{
			name:         "codebuddy conversation wins",
			headers:      map[string]string{codeBuddyConversationHeader: "old-codebuddy"},
			body:         []byte(promptBody),
			expectedSeed: "old-codebuddy",
		},
		{
			name:         "prompt cache key wins before content",
			body:         []byte(promptBody),
			expectedSeed: "old-prompt-key",
		},
		{
			name:         "content fallback is deterministic",
			body:         contentBody,
			expectedSeed: deriveOpenAIContentSessionSeed(contentBody),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newSessionIDTransitionTestContext(t, 41, 17)
			for name, value := range tt.headers {
				c.Request.Header.Set(name, value)
			}

			primaryHash := (&OpenAIGatewayService{}).GenerateSessionHash(c, tt.body)
			expectedCurrentHash, expectedLegacyHash := deriveOpenAISessionHashes(tt.expectedSeed)
			require.Equal(t, DeriveSessionHashFromSeed("canonical-codex-session"), primaryHash)
			require.Equal(t, expectedCurrentHash, openAIPreCanonicalSessionHashFromContext(c.Request.Context(), primaryHash))
			require.Equal(t, expectedLegacyHash, openAIPreCanonicalLegacySessionHashFromContext(c.Request.Context(), primaryHash))
		})
	}
}

type sessionIDTransitionStickyCache struct {
	stubGatewayCache
	bindings map[string]int64
	reads    []string
}

func (c *sessionIDTransitionStickyCache) scopedKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%d:%s", groupID, sessionHash)
}

func (c *sessionIDTransitionStickyCache) GetSessionAccountID(_ context.Context, groupID int64, sessionHash string) (int64, error) {
	key := c.scopedKey(groupID, sessionHash)
	c.reads = append(c.reads, key)
	if accountID, ok := c.bindings[key]; ok {
		return accountID, nil
	}
	return 0, ErrStickySessionNotFound
}

func (c *sessionIDTransitionStickyCache) SetSessionAccountID(_ context.Context, groupID int64, sessionHash string, accountID int64, _ time.Duration) error {
	if c.bindings == nil {
		c.bindings = make(map[string]int64)
	}
	c.bindings[c.scopedKey(groupID, sessionHash)] = accountID
	return nil
}

func TestPreCanonicalSessionIDBridge_RecoversScopedStickyXXHashAndSHA256Mirror(t *testing.T) {
	const (
		groupID  int64 = 17
		apiKeyID int64 = 41
	)
	body := []byte(`{"model":"gpt-5.4","prompt_cache_key":"old-prompt-key"}`)
	c := newSessionIDTransitionTestContext(t, apiKeyID, groupID)
	svc := &OpenAIGatewayService{}
	primaryHash := svc.GenerateSessionHash(c, body)
	fallbackHash := openAIPreCanonicalSessionHashFromContext(c.Request.Context(), primaryHash)
	fallbackLegacyHash := openAIPreCanonicalLegacySessionHashFromContext(c.Request.Context(), primaryHash)
	require.NotEmpty(t, fallbackHash)
	require.NotEmpty(t, fallbackLegacyHash)

	t.Run("old xxhash binding", func(t *testing.T) {
		cache := &sessionIDTransitionStickyCache{bindings: map[string]int64{}}
		cache.bindings[cache.scopedKey(groupID, "openai:"+fallbackHash)] = 101
		svc.cache = cache

		requestGroupID := groupID
		accountID, err := svc.getStickySessionAccountID(c.Request.Context(), &requestGroupID, primaryHash)
		require.NoError(t, err)
		require.Equal(t, int64(101), accountID)
		require.Contains(t, cache.reads, cache.scopedKey(groupID, "openai:"+fallbackHash))
	})

	t.Run("old sha256 dual write", func(t *testing.T) {
		cache := &sessionIDTransitionStickyCache{bindings: map[string]int64{}}
		cache.bindings[cache.scopedKey(groupID, "openai:"+fallbackLegacyHash)] = 102
		svc.cache = cache

		requestGroupID := groupID
		accountID, err := svc.getStickySessionAccountID(c.Request.Context(), &requestGroupID, primaryHash)
		require.NoError(t, err)
		require.Equal(t, int64(102), accountID)
		require.Contains(t, cache.reads, cache.scopedKey(groupID, "openai:"+fallbackLegacyHash))
	})

	t.Run("other group is never read as fallback", func(t *testing.T) {
		cache := &sessionIDTransitionStickyCache{bindings: map[string]int64{}}
		cache.bindings[cache.scopedKey(groupID+1, "openai:"+fallbackHash)] = 103
		svc.cache = cache

		requestGroupID := groupID
		accountID, _ := svc.getStickySessionAccountID(c.Request.Context(), &requestGroupID, primaryHash)
		require.Zero(t, accountID)
		for _, read := range cache.reads {
			require.NotContains(t, read, fmt.Sprintf("%d:", groupID+1))
		}
	})
}

// ResponsesWebSocket obtains its local scheduling context before it has read
// the first response.create frame.  GenerateSessionHashWithFallback attaches
// the deterministic v0.1.177 bridge to c.Request.Context(), so the handler
// must refresh that local context before selecting an account.  This proves
// the refreshed WS scheduler context can recover the old scoped binding while
// the pre-generation snapshot cannot accidentally do so.
func TestPreCanonicalSessionIDBridge_WSContextRefreshEnablesStickyRecovery(t *testing.T) {
	const (
		groupID  int64 = 17
		apiKeyID int64 = 41
		account  int64 = 104
	)
	body := []byte(`{"type":"response.create","model":"gpt-5.4","prompt_cache_key":"old-ws-prompt-key"}`)
	c := newSessionIDTransitionTestContext(t, apiKeyID, groupID)
	staleSchedulingCtx := c.Request.Context()

	svc := &OpenAIGatewayService{}
	primaryHash := svc.GenerateSessionHashWithFallback(c, body, "ws-ingress-fallback")
	fallbackHash := openAIPreCanonicalSessionHashFromContext(c.Request.Context(), primaryHash)
	require.NotEmpty(t, fallbackHash)

	cache := &sessionIDTransitionStickyCache{bindings: map[string]int64{
		fmt.Sprintf("%d:%s", groupID, "openai:"+fallbackHash): account,
	}}
	svc.cache = cache
	requestGroupID := groupID

	staleAccountID, _ := svc.getStickySessionAccountID(staleSchedulingCtx, &requestGroupID, primaryHash)
	require.Zero(t, staleAccountID, "a pre-generation WS context must not see the bridge")

	refreshedAccountID, err := svc.getStickySessionAccountID(c.Request.Context(), &requestGroupID, primaryHash)
	require.NoError(t, err)
	require.Equal(t, account, refreshedAccountID, "the refreshed WS scheduler context must recover the old scoped binding")
}

func TestPreCanonicalSessionIDBridge_AlphaSearchFallbackRecoversSticky(t *testing.T) {
	const (
		groupID  int64 = 17
		apiKeyID int64 = 41
		account  int64 = 105
		searchID       = "search_legacy_request_id"
	)
	svc := &OpenAIGatewayService{}

	t.Run("old endpoint fallback is bridged", func(t *testing.T) {
		c := newSessionIDTransitionTestContext(t, apiKeyID, groupID)
		primaryHash := svc.GenerateSessionHashWithFallback(c, nil, searchID)
		fallbackHash, fallbackLegacyHash := deriveOpenAISessionHashes(searchID)
		require.Equal(t, DeriveSessionHashFromSeed("canonical-codex-session"), primaryHash)
		require.Equal(t, fallbackHash, openAIPreCanonicalSessionHashFromContext(c.Request.Context(), primaryHash))
		require.Equal(t, fallbackLegacyHash, openAIPreCanonicalLegacySessionHashFromContext(c.Request.Context(), primaryHash))

		cache := &sessionIDTransitionStickyCache{bindings: map[string]int64{
			fmt.Sprintf("%d:%s", groupID, "openai:"+fallbackHash): account,
		}}
		svc.cache = cache
		requestGroupID := groupID
		accountID, err := svc.getStickySessionAccountID(c.Request.Context(), &requestGroupID, primaryHash)
		require.NoError(t, err)
		require.Equal(t, account, accountID)
	})

	t.Run("old explicit header still wins over endpoint fallback", func(t *testing.T) {
		c := newSessionIDTransitionTestContext(t, apiKeyID, groupID)
		c.Request.Header.Set("session_id", "old-underscore-session")
		primaryHash := svc.GenerateSessionHashWithFallback(c, nil, searchID)
		expectedHash, _ := deriveOpenAISessionHashes("old-underscore-session")
		require.Equal(t, expectedHash, openAIPreCanonicalSessionHashFromContext(c.Request.Context(), primaryHash))
		require.NotEqual(t, DeriveSessionHashFromSeed(searchID), openAIPreCanonicalSessionHashFromContext(c.Request.Context(), primaryHash))
	})
}

func TestPreCanonicalSessionIDBridge_RawStateRecoveryRetainsEveryScope(t *testing.T) {
	const (
		groupID  int64 = 17
		apiKeyID int64 = 41
		account  int64 = 101
	)
	body := []byte(`{"model":"gpt-5.4","prompt_cache_key":"old-prompt-key"}`)
	sharedCache := &stubGatewayCacheWithOriginStore{
		stubCodexTurnStateOriginStore: stubCodexTurnStateOriginStore{
			data:        make(map[string]int64),
			sessionData: make(map[string]string),
		},
	}
	accountA := &Account{ID: account}

	// Blue is intentionally stored under the hash v0.1.177 derived before
	// session-id was recognized by scheduler state routing.
	blue := &OpenAIGatewayService{cache: sharedCache}
	cBlue := newSessionIDTransitionTestContext(t, apiKeyID, groupID)
	newHash := blue.GenerateSessionHash(cBlue, body)
	oldHash := openAIPreCanonicalSessionHashFromContext(cBlue.Request.Context(), newHash)
	require.NotEmpty(t, oldHash)
	require.True(t, blue.commitOpenAIWSSessionTurnState(cBlue, accountA, blue.getOpenAIWSStateStore(), groupID, oldHash, "old-handshake-state"))

	// Green's new canonical hash misses first, then probes only oldHash and
	// accepts it only after the normal exact-blob provenance check succeeds.
	green := &OpenAIGatewayService{cache: sharedCache}
	cGreen := newSessionIDTransitionTestContext(t, apiKeyID, groupID)
	greenHash := green.GenerateSessionHash(cGreen, body)
	require.Equal(t, newHash, greenHash)
	require.Equal(t, "old-handshake-state", green.loadOpenAIWSSessionTurnState(cGreen, accountA, green.getOpenAIWSStateStore(), groupID, greenHash))

	otherKey := newSessionIDTransitionTestContext(t, apiKeyID+1, groupID)
	otherKeyHash := green.GenerateSessionHash(otherKey, body)
	require.Empty(t, green.loadOpenAIWSSessionTurnState(otherKey, accountA, green.getOpenAIWSStateStore(), groupID, otherKeyHash), "another API key must not recover the old raw state")

	otherGroup := newSessionIDTransitionTestContext(t, apiKeyID, groupID+1)
	otherGroupHash := green.GenerateSessionHash(otherGroup, body)
	require.Empty(t, green.loadOpenAIWSSessionTurnState(otherGroup, accountA, green.getOpenAIWSStateStore(), groupID+1, otherGroupHash), "another group must not recover the old raw state")

	otherAccount := newSessionIDTransitionTestContext(t, apiKeyID, groupID)
	otherAccountHash := green.GenerateSessionHash(otherAccount, body)
	require.Empty(t, green.loadOpenAIWSSessionTurnState(otherAccount, &Account{ID: account + 1}, green.getOpenAIWSStateStore(), groupID, otherAccountHash), "another upstream account must not recover the old raw state")

	// The migration switch disables both sticky and raw-state compatibility
	// reads. A fresh store rules out a same-process L1 hit and proves the old
	// B key is not consulted when the operator turns the bridge off.
	compatDisabled := &OpenAIGatewayService{
		cache: sharedCache,
		cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAIWS: config.GatewayOpenAIWSConfig{SessionHashReadOldFallback: false},
		}},
	}
	cDisabled := newSessionIDTransitionTestContext(t, apiKeyID, groupID)
	disabledHash := compatDisabled.GenerateSessionHash(cDisabled, body)
	require.Empty(t, compatDisabled.loadOpenAIWSSessionTurnState(cDisabled, accountA, compatDisabled.getOpenAIWSStateStore(), groupID, disabledHash), "disabled old-hash reads must not restore raw WS state")

	// The bridge is best-effort by construction: if a historical content-based
	// hash cannot be re-derived from the current request, it does not scan or
	// guess another key.
	changedBody := []byte(`{"model":"gpt-5.4","input":"different first user turn"}`)
	cChanged := newSessionIDTransitionTestContext(t, apiKeyID, groupID)
	changedHash := green.GenerateSessionHash(cChanged, changedBody)
	require.Empty(t, green.loadOpenAIWSSessionTurnState(cChanged, accountA, green.getOpenAIWSStateStore(), groupID, changedHash))
}
