package backup

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/figchain/go-client/pkg/encryption"
)

// LoadPrivateKey loads an Ed25519 or X25519 private key from a hex string.
func LoadPrivateKey(hexKey string) (any, error) {
	keyBytes, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key hex: %w", err)
	}

	if len(keyBytes) == 32 || len(keyBytes) == 64 {
		return keyBytes, nil
	}

	return nil, fmt.Errorf("invalid private key length: got %d, want 32 or 64", len(keyBytes))
}

// CalculateKeyFingerprint calculates the SHA-256 fingerprint of the public key.
func CalculateKeyFingerprint(key any) (string, error) {
	keyBytes, ok := key.([]byte)
	if !ok {
		return "", fmt.Errorf("invalid key type")
	}

	var pubBytes []byte
	if len(keyBytes) == 32 {
		// Assume seed, derive pub
		priv := ed25519.NewKeyFromSeed(keyBytes)
		pubBytes = priv.Public().(ed25519.PublicKey)
	} else if len(keyBytes) == 64 {
		pubBytes = keyBytes[32:]
	} else {
		return "", fmt.Errorf("invalid key length for fingerprint derivation")
	}

	hash := sha256.Sum256(pubBytes)
	return hex.EncodeToString(hash[:]), nil
}

// DecryptAesKey decrypts the AES secret key using the X25519 private key.
func DecryptAesKey(encryptedKeyBase64 string, privateKey any) ([]byte, error) {
	privBytes, ok := privateKey.([]byte)
	if !ok {
		return nil, fmt.Errorf("invalid private key type")
	}
	if len(privBytes) > 32 {
		privBytes = privBytes[:32] // Use seed
	}

	blob, err := base64.StdEncoding.DecodeString(encryptedKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid key base64: %w", err)
	}

	return encryption.DecryptX25519(blob, privBytes)
}

// DecryptData decrypts the payload using AES-GCM.
func DecryptData(encryptedDataBase64 string, aesKey []byte) (string, error) {
	blob, err := base64.StdEncoding.DecodeString(encryptedDataBase64)
	if err != nil {
		return "", fmt.Errorf("invalid data base64: %w", err)
	}

	plaintext, err := encryption.DecryptAESGCM(blob, aesKey)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
