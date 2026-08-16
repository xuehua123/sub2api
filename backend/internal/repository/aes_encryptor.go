package repository

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// AESEncryptor implements SecretEncryptor using AES-256-GCM
type AESEncryptor struct {
	key []byte
}

// NewAESEncryptor creates a new AES encryptor
func NewAESEncryptor(cfg *config.Config) (service.SecretEncryptor, error) {
	key, err := hex.DecodeString(cfg.Totp.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("invalid totp encryption key: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("totp encryption key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}

	return &AESEncryptor{key: key}, nil
}

// NewCodexTurnStateEncryptor derives a dedicated AES-GCM key from the
// mandatory JWT secret. TOTP's encryption key may intentionally be generated
// per process when it is absent, which would make a blue/green handoff unable
// to decrypt shared state. JWT signing material is required at normal startup
// and must already remain stable across slots for authenticated sessions.
//
// Domain separation ensures this does not reuse the JWT signing bytes directly
// as an AES key or collide with any other encrypted payload type.
func NewCodexTurnStateEncryptor(cfg *config.Config) (service.SecretEncryptor, error) {
	if cfg == nil {
		return nil, fmt.Errorf("codex turn-state encryption config is nil")
	}
	jwtSecret := strings.TrimSpace(cfg.JWT.Secret)
	if jwtSecret == "" {
		return nil, fmt.Errorf("codex turn-state encryption requires jwt secret")
	}
	derived := sha256.Sum256([]byte("sub2api:codex-turn-state:v1\x00" + jwtSecret))
	key := make([]byte, len(derived))
	copy(key, derived[:])
	return &AESEncryptor{key: key}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM
// Output format: base64(nonce + ciphertext + tag)
func (e *AESEncryptor) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	// Generate a random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	// Encrypt the plaintext
	// Seal appends the ciphertext and tag to the nonce
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode as base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-256-GCM
func (e *AESEncryptor) Decrypt(ciphertext string) (string, error) {
	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce, ciphertextData := data[:nonceSize], data[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
