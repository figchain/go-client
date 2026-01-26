package config

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// BootstrapStrategy defines the strategy for bootstrapping the client.
type BootstrapStrategy string

const (
	BootstrapStrategyServer       BootstrapStrategy = "server"
	BootstrapStrategyServerFirst  BootstrapStrategy = "server-first"
	BootstrapStrategyS3BackupOnly BootstrapStrategy = "s3-backup-only"
	BootstrapStrategyHybrid       BootstrapStrategy = "hybrid"
)

// Config holds the client configuration.
type Config struct {
	BaseURL           string            `mapstructure:"base_url"`
	LongPollingURL    string            `mapstructure:"long_polling_url"`
	EnvironmentID     string            `mapstructure:"environment_id"`
	TenantID          string            `mapstructure:"tenant_id"`
	PollingInterval   time.Duration     `mapstructure:"polling_interval"`
	MaxRetries        int               `mapstructure:"max_retries"`
	RetryDelay        time.Duration     `mapstructure:"retry_delay"`
	AsOfTimestamp     string            `mapstructure:"as_of_timestamp"`
	Namespaces        []string          `mapstructure:"namespaces"`
	HTTPClient        *http.Client      `mapstructure:"-"` // Cannot be configured via yaml/env
	ClientSecret      string            `mapstructure:"client_secret"`
	UseLongPolling    bool              `mapstructure:"use_long_polling"`
	BootstrapStrategy BootstrapStrategy `mapstructure:"bootstrap_strategy"`

	// S3 Backup Configuration
	S3BackupBucket          string `mapstructure:"s3_backup_bucket"`
	S3BackupPrefix          string `mapstructure:"s3_backup_prefix"`
	S3BackupRegion          string `mapstructure:"s3_backup_region"`
	S3BackupEndpoint        string `mapstructure:"s3_backup_endpoint"`
	S3BackupPathStyleAccess bool   `mapstructure:"s3_backup_path_style_access"`
	S3BackupEnabled         bool   `mapstructure:"s3_backup_enabled"`
	EncryptionPrivateKey    string `mapstructure:"encryption_private_key"` // Hex-encoded X25519
	AuthPrivateKey          string `mapstructure:"auth_private_key"`       // Hex-encoded Ed25519
	AuthClientID            string `mapstructure:"auth_client_id"`
	AuthCredentialID        string `mapstructure:"auth_credential_id"`
}

