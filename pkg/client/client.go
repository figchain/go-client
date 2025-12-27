package client

import (
	"context"
	"fmt"
	"log"
	"maps"
	"reflect"
	"sync"
	"time"

	"github.com/hamba/avro/v2"

	"github.com/figchain/go-client/pkg/bootstrap"
	"github.com/figchain/go-client/pkg/config"
	"github.com/figchain/go-client/pkg/encryption"
	"github.com/figchain/go-client/pkg/evaluation"
	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/store"
	"github.com/figchain/go-client/pkg/transport"
	"github.com/figchain/go-client/pkg/util"
	"github.com/figchain/go-client/pkg/vault"
)

// AvroRecord is an interface that provides the Avro schema.
type AvroRecord interface {
	Schema() string
}

// Client is the main entry point for the FigChain client.
type Client struct {
	cfg               *config.Config
	store             store.Store
	evaluator         evaluation.Evaluator
	transport         transport.Transport
	namespaceCursors  map[string]string
	watchers          map[string][]chan model.FigFamily
	listeners         map[string][]func(model.FigFamily)
	encryptionService *encryption.Service
	mu                sync.RWMutex
	wg                sync.WaitGroup
	closeCh           chan struct{}
}

// New creates a new Client.
func New(opts ...config.Option) (*Client, error) {
	cfg := config.DefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("BaseURL is required")
	}
	if cfg.EnvironmentID == "" {
		return nil, fmt.Errorf("EnvironmentID is required")
	}
	if cfg.ClientSecret == "" && cfg.AuthPrivateKeyPath == "" {
		return nil, fmt.Errorf("an authentication method must be configured. Please provide either a ClientSecret or an AuthPrivateKeyPath")
	}

	var tokenProvider transport.TokenProvider
	if cfg.AuthPrivateKeyPath != "" {
		if len(cfg.Namespaces) > 1 {
			return nil, fmt.Errorf("private key authentication can only be used with a single namespace")
		}
		pk, err := util.LoadRSAPrivateKey(cfg.AuthPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load auth private key: %w", err)
		}

		// Use EnvironmentID as placeholder if AuthClientID not set, but prefer AuthClientID
		serviceAccountID := cfg.EnvironmentID
		if cfg.AuthClientID != "" {
			serviceAccountID = cfg.AuthClientID
		}

		// Use first namespace if available for auth token scope
		namespace := ""
		if len(cfg.Namespaces) > 0 {
			namespace = cfg.Namespaces[0]
		}
		tokenProvider = transport.NewPrivateKeyTokenProvider(pk, serviceAccountID, cfg.TenantID, namespace, "")
	} else {
		tokenProvider = transport.NewSharedSecretTokenProvider(cfg.ClientSecret)
	}

	tr := transport.NewHTTPTransport(cfg.HTTPClient, cfg.BaseURL, tokenProvider, cfg.EnvironmentID)

	var encService *encryption.Service
	if cfg.EncryptionPrivateKeyPath != "" {
		svc, err := encryption.NewService(tr, cfg.EncryptionPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create encryption service: %w", err)
		}
		encService = svc
	}

	c := &Client{
		cfg:               cfg,
		store:             store.NewMemoryStore(),
		evaluator:         evaluation.NewRuleBasedEvaluator(),
		transport:         tr,
		encryptionService: encService,
		namespaceCursors:  make(map[string]string),
		watchers:          make(map[string][]chan model.FigFamily),
		listeners:         make(map[string][]func(model.FigFamily)),
		closeCh:           make(chan struct{}),
	}

	// Select Bootstrap Strategy
	var strategy bootstrap.Strategy
	serverStrategy := bootstrap.NewServerStrategy(tr, cfg.EnvironmentID, cfg.AsOfTimestamp)

	if cfg.VaultEnabled {
		vs, err := vault.NewDefaultVaultService(context.Background(), cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create vault service: %w", err)
		}
		vaultStrategy := bootstrap.NewVaultStrategy(vs)

		switch cfg.BootstrapStrategy {
		case config.BootstrapStrategyVault:
			strategy = vaultStrategy
		case config.BootstrapStrategyHybrid:
			strategy = bootstrap.NewHybridStrategy(vaultStrategy, serverStrategy, tr, cfg.EnvironmentID)
		case config.BootstrapStrategyServerFirst, "":
			strategy = bootstrap.NewFallbackStrategy(serverStrategy, vaultStrategy)
		case config.BootstrapStrategyServer:
			strategy = serverStrategy
		default:
			log.Printf("Unknown bootstrap strategy %q, using Default (ServerFirst with Fallback)", cfg.BootstrapStrategy)
			strategy = bootstrap.NewFallbackStrategy(serverStrategy, vaultStrategy)
		}
	} else {
		strategy = serverStrategy
	}

	log.Printf("Bootstrapping with strategy: %T", strategy)

	// Execute Bootstrap
	result, err := strategy.Bootstrap(context.Background(), cfg.Namespaces)
	if err != nil {
		return nil, fmt.Errorf("bootstrap failed: %w", err)
	}

	// Populate Store
	for _, ff := range result.FigFamilies {
		c.store.Put(ff)
	}

	// Set Cursors
	c.mu.Lock()
	for ns, cursor := range result.Cursors {
		c.namespaceCursors[ns] = cursor
	}
	c.mu.Unlock()

	// Start polling
	c.wg.Add(1)
	go c.pollLoop()

	return c, nil
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	close(c.closeCh)
	c.wg.Wait()
	return c.transport.Close()
}

