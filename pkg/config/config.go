package config

import (
	"net/http"
	"time"
)

// Config holds the client configuration.
type Config struct {
	BaseURL         string
	LongPollingURL  string
	EnvironmentID   string
	PollingInterval time.Duration
	MaxRetries      int
	RetryDelay      time.Duration
	AsOfTimestamp   string
	Namespaces      []string
	HTTPClient      *http.Client
	ClientSecret    string
	UseLongPolling  bool
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

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		BaseURL:         "https://app.figchain.io/api/",
		PollingInterval: 60 * time.Second,
		MaxRetries:      3,
		RetryDelay:      1 * time.Second,
		HTTPClient:      http.DefaultClient,
		UseLongPolling:  true,
	}
}
