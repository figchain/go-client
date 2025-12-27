package transport

import (
	"crypto/rsa"
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
	privateKey       *rsa.PrivateKey
	serviceAccountID string
	tenantID         string
	namespace        string
	keyID            string
	tokenTTL         time.Duration
}

// NewPrivateKeyTokenProvider creates a new PrivateKeyTokenProvider.
func NewPrivateKeyTokenProvider(privateKey *rsa.PrivateKey, serviceAccountID, tenantID, namespace, keyID string) *PrivateKeyTokenProvider {
	return &PrivateKeyTokenProvider{
		privateKey:       privateKey,
		serviceAccountID: serviceAccountID,
		tenantID:         tenantID,
		namespace:        namespace,
		keyID:            keyID,
		tokenTTL:         10 * time.Minute,
	}
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

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if p.keyID != "" {
		token.Header["kid"] = p.keyID
	}

	signedToken, err := token.SignedString(p.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	// fmt.Printf("DEBUG: Generated JWT: %s\n", signedToken)
	// fmt.Printf("DEBUG: Generated JWT: %s\n", signedToken)
	return signedToken, nil
}