// GetFig retrieves a configuration and deserializes it into target.
func (c *Client) GetFig(key string, target any, ctx *evaluation.EvaluationContext) error {
	// Assume single namespace for now or pick first
	if len(c.cfg.Namespaces) == 0 {
		return fmt.Errorf("no namespaces configured")
	}
	namespace := c.cfg.Namespaces[0]

	figFamily, ok := c.store.Get(namespace, key)
	if !ok {
		return fmt.Errorf("fig not found: %s", key)
	}

	fig, err := c.evaluator.Evaluate(figFamily, ctx)
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}
	if fig == nil {
		return fmt.Errorf("no matching fig found for key: %s", key)
	}

	log.Printf("DEBUG GetFig: key=%s, IsEncrypted=%v, PayloadLen=%d", key, fig.IsEncrypted, len(fig.Payload))

	// Decrypt
	payload := fig.Payload
	if c.encryptionService != nil && fig.IsEncrypted {
		p, err := c.encryptionService.Decrypt(context.Background(), fig, namespace)
		if err != nil {
			log.Printf("Failed to decrypt fig with key '%s' in namespace '%s': %v", key, namespace, err)
			return nil // Return nil instead of error for resilience
		}
		payload = p
	}

	// Deserialize Avro
	record, ok := target.(AvroRecord)
	if !ok {
		return fmt.Errorf("target must implement AvroRecord interface with Schema() string method")
	}

	schema, err := avro.Parse(record.Schema())
	if err != nil {
		return fmt.Errorf("failed to parse schema from target: %w", err)
	}

	if err := avro.Unmarshal(schema, payload, target); err != nil {
		return fmt.Errorf("failed to unmarshal avro: %w", err)
	}

	return nil
}

// Watch returns a channel that receives updates for a specific key.
func (c *Client) Watch(ctx context.Context, key string) <-chan model.FigFamily {
	ch := make(chan model.FigFamily, 1)
	c.mu.Lock()
	c.watchers[key] = append(c.watchers[key], ch)
	c.mu.Unlock()

	go func() {
		<-ctx.Done()
		c.mu.Lock()
		defer c.mu.Unlock()
		// Remove channel from watchers
		if chans, ok := c.watchers[key]; ok {
			for i, listener := range chans {
				if listener == ch {
					c.watchers[key] = append(chans[:i], chans[i+1:]...)
					break
				}
			}
		}
		close(ch)
	}()

	return ch
}
func (c *Client) pollLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.closeCh:
			return
		default:
			// Perform long poll
			c.pollUpdates()
		}
	}
}

