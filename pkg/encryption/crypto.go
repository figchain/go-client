package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

var (
	ErrInvalidKey = errors.New("invalid key")
	ErrUnwrap     = errors.New("unwrap failed")
)

// DecryptX25519 implements ECIES decryption consistent with the web client.
// Format: EphemeralPubKey (32 bytes) || IV (12 bytes) || Ciphertext
func DecryptX25519(packedBlob []byte, privateKeyBytes []byte) ([]byte, error) {
	if len(packedBlob) < 32+12 {
		return nil, fmt.Errorf("blob too short")
	}

	ephemeralPubKey := packedBlob[:32]
	iv := packedBlob[32:44]
	ciphertext := packedBlob[44:]

	// 1. Derive Shared Secret: X25519(myPriv, ephPub)
	sharedSecret, err := curve25519.X25519(privateKeyBytes, ephemeralPubKey)
	if err != nil {
		return nil, fmt.Errorf("ECDH failed: %w", err)
	}

	// 2. Derive KEK: HKDF-SHA256(sharedSecret, salt="", info="")
	kek := make([]byte, 32) // AES-256
	hkdfReader := hkdf.New(sha256.New, sharedSecret, nil, nil)
	if _, err := io.ReadFull(hkdfReader, kek); err != nil {
		return nil, fmt.Errorf("HKDF failed: %w", err)
	}

	// 3. Decrypt: AES-GCM(kek, iv, ciphertext)
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("AES creation failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("GCM creation failed: %w", err)
	}

	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// SignEd25519 signs a message using an Ed25519 private key.
// The private key must be 64 bytes (seed + public).
func SignEd25519(message []byte, privateKeyHex string) ([]byte, error) {
	privBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key hex: %w", err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: got %d, want %d", len(privBytes), ed25519.PrivateKeySize)
	}

	return ed25519.Sign(ed25519.PrivateKey(privBytes), message), nil
}

// DecryptAESGCM decrypts data using AES-GCM.
// Format: IV (12 bytes) || Ciphertext
func DecryptAESGCM(packedData []byte, key []byte) ([]byte, error) {
	if len(packedData) < 12 {
		return nil, fmt.Errorf("data too short")
	}
	iv := packedData[:12]
	ciphertext := packedData[12:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, iv, ciphertext, nil)
}
