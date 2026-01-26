package transport

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPrivateKeyTokenProviderStruct(t *testing.T) {
	// 1. Generate standard Go key (64-byte private part of a keypair)
	_, priv, _ := ed25519.GenerateKey(nil)
	privHex64 := hex.EncodeToString(priv)

	// 2. Extract 32-byte seed (standard Ed25519 seed)
	seed := priv.Seed()
	seedHex32 := hex.EncodeToString(seed)

	t.Run("Accepts 64-byte private key", func(t *testing.T) {
		provider, err := NewPrivateKeyTokenProvider(privHex64, "svc", "tenant", "ns", "keyid")
		assert.NoError(t, err)
		assert.NotNil(t, provider)

		token, err := provider.GetToken()
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("Accepts 32-byte seed and expands it", func(t *testing.T) {
		provider, err := NewPrivateKeyTokenProvider(seedHex32, "svc", "tenant", "ns", "keyid")
		assert.NoError(t, err)
		assert.NotNil(t, provider)

		// Verify that the resulting key matches the 64-byte expected key
		assert.Equal(t, priv, provider.privateKey)

		token, err := provider.GetToken()
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("Rejects invalid length", func(t *testing.T) {
		invalidHex := hex.EncodeToString([]byte("invalid-length-bytes"))
		provider, err := NewPrivateKeyTokenProvider(invalidHex, "svc", "tenant", "ns", "keyid")
		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "invalid ed25519 private key length")
	})
}
