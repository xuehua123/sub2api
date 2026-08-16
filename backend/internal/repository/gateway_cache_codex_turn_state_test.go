package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheCodexTurnStateIsEncryptedAndReusableAcrossSlots(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	// Blue and green intentionally use distinct TOTP keys. The turn-state key
	// must instead derive from their shared JWT signing secret, which normal
	// startup requires and which is already a blue/green compatibility boundary.
	blueCfg := &config.Config{
		JWT:  config.JWTConfig{Secret: strings.Repeat("j", 32)},
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("a", 64)},
	}
	greenCfg := &config.Config{
		JWT:  config.JWTConfig{Secret: strings.Repeat("j", 32)},
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("b", 64)},
	}
	blueCipher, err := NewCodexTurnStateEncryptor(blueCfg)
	require.NoError(t, err)
	greenCipher, err := NewCodexTurnStateEncryptor(greenCfg)
	require.NoError(t, err)

	blueCache := NewGatewayCache(client, blueCipher)
	_, ok := blueCache.(service.CodexTurnStateSessionStore)
	require.True(t, ok)
	blueOriginStore, ok := blueCache.(service.CodexTurnStateOriginStore)
	require.True(t, ok)

	const (
		scope     = "7:11:29:session-hash"
		turnState = "opaque-handshake-credential-do-not-log"
		originKey = "11:session-state-hash"
	)
	ctx := context.Background()
	require.NoError(t, blueOriginStore.SetTurnStateOrigin(ctx, originKey, 29, time.Minute))

	// Exercise the actual WS L1/L2 adapter on two separately constructed slots,
	// not only the cache interface. Green has no local state and must recover
	// from Blue's encrypted Redis record.
	blueWSStore := service.NewOpenAIWSStateStore(blueCache)
	blueWSStore.BindSessionTurnState(7, 11, 29, "session-hash", turnState, time.Minute)

	// Redis holds only AES-GCM ciphertext: no raw state (or scope envelope) is
	// visible to a cache dump, keyspace inspection, or accidental debug log.
	redisKey := codexSessionTurnStateRedisKey(scope)
	raw, err := client.Get(ctx, redisKey).Result()
	require.NoError(t, err)
	require.NotContains(t, raw, turnState)
	require.NotContains(t, raw, scope)
	keys, err := client.Keys(ctx, codexSessionTurnStatePrefix+"*").Result()
	require.NoError(t, err)
	require.Equal(t, []string{redisKey}, keys)
	require.NotContains(t, strings.Join(keys, ","), scope)

	// A separately constructed green cache decrypts the same record despite a
	// different TOTP key, proving the production derivation is cross-slot stable.
	greenCache := NewGatewayCache(client, greenCipher)
	greenSessionStore, ok := greenCache.(service.CodexTurnStateSessionStore)
	require.True(t, ok)
	greenOriginStore, ok := greenCache.(service.CodexTurnStateOriginStore)
	require.True(t, ok)

	accountID, originTTL, originFound, err := greenOriginStore.GetTurnStateOrigin(ctx, originKey)
	require.NoError(t, err)
	require.True(t, originFound)
	require.Equal(t, int64(29), accountID)
	require.Positive(t, originTTL)

	recovered, stateTTL, found, err := greenSessionStore.GetSessionTurnState(ctx, scope)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, turnState, recovered)
	require.Positive(t, stateTTL)
	greenWSStore := service.NewOpenAIWSStateStore(greenCache)
	recoveredByGreenWS, foundByGreenWS := greenWSStore.GetSessionTurnState(7, 11, 29, "session-hash")
	require.True(t, foundByGreenWS, "green WS state store must recover Blue's encrypted L2 state")
	require.Equal(t, turnState, recoveredByGreenWS)

	// Copying an encrypted record into another API-key/session/account scope is
	// rejected because that exact scope is authenticated inside the envelope.
	const otherScope = "7:12:29:session-hash"
	require.NoError(t, client.Set(ctx, codexSessionTurnStateRedisKey(otherScope), raw, time.Minute).Err())
	_, _, found, err = greenSessionStore.GetSessionTurnState(ctx, otherScope)
	require.Error(t, err)
	require.False(t, found)

	// A process with a different JWT secret fails closed rather than guessing or
	// returning a raw credential.
	wrongCipher, err := NewCodexTurnStateEncryptor(&config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("x", 32)}})
	require.NoError(t, err)
	wrongCache := NewGatewayCache(client, wrongCipher)
	wrongStore, ok := wrongCache.(service.CodexTurnStateSessionStore)
	require.True(t, ok)
	_, _, found, err = wrongStore.GetSessionTurnState(ctx, scope)
	require.Error(t, err)
	require.False(t, found)
}

func TestProvideGatewayCache_BootstrapWithoutJWTDisablesRawStateL2(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	// `config.ProvideConfig` intentionally permits an empty JWT secret until
	// Ent bootstraps it from the database. Cache construction must not make that
	// setup path unavailable or substitute a process-local encryption key.
	cache := ProvideGatewayCache(client, &config.Config{})
	sessionStore, ok := cache.(service.CodexTurnStateSessionStore)
	require.True(t, ok)

	err := sessionStore.SetSessionTurnState(context.Background(), "1:2:3:bootstrap", "raw-state", time.Minute)
	require.Error(t, err)
	_, err = redisServer.Get(codexSessionTurnStateRedisKey("1:2:3:bootstrap"))
	require.Error(t, err, "missing bootstrap key must not persist raw state in plaintext or with a random cipher")

	// Ent bootstrap mutates this same config instance before serving traffic.
	// A delayed cipher derivation can then enable the protected L2 without a
	// process restart or a transient per-process key.
	cfg := &config.Config{}
	cache = ProvideGatewayCache(client, cfg)
	sessionStore, ok = cache.(service.CodexTurnStateSessionStore)
	require.True(t, ok)
	require.NotNil(t, sessionStore)
	require.Error(t, sessionStore.SetSessionTurnState(context.Background(), "1:2:3:after-bootstrap", "raw-state", time.Minute))
	cfg.JWT.Secret = strings.Repeat("j", 32)
	require.NoError(t, sessionStore.SetSessionTurnState(context.Background(), "1:2:3:after-bootstrap", "raw-state", time.Minute))
	state, _, found, err := sessionStore.GetSessionTurnState(context.Background(), "1:2:3:after-bootstrap")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "raw-state", state)
}
