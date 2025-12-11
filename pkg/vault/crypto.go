package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
)

// LoadPrivateKey loads an RSA private key from a PEM file.
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the key")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try parsing as PKCS1
		var pkcs1Err error
		key, pkcs1Err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if pkcs1Err != nil {
			return nil, fmt.Errorf("failed to parse private key (PKCS8: %v, PKCS1: %v)", err, pkcs1Err)
		}
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not an RSA private key")
	}

	return rsaKey, nil
}

// CalculateKeyFingerprint calculates the SHA-256 fingerprint of the public key.
func CalculateKeyFingerprint(key *rsa.PrivateKey) (string, error) {
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	hash := sha256.Sum256(pubKeyBytes)
	return hex.EncodeToString(hash[:]), nil
}

// DecryptAesKey decrypts the base64 encoded AES key using the RSA private key.
func DecryptAesKey(encryptedKeyBase64 string, privateKey *rsa.PrivateKey) ([]byte, error) {
	encryptedBytes, err := base64.StdEncoding.DecodeString(encryptedKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 key: %w", err)
	}

	// Java Uses OAEP with SHA-256 and MGF1 + SHA-256
	hash := sha256.New()
	aesKey, err := rsa.DecryptOAEP(hash, rand.Reader, privateKey, encryptedBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	return aesKey, nil
}

// DecryptData decrypts the base64 encoded data using AES-GCM.
func DecryptData(encryptedDataBase64 string, aesKey []byte) (string, error) {
	encryptedBytes, err := base64.StdEncoding.DecodeString(encryptedDataBase64)
	if err != nil {
		return "", fmt.Errorf("invalid base64 data: %w", err)
	}

	if len(encryptedBytes) < 12 {
		return "", fmt.Errorf("encrypted data too short")
	}

	iv := encryptedBytes[:12]
	ciphertext := encryptedBytes[12:]

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCMWithNonceSize(block, 12) // Default tag size is 16 bytes (128 bits)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := aesgcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt data: %w", err)
	}

	return string(plaintext), nil
}
