package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	fc_config "github.com/figchain/go-client/pkg/config"
	"github.com/figchain/go-client/pkg/model"
)

type VaultBackup struct {
	Version        string `json:"version"`
	KeyFingerprint string `json:"keyFingerprint"`
	EncryptedKey   string `json:"encryptedKey"`
	EncryptedData  string `json:"encryptedData"`
}

type VaultPayload struct {
	TenantID    string            `json:"tenantId"`
	GeneratedAt string            `json:"generatedAt"`
	SyncToken   string            `json:"syncToken"`
	Items       []model.FigFamily `json:"items"`
}

type VaultService struct {
	cfg     *fc_config.Config
	fetcher VaultFetcher
}

func NewVaultService(cfg *fc_config.Config, fetcher VaultFetcher) *VaultService {
	return &VaultService{cfg: cfg, fetcher: fetcher}
}

func NewDefaultVaultService(ctx context.Context, cfg *fc_config.Config) (*VaultService, error) {
	fetcher, err := NewS3VaultFetcher(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return NewVaultService(cfg, fetcher), nil
}

func (s *VaultService) LoadBackup(ctx context.Context) (*VaultPayload, error) {
	if !s.cfg.VaultEnabled {
		return nil, fmt.Errorf("vault is not enabled")
	}

	if s.cfg.VaultPrivateKeyPath == "" {
		return nil, fmt.Errorf("vault private key path is not configured")
	}

	// 1. Load Private Key
	privateKey, err := LoadPrivateKey(s.cfg.VaultPrivateKeyPath)
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

	var backup VaultBackup
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
	var payload VaultPayload
	if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	return &payload, nil
}
