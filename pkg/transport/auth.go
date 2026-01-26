package transport

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenProvider is an interface for providing authentication tokens.
type TokenProvider interface {
	GetToken() (string, error)
}

// SharedSecretTokenProvider uses a static client secret.
type SharedSecretTokenProvider struct {
	clientSecret string
}

// NewSharedSecretTokenProvider creates a new SharedSecretTokenProvider.
func NewSharedSecretTokenProvider(clientSecret string) *SharedSecretTokenProvider {
	return &SharedSecretTokenProvider{
		clientSecret: clientSecret,
	}
}

func (p *SharedSecretTokenProvider) GetToken() (string, error) {
	return p.clientSecret, nil
}

// PrivateKeyTokenProvider generates a signed JWT using a private key.
type PrivateKeyTokenProvider struct {
	privateKey       ed25519.PrivateKey
	serviceAccountID string
	tenantID         string
	namespace        string
	keyID            string
	tokenTTL         time.Duration
}

// NewPrivateKeyTokenProvider creates a new PrivateKeyTokenProvider.
// If tokenTTL is 0, it defaults to 10 minutes.
func NewPrivateKeyTokenProvider(privateKeyHex, serviceAccountID, tenantID, namespace, keyID string) (*PrivateKeyTokenProvider, error) {
	return NewPrivateKeyTokenProviderWithTTL(privateKeyHex, serviceAccountID, tenantID, namespace, keyID, 10*time.Minute)
}

// NewPrivateKeyTokenProviderWithTTL creates a new PrivateKeyTokenProvider with a custom TTL.
func NewPrivateKeyTokenProviderWithTTL(privateKeyHex, serviceAccountID, tenantID, namespace, keyID string, tokenTTL time.Duration) (*PrivateKeyTokenProvider, error) {
	if tokenTTL == 0 {
		tokenTTL = 10 * time.Minute
	}

	privBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key hex: %w", err)
	}
	var privateKey ed25519.PrivateKey
	if len(privBytes) == ed25519.SeedSize {
		privateKey = ed25519.NewKeyFromSeed(privBytes)
	} else if len(privBytes) == ed25519.PrivateKeySize {
		privateKey = ed25519.PrivateKey(privBytes)
	} else {
		return nil, fmt.Errorf("invalid ed25519 private key length: got %d, want %d or %d", len(privBytes), ed25519.SeedSize, ed25519.PrivateKeySize)
	}

	return &PrivateKeyTokenProvider{
		privateKey:       privateKey,
		serviceAccountID: serviceAccountID,
		tenantID:         tenantID,
		namespace:        namespace,
		keyID:            keyID,
		tokenTTL:         tokenTTL,
	}, nil
}

func (p *PrivateKeyTokenProvider) GetToken() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":       p.serviceAccountID,
		"sub":       p.serviceAccountID,
		"exp":       jwt.NewNumericDate(now.Add(p.tokenTTL)),
		"iat":       jwt.NewNumericDate(now),
		"nbf":       jwt.NewNumericDate(now),
		"tenant_id": p.tenantID,
	}
	if p.namespace != "" {
		claims["namespace"] = p.namespace
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	if p.keyID != "" {
		token.Header["kid"] = p.keyID
	}

	signedToken, err := token.SignedString(p.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, nil
}