// LoadConfig loads configuration from a YAML file and environment variables.
func LoadConfig(path string) (*Config, error) {
	v := viper.New()

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("figchain")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
	}

	// Environment variable overrides
	v.SetEnvPrefix("FIGCHAIN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Compatibility mapping: some clients (Java/Python) use canonical env names
	// like FIGCHAIN_URL or FIGCHAIN_POLLING_INTERVAL_MS. Add fallbacks so the
	// Go client accepts those names while keeping its existing behavior.

	// Helper maps for environment variable loading
	stringMap := map[string]string{
		"FIGCHAIN_URL":                    "base_url",
		"FIGCHAIN_LONG_POLLING_URL":       "long_polling_url",
		"FIGCHAIN_CLIENT_SECRET":          "client_secret",
		"FIGCHAIN_ENVIRONMENT_ID":         "environment_id",
		"FIGCHAIN_AS_OF_TIMESTAMP":        "as_of_timestamp",
		"FIGCHAIN_BOOTSTRAP_STRATEGY":     "bootstrap_strategy",
		"FIGCHAIN_S3_BACKUP_BUCKET":       "s3_backup_bucket",
		"FIGCHAIN_S3_BACKUP_PREFIX":       "s3_backup_prefix",
		"FIGCHAIN_S3_BACKUP_REGION":       "s3_backup_region",
		"FIGCHAIN_S3_BACKUP_ENDPOINT":     "s3_backup_endpoint",
		"FIGCHAIN_ENCRYPTION_PRIVATE_KEY": "encryption_private_key",
		"FIGCHAIN_AUTH_CLIENT_ID":         "auth_client_id",
		"FIGCHAIN_AUTH_CREDENTIAL_ID":     "auth_credential_id",
		"FIGCHAIN_IDENTITY_PRIVATE_KEY":   "auth_private_key",
		"FIGCHAIN_TENANT_ID":              "tenant_id",
	}
	for env, key := range stringMap {
		if val, ok := os.LookupEnv(env); ok {
			v.Set(key, val)
		}
	}

	boolMap := map[string]string{
		"FIGCHAIN_S3_BACKUP_ENABLED":           "s3_backup_enabled",
		"FIGCHAIN_S3_BACKUP_PATH_STYLE_ACCESS": "s3_backup_path_style_access",
	}
	for env, key := range boolMap {
		if val, ok := os.LookupEnv(env); ok {
			if b, err := strconv.ParseBool(val); err == nil {
				v.Set(key, b)
			}
		}
	}

	intMap := map[string]string{
		"FIGCHAIN_MAX_RETRIES": "max_retries",
	}
	for env, key := range intMap {
		if val, ok := os.LookupEnv(env); ok {
			if i, err := strconv.Atoi(val); err == nil {
				v.Set(key, i)
			}
		}
	}

	durationMap := map[string]string{
		"FIGCHAIN_POLLING_INTERVAL_MS": "polling_interval",
		"FIGCHAIN_RETRY_DELAY_MS":      "retry_delay",
	}
	for env, key := range durationMap {
		if val, ok := os.LookupEnv(env); ok {
			if ms, err := strconv.Atoi(val); err == nil {
				v.Set(key, time.Duration(ms)*time.Millisecond)
			}
		}
	}

	// Plural takes precedence
	if val, ok := os.LookupEnv("FIGCHAIN_NAMESPACES"); ok && !v.IsSet("namespaces") {
		v.Set("namespaces", splitAndTrim(val))
	} else if val, ok := os.LookupEnv("FIGCHAIN_NAMESPACE"); ok && !v.IsSet("namespaces") {
		// Singular fallback
		v.Set("namespaces", []string{strings.TrimSpace(val)})
	}

	// Defaults
	v.SetDefault("base_url", "https://app.figchain.io/api/")
	v.SetDefault("polling_interval", "60s")
	v.SetDefault("max_retries", 3)
	v.SetDefault("retry_delay", "1s")
	v.SetDefault("use_long_polling", true)
	v.SetDefault("s3_backup_enabled", false)
	v.SetDefault("bootstrap_strategy", string(BootstrapStrategyServer))

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		// Config file not found is fine, we just rely on defaults/env vars
	}

	// Map camelCase keys from JSON to snake_case for mapstructure if needed.
	// This ensures consistency between different configuration sources.
	jsonKeyAliases := map[string]string{
		"environmentId": "environment_id",
		"tenantId":      "tenant_id",
		"credentialId":  "auth_credential_id",
		"privateKey":    "auth_private_key",
	}
	for camelKey, snakeKey := range jsonKeyAliases {
		if v.IsSet(camelKey) && !v.IsSet(snakeKey) {
			v.Set(snakeKey, v.Get(camelKey))
		}
	}

	// Handle legacy "namespace" field (single string) and "namespaces" (list)
	// Merge them if both exist
	uniqueNamespaces := make(map[string]struct{})

	if v.IsSet("namespace") {
		ns := v.GetString("namespace")
		if strings.TrimSpace(ns) != "" {
			uniqueNamespaces[ns] = struct{}{}
		}
	}

	if v.IsSet("namespaces") {
		switch val := v.Get("namespaces").(type) {
		case string:
			for _, s := range splitAndTrim(val) {
				uniqueNamespaces[s] = struct{}{}
			}
		case []interface{}:
			for _, it := range val {
				if s, ok := it.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						uniqueNamespaces[s] = struct{}{}
					}
				}
			}
		}
	}

	// Flatten unique map to slice
	finalNamespaces := make([]string, 0, len(uniqueNamespaces))
	for ns := range uniqueNamespaces {
		finalNamespaces = append(finalNamespaces, ns)
	}
	if len(finalNamespaces) > 0 {
		v.Set("namespaces", finalNamespaces)
	}

	// Ensure auth client id is string if set
	if v.IsSet("auth_client_id") {
		v.Set("auth_client_id", v.GetString("auth_client_id"))
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}

	// Manual handling for HTTPClient as it's not serializable
	config.HTTPClient = http.DefaultClient

	return &config, nil
}

// Option is a functional option for configuring the client.
type Option func(*Config)

// WithBaseURL sets the base URL for the API.
func WithBaseURL(url string) Option {
	return func(c *Config) {
		c.BaseURL = url
	}
}

