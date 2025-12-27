package config

import (
	"net/http"
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
	AuthClientID             string `mapstructure:"auth_client_id"`
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

// WithAuthClientID sets the auth client ID.
func WithAuthClientID(id string) Option {
	return func(c *Config) {
		c.AuthClientID = id
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
