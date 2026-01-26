package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	fc_config "github.com/figchain/go-client/pkg/config"
	"github.com/figchain/go-client/pkg/model"
)

type BackupFile struct {
	Version        string `json:"version"`
	KeyFingerprint string `json:"keyFingerprint"`
	EncryptedKey   string `json:"encryptedKey"`
	EncryptedData  string `json:"encryptedData"`
}

type BackupPayload struct {
	TenantID    string            `json:"tenantId"`
	GeneratedAt string            `json:"generatedAt"`
	SyncToken   string            `json:"syncToken"`
	Items       []model.FigFamily `json:"items"`
}

type S3BackupService struct {
	cfg     *fc_config.Config
	fetcher BackupFetcher
}

func NewS3BackupService(cfg *fc_config.Config, fetcher BackupFetcher) *S3BackupService {
	return &S3BackupService{cfg: cfg, fetcher: fetcher}
}

func NewDefaultS3BackupService(ctx context.Context, cfg *fc_config.Config) (*S3BackupService, error) {
	fetcher, err := NewS3BackupFetcher(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return NewS3BackupService(cfg, fetcher), nil
}

func (s *S3BackupService) LoadBackup(ctx context.Context) (*BackupPayload, error) {
	if !s.cfg.S3BackupEnabled {
		return nil, fmt.Errorf("s3 backup is not enabled")
	}

	// 1. Load Private Key
	privateKey, err := LoadPrivateKey(s.cfg.AuthPrivateKey)
	if err != nil {
		// Fallback to encryption key if auth key not sufficient
		privateKey, err = LoadPrivateKey(s.cfg.EncryptionPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load private key: %w", err)
		}
	}

	// 2. Calculate Fingerprint
	fingerprint, err := CalculateKeyFingerprint(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate key fingerprint: %w", err)
	}

	// 3. Fetch Encrypted Backup
	reader, err := s.fetcher.FetchBackup(ctx, fingerprint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch backup: %w", err)
	}
	defer reader.Close()

	backupBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup: %w", err)
	}

	var backup BackupFile
	if err := json.Unmarshal(backupBytes, &backup); err != nil {
		return nil, fmt.Errorf("failed to parse backup file: %w", err)
	}

	// 4. Decrypt AES Key
	aesKey, err := DecryptAesKey(backup.EncryptedKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	// 5. Decrypt Data
	jsonPayload, err := DecryptData(backup.EncryptedData, aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt payload: %w", err)
	}

	// 6. Parse Payload
	var payload BackupPayload
	if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	return &payload, nil
}