// WithLongPollingURL sets the base URL for long polling.
func WithLongPollingURL(url string) Option {
	return func(c *Config) {
		c.LongPollingURL = url
	}
}

// WithEnvironmentID sets the environment ID.
func WithEnvironmentID(id string) Option {
	return func(c *Config) {
		c.EnvironmentID = id
	}
}

// WithTenantID sets the tenant ID.
func WithTenantID(id string) Option {
	return func(c *Config) {
		c.TenantID = id
	}
}

// WithPollingInterval sets the polling interval.
func WithPollingInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.PollingInterval = interval
	}
}

// WithMaxRetries sets the maximum number of retries.
func WithMaxRetries(retries int) Option {
	return func(c *Config) {
		c.MaxRetries = retries
	}
}

// WithRetryDelay sets the delay between retries.
func WithRetryDelay(delay time.Duration) Option {
	return func(c *Config) {
		c.RetryDelay = delay
	}
}

// WithAsOfTimestamp sets the as-of timestamp.
func WithAsOfTimestamp(timestamp string) Option {
	return func(c *Config) {
		c.AsOfTimestamp = timestamp
	}
}

// WithNamespaces sets the namespaces to fetch.
func WithNamespaces(namespaces ...string) Option {
	return func(c *Config) {
		c.Namespaces = namespaces
	}
}

// WithHTTPClient sets the HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) {
		c.HTTPClient = client
	}
}

// WithClientSecret sets the client secret.
func WithClientSecret(secret string) Option {
	return func(c *Config) {
		c.ClientSecret = secret
	}
}

// WithLongPolling enables or disables long polling.
func WithLongPolling(enable bool) Option {
	return func(c *Config) {
		c.UseLongPolling = enable
	}
}

// WithBootstrapStrategy sets the bootstrap strategy.
func WithBootstrapStrategy(strategy BootstrapStrategy) Option {
	return func(c *Config) {
		c.BootstrapStrategy = strategy
	}
}

// WithS3BackupBucket sets the S3 bucket for the S3 Backup.
func WithS3BackupBucket(bucket string) Option {
	return func(c *Config) {
		c.S3BackupBucket = bucket
	}
}

// WithS3BackupPrefix sets the object prefix for the S3 Backup.
func WithS3BackupPrefix(prefix string) Option {
	return func(c *Config) {
		c.S3BackupPrefix = prefix
	}
}

// WithS3BackupRegion sets the AWS region for the S3 Backup.
func WithS3BackupRegion(region string) Option {
	return func(c *Config) {
		c.S3BackupRegion = region
	}
}

// WithS3BackupEndpoint sets the custom endpoint for the S3 Backup (e.g. for MinIO).
func WithS3BackupEndpoint(endpoint string) Option {
	return func(c *Config) {
		c.S3BackupEndpoint = endpoint
	}
}

// WithS3BackupPathStyle sets whether to use path-style access for the S3 Backup.
func WithS3BackupPathStyle(enabled bool) Option {
	return func(c *Config) {
		c.S3BackupPathStyleAccess = enabled
	}
}

// WithS3BackupEnabled sets whether the S3 Backup is enabled.
func WithS3BackupEnabled(enabled bool) Option {
	return func(c *Config) {
		c.S3BackupEnabled = enabled
	}
}

// WithAuthClientID sets the auth client ID.
func WithAuthClientID(id string) Option {
	return func(c *Config) {
		c.AuthClientID = id
	}
}

// WithAuthPrivateKey sets the auth private key hex content.
func WithAuthPrivateKey(hex string) Option {
	return func(c *Config) {
		c.AuthPrivateKey = hex
	}
}

// WithAuthCredentialID sets the auth credential ID.
func WithAuthCredentialID(id string) Option {
	return func(c *Config) {
		c.AuthCredentialID = id
	}
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		BaseURL:           "https://app.figchain.io/api/",
		PollingInterval:   60 * time.Second,
		MaxRetries:        3,
		RetryDelay:        1 * time.Second,
		HTTPClient:        http.DefaultClient,
		UseLongPolling:    true,
		S3BackupEnabled:   false,
		BootstrapStrategy: BootstrapStrategyServer,
	}
}

// WithConfig replaces the configuration with the provided one.
func WithConfig(cfg *Config) Option {
	return func(c *Config) {
		*c = *cfg
	}
}
func splitAndTrim(s string) []string {
	parts := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
