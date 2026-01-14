package encryption

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/transport"
)

type VaultConfig struct {
	Bucket    string
	Prefix    string
	Region    string
	Endpoint  string
	PathStyle bool
}

type Service struct {
	transport    transport.Transport
	privateKey   *rsa.PrivateKey
	nskCache     sync.Map
	VaultEnabled bool
	VaultConfig  VaultConfig
	ClientID     string
}

func NewService(t transport.Transport, privateKeyPath string) (*Service, error) {
	pk, err := LoadPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}
	return NewServiceWithKey(t, pk), nil
}

func NewServiceWithKey(t transport.Transport, pk *rsa.PrivateKey) *Service {
	return &Service{
		transport:  t,
		privateKey: pk,
	}
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
		// Fallback to AES-GCM (used by fc-ui)
		var errGCM error
		dek, errGCM = DecryptAESGCM(wrappedDek, nsk)
		if errGCM != nil {
			return nil, fmt.Errorf("unwrap dek failed (RFC3394: %v, AES-GCM: %v)", err, errGCM)
		}
	}

	payload, err := DecryptAESGCM(fig.Payload, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}

	return payload, nil
}

func (s *Service) getNSK(ctx context.Context, namespace, keyID string) ([]byte, error) {
	if keyID != "" {
		if val, ok := s.nskCache.Load(keyID); ok {
			return val.([]byte), nil
		}
	}

	var matchingKey *model.NamespaceKey

	// 1. Try API
	nsKeys, err := s.transport.GetNamespaceKey(ctx, namespace)
	if err == nil {
		for _, k := range nsKeys {
			if k.KeyID == keyID {
				matchingKey = k
				break
			}
		}
		if matchingKey == nil && keyID == "" && len(nsKeys) == 1 {
			matchingKey = nsKeys[0]
		}
	} else {
		if !s.VaultEnabled {
			// If api failed and vault disabled, return error
			return nil, err
		}
		// proceed to fallback
	}

	// 2. Fallback to S3
	if matchingKey == nil && s.VaultEnabled && s.ClientID != "" {
		s3Key, errS3 := s.fetchFromS3(ctx, namespace)
		if errS3 != nil {
			log.Printf("WARN: Failed to fetch NSK from S3: %v", errS3)
		} else {
			// Check ID match if requested
			if keyID == "" || (s3Key.KeyID == keyID) {
				matchingKey = s3Key
			}
		}
	}

	if matchingKey == nil {
		return nil, fmt.Errorf("no matching key found for namespace %s and keyId %s (API: %v)", namespace, keyID, err)
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

func (s *Service) fetchFromS3(ctx context.Context, namespace string) (*model.NamespaceKey, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	if s.VaultConfig.Region != "" {
		cfg.Region = s.VaultConfig.Region
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if s.VaultConfig.Endpoint != "" {
			o.BaseEndpoint = aws.String(s.VaultConfig.Endpoint)
		}
		o.UsePathStyle = s.VaultConfig.PathStyle
	})

	prefix := s.VaultConfig.Prefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// Path: {prefix}devices/{client_id}/namespaces/{namespace}.json
	key := fmt.Sprintf("%sdevices/%s/namespaces/%s.json", prefix, s.ClientID, namespace)

	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.VaultConfig.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()

	bodyBytes, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, err
	}

	// Need a struct to unmarshal. NamespaceKey model has JSON tags?
	var raw struct {
		KeyID      interface{} `json:"keyId"` // Can be int or string
		WrappedKey string      `json:"wrappedKey"`
		Algorithm  string      `json:"algorithm"`
	}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, err
	}

	keyIDStr := fmt.Sprintf("%v", raw.KeyID) // handle int/string flexibility

	return &model.NamespaceKey{
		KeyID:      keyIDStr,
		WrappedKey: raw.WrappedKey,
	}, nil
}