func (c *Client) pollUpdates() {
	c.mu.RLock()
	cursors := make(map[string]string)
	maps.Copy(cursors, c.namespaceCursors)
	c.mu.RUnlock()

	for ns, cursor := range cursors {
		req := &model.UpdateFetchRequest{
			Namespace:     ns,
			Cursor:        cursor,
			EnvironmentID: c.cfg.EnvironmentID,
		}
		resp, err := c.transport.FetchUpdate(context.Background(), req)
		if err != nil {
			log.Printf("Failed to fetch updates for %s: %v", ns, err)
			// Prevent tight loop on error (backoff)
			select {
			case <-c.closeCh:
				return
			case <-time.After(c.cfg.PollingInterval):
				continue
			}
		}

		if len(resp.FigFamilies) > 0 {
			c.mu.Lock()
			for _, ff := range resp.FigFamilies {
				c.store.Put(ff)

				// Notify type-specific listeners
				if callbacks, ok := c.listeners[ff.Definition.Key]; ok {
					for _, cb := range callbacks {
						cb(ff)
					}
				}

				// Notify watchers
				if chans, ok := c.watchers[ff.Definition.Key]; ok {
					for _, ch := range chans {
						select {
						case ch <- ff:
						default:
							// Drop update if channel is full
						}
					}
				}
			}
			c.mu.Unlock()
		}

		if resp.Cursor != "" {
			c.mu.Lock()
			c.namespaceCursors[ns] = resp.Cursor
			c.mu.Unlock()
		}
	}
}

// RegisterListener registers a callback for updates to a specific key.
// The callback is invoked with the deserialized object when an update occurs.
//
// IMPORTANT: This feature should be used for SERVER-SCOPED configuration only (e.g. global flags).
// The update is evaluated with an empty context. If your rules depend on user-specific attributes
// (like request-scoped context), this listener may receive default values or fail to match rules.
// For request-scoped configuration, use GetFig() with the appropriate context when needed.
func (c *Client) RegisterListener(key string, prototype AvroRecord, callback func(AvroRecord)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// We create a wrapper func that handles the logic
	wrapper := func(ff model.FigFamily) {
		// Empty context
		ctx := evaluation.NewEvaluationContext(nil)
		fig, err := c.evaluator.Evaluate(&ff, ctx)
		if err != nil || fig == nil {
			log.Printf("Listener evaluation failed for %s: %v", key, err)
			return
		}

		// Create new instance of prototype type using reflection
		// prototype should be a pointer to a struct
		t := reflect.TypeOf(prototype)
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		targetVal := reflect.New(t)
		target := targetVal.Interface()

		schema, err := avro.Parse(prototype.Schema())
		if err != nil {
			log.Printf("Listener schema parse failed for %s: %v", key, err)
			return
		}

		payload := fig.Payload
		if c.encryptionService != nil && fig.IsEncrypted {
			p, err := c.encryptionService.Decrypt(context.Background(), fig, ff.Definition.Namespace)
			if err != nil {
				log.Printf("Listener decryption failed for %s: %v", key, err)
				return
			}
			payload = p
		}

		if err := avro.Unmarshal(schema, payload, target); err != nil {
			log.Printf("Listener unmarshal failed for %s: %v", key, err)
			return
		}

		// Callback with the new object (cast back to interface)
		if record, ok := target.(AvroRecord); ok {
			callback(record)
		} else {
			log.Printf("Listener callback failed for key %s: created object of type %T does not implement AvroRecord", key, target)
		}
	}

	c.listeners[key] = append(c.listeners[key], wrapper)
}
