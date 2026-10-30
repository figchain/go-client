package backup

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/figchain/go-client/pkg/encryption"
	"golang.org/x/crypto/curve25519"
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

// CalculateKeyFingerprint returns the lowercase hex SHA-256 of the raw 32-byte
// X25519 public key derived from the private key. For 64-byte keys the first
// 32 bytes are used as the X25519 private key (consistent with DecryptAesKey).
func CalculateKeyFingerprint(key any) (string, error) {
	keyBytes, ok := key.([]byte)
	if !ok {
		return "", fmt.Errorf("invalid key type")
	}
	if len(keyBytes) != 32 && len(keyBytes) != 64 {
		return "", fmt.Errorf("invalid key length for fingerprint derivation")
	}

	pubBytes, err := curve25519.X25519(keyBytes[:32], curve25519.Basepoint)
	if err != nil {
		return "", fmt.Errorf("failed to derive X25519 public key: %w", err)
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

// DecryptData decrypts the payload using AES-GCM (no AAD, as used by v2 backups).
func DecryptData(encryptedDataBase64 string, aesKey []byte) (string, error) {
	return DecryptDataWithAAD(encryptedDataBase64, aesKey, nil)
}

// DecryptDataWithAAD decrypts the payload using AES-GCM, authenticating aad.
func DecryptDataWithAAD(encryptedDataBase64 string, aesKey []byte, aad []byte) (string, error) {
	blob, err := base64.StdEncoding.DecodeString(encryptedDataBase64)
	if err != nil {
		return "", fmt.Errorf("invalid data base64: %w", err)
	}

	plaintext, err := encryption.DecryptAESGCMWithAAD(blob, aesKey, aad)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
