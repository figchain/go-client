package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/figchain/go-client/pkg/util"
)

var (
	ErrInvalidKey = errors.New("invalid key")
	ErrUnwrap     = errors.New("unwrap failed")
)

func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	return util.LoadRSAPrivateKey(path)
}

func DecryptRSAOAEP(cipherText []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hash := sha256.New()
	return rsa.DecryptOAEP(hash, rand.Reader, privateKey, cipherText, nil)
}

func DecryptAESGCM(cipherText []byte, key []byte) ([]byte, error) {
	if len(cipherText) < 12 {
		return nil, fmt.Errorf("cipher text too short")
	}
	iv := cipherText[:12]
	actualCipher := cipherText[12:]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesgcm, err := cipher.NewGCMWithNonceSize(block, 12)
	if err != nil {
		return nil, err
	}
	return aesgcm.Open(nil, iv, actualCipher, nil)
}

// UnwrapAESKey implements RFC 3394 AES Key Unwrap.
func UnwrapAESKey(wrappedKey, kek []byte) ([]byte, error) {
	if len(wrappedKey)%8 != 0 {
		return nil, errors.New("invalid wrapped key length")
	}
	n := len(wrappedKey)/8 - 1
	if n < 1 {
		return nil, errors.New("wrapped key too short")
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}

	a := make([]byte, 8)
	copy(a, wrappedKey[:8])

	r := make([]byte, len(wrappedKey)-8)
	copy(r, wrappedKey[8:])

	for j := 5; j >= 0; j-- {
		for i := n; i >= 1; i-- {
			t := uint64(n*j + i)

			// A = A ^ t
			val := binary.BigEndian.Uint64(a)
			binary.BigEndian.PutUint64(a, val^t)

			// B = AES_DEC(K, A | R[i])
			offset := (i - 1) * 8

			input := make([]byte, 16)
			copy(input[:8], a)
			copy(input[8:], r[offset:offset+8])

			output := make([]byte, 16)
			block.Decrypt(output, input)

			// A = MSB(64, B)
			copy(a, output[:8])
			// R[i] = LSB(64, B)
			copy(r[offset:offset+8], output[8:])
		}
	}

	// Check IV (0xA6A6A6A6A6A6A6A6)
	if binary.BigEndian.Uint64(a) != 0xA6A6A6A6A6A6A6A6 {
		return nil, errors.New("integrity check failed")
	}

	return r, nil
}
