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
	BootstrapStrategyServer      BootstrapStrategy = "server"
	BootstrapStrategyServerFirst BootstrapStrategy = "server-first"
	BootstrapStrategyVault       BootstrapStrategy = "vault"
	BootstrapStrategyHybrid      BootstrapStrategy = "hybrid"
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

	// Vault Configuration
	VaultBucket              string `mapstructure:"vault_bucket"`
	VaultPrefix              string `mapstructure:"vault_prefix"`
	VaultRegion              string `mapstructure:"vault_region"`
	VaultEndpoint            string `mapstructure:"vault_endpoint"`
	VaultPathStyle           bool   `mapstructure:"vault_path_style"`
	VaultPrivateKeyPath      string `mapstructure:"vault_private_key_path"`
	VaultEnabled             bool   `mapstructure:"vault_enabled"`
	EncryptionPrivateKeyPath string `mapstructure:"encryption_private_key_path"`
	AuthPrivateKeyPath       string `mapstructure:"auth_private_key_path"`
	AuthPrivateKeyPEM        string `mapstructure:"auth_private_key_pem"`
	AuthClientID             string `mapstructure:"auth_client_id"`
	AuthCredentialID         string `mapstructure:"auth_credential_id"`
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
		"FIGCHAIN_URL":                         "base_url",
		"FIGCHAIN_LONG_POLLING_URL":            "long_polling_url",
		"FIGCHAIN_CLIENT_SECRET":               "client_secret",
		"FIGCHAIN_ENVIRONMENT_ID":              "environment_id",
		"FIGCHAIN_AS_OF_TIMESTAMP":             "as_of_timestamp",
		"FIGCHAIN_BOOTSTRAP_STRATEGY":          "bootstrap_strategy",
		"FIGCHAIN_VAULT_BUCKET":                "vault_bucket",
		"FIGCHAIN_VAULT_PREFIX":                "vault_prefix",
		"FIGCHAIN_VAULT_REGION":                "vault_region",
		"FIGCHAIN_VAULT_ENDPOINT":              "vault_endpoint",
		"FIGCHAIN_VAULT_PRIVATE_KEY_PATH":      "vault_private_key_path",
		"FIGCHAIN_ENCRYPTION_PRIVATE_KEY_PATH": "encryption_private_key_path",
		"FIGCHAIN_AUTH_PRIVATE_KEY_PATH":       "auth_private_key_path",
		"FIGCHAIN_AUTH_CLIENT_ID":              "auth_client_id",
		"FIGCHAIN_AUTH_CREDENTIAL_ID":          "auth_credential_id",
		"FIGCHAIN_TENANT_ID":                   "tenant_id",
	}
	for env, key := range stringMap {
		if val, ok := os.LookupEnv(env); ok && !v.IsSet(key) {
			v.Set(key, val)
		}
	}

	boolMap := map[string]string{
		"FIGCHAIN_VAULT_ENABLED":           "vault_enabled",
		"FIGCHAIN_VAULT_PATH_STYLE_ACCESS": "vault_path_style",
	}
	for env, key := range boolMap {
		if val, ok := os.LookupEnv(env); ok && !v.IsSet(key) {
			if b, err := strconv.ParseBool(val); err == nil {
				v.Set(key, b)
			}
		}
	}

	intMap := map[string]string{
		"FIGCHAIN_MAX_RETRIES": "max_retries",
	}
	for env, key := range intMap {
		if val, ok := os.LookupEnv(env); ok && !v.IsSet(key) {
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
		if val, ok := os.LookupEnv(env); ok && !v.IsSet(key) {
			if ms, err := strconv.Atoi(val); err == nil {
				v.Set(key, time.Duration(ms)*time.Millisecond)
			}
		}
	}

	if val, ok := os.LookupEnv("FIGCHAIN_NAMESPACES"); ok && !v.IsSet("namespaces") {
		v.Set("namespaces", splitAndTrim(val))
	}

	// Defaults
	v.SetDefault("base_url", "https://app.figchain.io/api/")
	v.SetDefault("polling_interval", "60s")
	v.SetDefault("max_retries", 3)
	v.SetDefault("retry_delay", "1s")
	v.SetDefault("use_long_polling", true)
	v.SetDefault("vault_enabled", false)
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
		"privateKey":    "auth_private_key_pem",
	}
	for camelKey, snakeKey := range jsonKeyAliases {
		if v.IsSet(camelKey) && !v.IsSet(snakeKey) {
			v.Set(snakeKey, v.Get(camelKey))
		}
	}

	// Handle legacy "namespace" field (single string)
	if v.IsSet("namespace") && !v.IsSet("namespaces") {
		v.Set("namespaces", []string{v.GetString("namespace")})
	}

	// Normalize namespaces types: support string, []interface{} and []string
	if v.IsSet("namespaces") {
		switch val := v.Get("namespaces").(type) {
		case string:
			v.Set("namespaces", splitAndTrim(val))
		case []interface{}:
			parts := []string{}
			for _, it := range val {
				if s, ok := it.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						parts = append(parts, s)
					}
				}
			}
			v.Set("namespaces", parts)
		}
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

// WithVaultBucket sets the S3 bucket for the Vault.
func WithVaultBucket(bucket string) Option {
	return func(c *Config) {
		c.VaultBucket = bucket
	}
}

// WithVaultPrefix sets the object prefix for the Vault.
func WithVaultPrefix(prefix string) Option {
	return func(c *Config) {
		c.VaultPrefix = prefix
	}
}

// WithVaultRegion sets the AWS region for the Vault.
func WithVaultRegion(region string) Option {
	return func(c *Config) {
		c.VaultRegion = region
	}
}

// WithVaultEndpoint sets the custom endpoint for the Vault (e.g. for MinIO).
func WithVaultEndpoint(endpoint string) Option {
	return func(c *Config) {
		c.VaultEndpoint = endpoint
	}
}

// WithVaultPathStyle sets whether to use path-style access for the Vault.
func WithVaultPathStyle(enabled bool) Option {
	return func(c *Config) {
		c.VaultPathStyle = enabled
	}
}

// WithVaultPrivateKeyPath sets the path to the private key for the Vault.
func WithVaultPrivateKeyPath(path string) Option {
	return func(c *Config) {
		c.VaultPrivateKeyPath = path
	}
}

// WithVaultEnabled sets whether the Vault is enabled.
func WithVaultEnabled(enabled bool) Option {
	return func(c *Config) {
		c.VaultEnabled = enabled
	}
}

// WithEncryptionPrivateKeyPath sets the path to the encryption private key.
func WithEncryptionPrivateKeyPath(path string) Option {
	return func(c *Config) {
		c.EncryptionPrivateKeyPath = path
	}
}

// WithAuthPrivateKeyPath sets the path to the authentication private key.
func WithAuthPrivateKeyPath(path string) Option {
	return func(c *Config) {
		c.AuthPrivateKeyPath = path
	}
}

// WithAuthClientID sets the auth client ID.
func WithAuthClientID(id string) Option {
	return func(c *Config) {
		c.AuthClientID = id
	}
}

// WithAuthPrivateKeyPEM sets the auth private key PEM content.
func WithAuthPrivateKeyPEM(pem string) Option {
	return func(c *Config) {
		c.AuthPrivateKeyPEM = pem
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
		VaultEnabled:      false,
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
