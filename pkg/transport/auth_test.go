package transport

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSharedSecretTokenProvider_GetToken(t *testing.T) {
	secret := "my-secret-token"
	provider := NewSharedSecretTokenProvider(secret)

	token, err := provider.GetToken()
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	if token != secret {
		t.Errorf("Expected token %s, got %s", secret, token)
	}
}

func TestPrivateKeyTokenProvider_GetToken(t *testing.T) {
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	serviceAccountID := "sa-123"
	tenantID := "tenant-456"
	namespace := "ns-1"
	keyID := "key-456"
	provider := NewPrivateKeyTokenProvider(pk, serviceAccountID, tenantID, namespace, keyID)

	tokenString, err := provider.GetToken()
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	// Verify token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return &pk.PublicKey, nil
	})

	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if !token.Valid {
		t.Error("Token is invalid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("Invalid claims type")
	}

	if iss, ok := claims["iss"].(string); !ok || iss != serviceAccountID {
		t.Errorf("Expected iss %s, got %v", serviceAccountID, claims["iss"])
	}
	if sub, ok := claims["sub"].(string); !ok || sub != serviceAccountID {
		t.Errorf("Expected sub %s, got %v", serviceAccountID, claims["sub"])
	}

	headerKid := token.Header["kid"]
	if headerKid != keyID {
		t.Errorf("Expected header kid %s, got %v", keyID, headerKid)
	}

	// Verify TTL
	expVal, ok := claims["exp"].(float64)
	if !ok {
		t.Fatal("exp claim missing or invalid")
	}
	expTime := time.Unix(int64(expVal), 0)
	if expTime.Before(time.Now()) {
		t.Error("Token is already expired")
	}
}
