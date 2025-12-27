package encryption

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"log"
	"sync"

	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/transport"
)

type Service struct {
	transport  transport.Transport
	privateKey *rsa.PrivateKey
	nskCache   sync.Map
}

func NewService(t transport.Transport, privateKeyPath string) (*Service, error) {
	pk, err := LoadPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}
	return &Service{
		transport:  t,
		privateKey: pk,
	}, nil
}

func (s *Service) Decrypt(ctx context.Context, fig *model.Fig, namespace string) ([]byte, error) {
	if !fig.IsEncrypted {
		return fig.Payload, nil
	}

	keyID := ""
	if fig.KeyID != nil {
		keyID = *fig.KeyID
	}

	nsk, err := s.getNSK(ctx, namespace, keyID)
	if err != nil {
		return nil, fmt.Errorf("get nsk: %w", err)
	}

	wrappedDek := fig.WrappedDek
	if len(wrappedDek) == 0 {
		return nil, fmt.Errorf("missing wrapped dek")
	}

	dek, err := UnwrapAESKey(wrappedDek, nsk)
	if err != nil {
		return nil, fmt.Errorf("unwrap dek: %w", err)
	}

	payload, err := DecryptAESGCM(fig.Payload, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}

	log.Printf("DEBUG Decryption: encrypted=%d bytes, decrypted=%d bytes, hex=%x\n",
		len(fig.Payload), len(payload), payload)

	return payload, nil
}

func (s *Service) getNSK(ctx context.Context, namespace, keyID string) ([]byte, error) {
	if keyID != "" {
		if val, ok := s.nskCache.Load(keyID); ok {
			return val.([]byte), nil
		}
	}

	nsKeys, err := s.transport.GetNamespaceKey(ctx, namespace)
	if err != nil {
		return nil, err
	}

	var matchingKey *model.NamespaceKey
	for _, k := range nsKeys {
		if keyID == "" && k.KeyID == "" {
			matchingKey = k
			break
		}
		if keyID != "" && k.KeyID == keyID {
			matchingKey = k
			break
		}
	}

	if matchingKey == nil {
		if keyID == "" {
			// If no keyID specified and there's exactly one key, use it
			if len(nsKeys) == 1 {
				matchingKey = nsKeys[0]
			} else if len(nsKeys) > 1 {
				// Multiple keys exist but fig has no keyID - this is ambiguous and unsafe
				return nil, fmt.Errorf("namespace %s has %d keys but fig has no keyId specified; cannot determine which key to use", namespace, len(nsKeys))
			} else {
				return nil, fmt.Errorf("no keys found for namespace %s", namespace)
			}
		} else {
			return nil, fmt.Errorf("no matching key found for namespace %s and keyId %s", namespace, keyID)
		}
	}

	wrappedKeyBytes, err := base64.StdEncoding.DecodeString(matchingKey.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("decode nsk: %w", err)
	}

	unwrappedNsk, err := DecryptRSAOAEP(wrappedKeyBytes, s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt nsk: %w", err)
	}

	if matchingKey.KeyID != "" {
		s.nskCache.Store(matchingKey.KeyID, unwrappedNsk)
	}

	return unwrappedNsk, nil
}
