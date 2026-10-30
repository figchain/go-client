package backup

import (
	"context"
	"fmt"
	"io"

	fc_config "github.com/figchain/go-client/pkg/config"
	"github.com/figchain/go-client/pkg/model"
)

type BackupPayload struct {
	TenantID    string            `json:"tenantId"`
	GeneratedAt string            `json:"generatedAt"`
	SyncToken   string            `json:"syncToken"`
	Items       []model.FigFamily `json:"items"`
	// Schemas maps schema URI to schema text, from the items and the v3 public section.
	Schemas map[string]string `json:"-"`
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
	// Backups are encrypted to the X25519 encryption key. Only fall back to the auth key when no
	// encryption key is configured: a valid Ed25519 auth key always loads, so trying it first
	// would silently pick the wrong key whenever both are set.
	keyHex := s.cfg.EncryptionPrivateKey
	if keyHex == "" {
		keyHex = s.cfg.AuthPrivateKey
	}
	privateKey, err := LoadPrivateKey(keyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
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

	return DecryptBackup(backupBytes, privateKey)
}
