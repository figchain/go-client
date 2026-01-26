package config

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func unsetEnv(keys []string) {
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}

func TestEnvCompatibilityAliases(t *testing.T) {
	keys := []string{
		"FIGCHAIN_URL",
		"FIGCHAIN_POLLING_INTERVAL_MS",
		"FIGCHAIN_RETRY_DELAY_MS",
		"FIGCHAIN_NAMESPACES",
		"FIGCHAIN_S3_BACKUP_PATH_STYLE_ACCESS",
		"FIGCHAIN_AUTH_CLIENT_ID",
	}
	defer unsetEnv(keys)

	_ = os.Setenv("FIGCHAIN_URL", "https://custom.example/")
	_ = os.Setenv("FIGCHAIN_POLLING_INTERVAL_MS", "45000")
	_ = os.Setenv("FIGCHAIN_RETRY_DELAY_MS", "250")
	_ = os.Setenv("FIGCHAIN_NAMESPACES", "ns1, ns2")
	_ = os.Setenv("FIGCHAIN_S3_BACKUP_PATH_STYLE_ACCESS", "true")
	_ = os.Setenv("FIGCHAIN_AUTH_CLIENT_ID", "client-123")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.BaseURL != "https://custom.example/" {
		t.Errorf("expected BaseURL from FIGCHAIN_URL, got %q", cfg.BaseURL)
	}

	if cfg.PollingInterval != 45*time.Second {
		t.Errorf("expected PollingInterval 45s, got %v", cfg.PollingInterval)
	}

	if cfg.RetryDelay != 250*time.Millisecond {
		t.Errorf("expected RetryDelay 250ms, got %v", cfg.RetryDelay)
	}

	expectedNS := []string{"ns1", "ns2"}
	if !reflect.DeepEqual(cfg.Namespaces, expectedNS) {
		t.Errorf("expected Namespaces %v, got %v", expectedNS, cfg.Namespaces)
	}

	if cfg.S3BackupPathStyleAccess != true {
		t.Errorf("expected S3BackupPathStyleAccess true, got %v", cfg.S3BackupPathStyleAccess)
	}

	if cfg.AuthClientID != "client-123" {
		t.Errorf("expected AuthClientID client-123, got %v", cfg.AuthClientID)
	}
}
