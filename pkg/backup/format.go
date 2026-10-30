package backup

import (
	"encoding/json"
	"fmt"
)

// Supported backup format versions.
const (
	BackupVersionV2 = "v2"
	BackupVersionV3 = "v3"
)

type BackupFile struct {
	Version   string `json:"version"`
	Algorithm string `json:"algorithm,omitempty"`
	// PublicData is the v3 public section as a JSON string. Its exact bytes are
	// authenticated as AES-GCM AAD, so it must be kept verbatim.
	PublicData     string `json:"publicData,omitempty"`
	KeyFingerprint string `json:"keyFingerprint"`
	EncryptedKey   string `json:"encryptedKey"`
	EncryptedData  string `json:"encryptedData"`
}

// PublicSchema is a schema entry in the v3 public section.
type PublicSchema struct {
	URI         string `json:"uri,omitempty"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Format      string `json:"format,omitempty"`
	Shared      bool   `json:"shared,omitempty"`
	Content     string `json:"content,omitempty"`
	ContentText string `json:"contentText,omitempty"`
}

// PublicSection is the unencrypted public section of a v3 backup.
type PublicSection struct {
	GeneratedAt string                  `json:"generatedAt"`
	Schemas     map[string]PublicSchema `json:"schemas"`
}

// ParsePublicSection parses the public section of a v3 backup. It returns nil
// for backups without one (e.g. v2). Note: integrity is only verified when the
// backup is decrypted with the private key.
func (b *BackupFile) ParsePublicSection() (*PublicSection, error) {
	if b.Version != BackupVersionV3 {
		return nil, nil
	}
	var pub PublicSection
	if err := json.Unmarshal([]byte(b.PublicData), &pub); err != nil {
		return nil, fmt.Errorf("failed to parse public section: %w", err)
	}
	return &pub, nil
}

// DecryptBackup decrypts a raw backup file (v2 or v3) with the given private key.
// For v3 the exact bytes of publicData are authenticated as AES-GCM AAD.
func DecryptBackup(backupBytes []byte, privateKey any) (*BackupPayload, error) {
	var backup BackupFile
	if err := json.Unmarshal(backupBytes, &backup); err != nil {
		return nil, fmt.Errorf("failed to parse backup file: %w", err)
	}

	var aad []byte
	switch backup.Version {
	case BackupVersionV2:
		// no AAD
	case BackupVersionV3:
		aad = []byte(backup.PublicData)
	default:
		return nil, fmt.Errorf("unsupported backup version %q (supported: %s, %s)", backup.Version, BackupVersionV2, BackupVersionV3)
	}

	aesKey, err := DecryptAesKey(backup.EncryptedKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	jsonPayload, err := DecryptDataWithAAD(backup.EncryptedData, aesKey, aad)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt payload: %w", err)
	}

	var payload BackupPayload
	if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}
	return &payload, nil
}
