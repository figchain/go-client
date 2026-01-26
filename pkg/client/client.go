package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hamba/avro/v2"

	"github.com/figchain/go-client/pkg/backup"
	"github.com/figchain/go-client/pkg/bootstrap"
	"github.com/figchain/go-client/pkg/config"
	"github.com/figchain/go-client/pkg/encryption"
	"github.com/figchain/go-client/pkg/evaluation"
	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/store"
	"github.com/figchain/go-client/pkg/transport"
)

// AvroRecord is an interface that provides the Avro schema.
type AvroRecord interface {
	Schema() avro.Schema
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
	schemas           map[string]string
	mu                sync.RWMutex
	wg                sync.WaitGroup
	closeCh           chan struct{}
	ctx               context.Context
	cancel            context.CancelFunc
}

// NewClientFromConfig creates a new Client from a JSON configuration file.
// Prioritizes Env vars over Config file, but Options over everything.
func NewClientFromConfig(path string, opts ...config.Option) (*Client, error) {
	// 1. Load Env/Defaults first
	cfg, err := config.LoadConfig("")
	if err != nil {
		return nil, fmt.Errorf("failed to load base config: %w", err)
	}

	// 2. Read JSON
	// We define a struct matching client-config.json
	type ClientConfig struct {
		Namespace            string   `json:"namespace"`
		Namespaces           []string `json:"namespaces"`
		CredentialID         string   `json:"credentialId"`
		AuthPrivateKey       string   `json:"authPrivateKey"`
		EncryptionPrivateKey string   `json:"encryptionPrivateKey"`
		EnvironmentID        string   `json:"environmentId"`
		TenantID             string   `json:"tenantId"`
		// Backup configuration is handled separately or via Env
		S3BackupEnabled         *bool  `json:"s3BackupEnabled"`
		S3BackupBucket          string `json:"s3BackupBucket"`
		S3BackupPrefix          string `json:"s3BackupPrefix"`
		S3BackupRegion          string `json:"s3BackupRegion"`
		S3BackupEndpoint        string `json:"s3BackupEndpoint"`
		S3BackupPathStyleAccess bool   `json:"s3BackupPathStyleAccess"`
	}

	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cc ClientConfig
	if err := json.Unmarshal(fileBytes, &cc); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 3. Apply JSON values as Defaults (fill if empty)
	// Note: LoadConfig("") might have left fields empty if not in Env or yaml.

	// Merge Namespaces
	nsMap := make(map[string]struct{})
	for _, ns := range cfg.Namespaces {
		nsMap[ns] = struct{}{}
	}
	if cc.Namespace != "" {
		nsMap[cc.Namespace] = struct{}{}
	}
	for _, ns := range cc.Namespaces {
		if ns != "" {
			nsMap[ns] = struct{}{}
		}
	}
	if len(nsMap) > 0 {
		cfg.Namespaces = make([]string, 0, len(nsMap))
		for ns := range nsMap {
			cfg.Namespaces = append(cfg.Namespaces, ns)
		}
	}

	if cfg.AuthCredentialID == "" && cc.CredentialID != "" {
		cfg.AuthCredentialID = cc.CredentialID
	}

	if cfg.AuthPrivateKey == "" && cc.AuthPrivateKey != "" {
		cfg.AuthPrivateKey = cc.AuthPrivateKey
	}

	if cfg.EncryptionPrivateKey == "" && cc.EncryptionPrivateKey != "" {
		cfg.EncryptionPrivateKey = cc.EncryptionPrivateKey
	}

	if cc.S3BackupEnabled != nil {
		cfg.S3BackupEnabled = *cc.S3BackupEnabled
	}
	if cc.S3BackupPathStyleAccess {
		cfg.S3BackupPathStyleAccess = true
	}
	if cc.S3BackupBucket != "" {
		cfg.S3BackupBucket = cc.S3BackupBucket
	}
	if cc.S3BackupPrefix != "" {
		cfg.S3BackupPrefix = cc.S3BackupPrefix
	}
	if cc.S3BackupRegion != "" {
		cfg.S3BackupRegion = cc.S3BackupRegion
	}
	if cc.S3BackupEndpoint != "" {
		cfg.S3BackupEndpoint = cc.S3BackupEndpoint
	}

	if cc.EnvironmentID != "" {
		cfg.EnvironmentID = cc.EnvironmentID
	}

	if cc.TenantID != "" {
		cfg.TenantID = cc.TenantID
	}

	// 4. Apply Options (Overrides everything)
	for _, opt := range opts {
		opt(cfg)
	}

	// 5. Create Client using the constructed config object
	log.Printf("DEBUG NewClientFromConfig: CredID=%s, TenantID=%s, EnvID=%s, Namespaces=%v, S3BackupEnabled=%v, PKLen=%d",
		cfg.AuthCredentialID, cfg.TenantID, cfg.EnvironmentID, cfg.Namespaces, cfg.S3BackupEnabled, len(cfg.AuthPrivateKey))
	// We use WithConfig to pass the fully populated configuration to New()
	return New(config.WithConfig(cfg))
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
	// Check for AuthPrivateKey (Hex)
	authKeyHex := cfg.AuthPrivateKey

	if cfg.ClientSecret == "" && authKeyHex == "" {
		return nil, fmt.Errorf("an authentication method must be configured. Please provide either a ClientSecret or AuthPrivateKey (Hex)")
	}

	var tokenProvider transport.TokenProvider

	if authKeyHex != "" {
		if len(cfg.Namespaces) > 1 {
			return nil, fmt.Errorf("private key authentication can only be used with a single namespace")
		}

		// Use EnvironmentID as placeholder if AuthClientID not set, but prefer AuthClientID
		serviceAccountID := cfg.EnvironmentID
		if cfg.AuthClientID != "" {
			serviceAccountID = cfg.AuthClientID
		} else if cfg.AuthCredentialID != "" {
			serviceAccountID = cfg.AuthCredentialID
		}

		// Use first namespace if available for auth token scope
		namespace := ""
		if len(cfg.Namespaces) > 0 {
			namespace = cfg.Namespaces[0]
		}

		var err error
		tokenProvider, err = transport.NewPrivateKeyTokenProvider(authKeyHex, serviceAccountID, cfg.TenantID, namespace, cfg.AuthCredentialID)
		if err != nil {
			return nil, fmt.Errorf("failed to create token provider: %w", err)
		}
	} else {
		tokenProvider = transport.NewSharedSecretTokenProvider(cfg.ClientSecret)
	}

	tr := transport.NewHTTPTransport(cfg.HTTPClient, cfg.BaseURL, tokenProvider, cfg.EnvironmentID)

	var encService *encryption.Service
	// Encryption requires a dedicated private key - no fallback to auth key
	encKeyHex := cfg.EncryptionPrivateKey
	if encKeyHex != "" {
		svc, err := encryption.NewService(tr, encKeyHex)
		if err != nil {
			return nil, fmt.Errorf("failed to create encryption service: %w", err)
		}
		encService = svc
	}

	if encService != nil && cfg.S3BackupEnabled {
		encService.S3BackupEnabled = true
		encService.S3BackupConfig = encryption.S3BackupConfig{
			Bucket:    cfg.S3BackupBucket,
			Prefix:    cfg.S3BackupPrefix,
			Region:    cfg.S3BackupRegion,
			Endpoint:  cfg.S3BackupEndpoint,
			PathStyle: cfg.S3BackupPathStyleAccess,
		}
		// Prefer CredentialID (targetId)
		encService.ClientID = cfg.AuthCredentialID
		if encService.ClientID == "" {
			encService.ClientID = cfg.AuthClientID
		}
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
		schemas:           make(map[string]string),
		closeCh:           make(chan struct{}),
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// Select Bootstrap Strategy
	var strategy bootstrap.Strategy
	serverStrategy := bootstrap.NewServerStrategy(tr, cfg.EnvironmentID, cfg.AsOfTimestamp)

	if cfg.S3BackupEnabled {
		vs, err := backup.NewDefaultS3BackupService(context.Background(), cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create s3 backup service: %w", err)
		}
		s3BackupStrategy := bootstrap.NewS3BackupStrategy(vs)

		switch cfg.BootstrapStrategy {
		case config.BootstrapStrategyS3BackupOnly:
			strategy = s3BackupStrategy
		case config.BootstrapStrategyHybrid:
			strategy = bootstrap.NewHybridStrategy(s3BackupStrategy, serverStrategy, tr, cfg.EnvironmentID)
		case config.BootstrapStrategyServerFirst, "":
			strategy = bootstrap.NewFallbackStrategy(serverStrategy, s3BackupStrategy)
		case config.BootstrapStrategyServer:
			strategy = serverStrategy
		default:
			log.Printf("WARN Unknown bootstrap strategy %q, using Default (ServerFirst with Fallback)", cfg.BootstrapStrategy)
			strategy = bootstrap.NewFallbackStrategy(serverStrategy, s3BackupStrategy)
		}
	} else {
		strategy = serverStrategy
	}

	log.Printf("INFO Bootstrapping with strategy: %T", strategy)

	// Execute Bootstrap
	result, err := strategy.Bootstrap(context.Background(), cfg.Namespaces)
	if err != nil {
		return nil, fmt.Errorf("bootstrap failed: %w", err)
	}

	// Populate Store
	for _, ff := range result.FigFamilies {
		c.store.Put(ff)
	}

	// Set Cursors and Schemas
	c.mu.Lock()
	maps.Copy(c.namespaceCursors, result.Cursors)
	maps.Copy(c.schemas, result.Schemas)
	c.mu.Unlock()

	// Start polling
	c.wg.Add(1)
	go c.pollLoop()

	return c, nil
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	close(c.closeCh)
	c.cancel()
	c.wg.Wait()
	return c.transport.Close()
}

// GetFig retrieves a configuration and deserializes it into target.
func (c *Client) GetFig(key string, target any, ctx *evaluation.EvaluationContext) error {
	payload, err := c.GetFigRaw(key, ctx)
	if err != nil {
		return err
	}

	// Deserialize Avro
	_, ok := target.(AvroRecord)
	if !ok {
		return fmt.Errorf("target must implement AvroRecord interface with Schema() avro.Schema method")
	}

	// Get the fig family to access the writer schema
	if len(c.cfg.Namespaces) == 0 {
		return fmt.Errorf("no namespaces configured")
	}
	namespace := c.cfg.Namespaces[0]
	figFamily, ok := c.store.Get(namespace, key)
	if !ok {
		return fmt.Errorf("fig not found: %s", key)
	}

	// Get the writer schema from the schemas map using SchemaURI as key
	c.mu.RLock()
	schemaContent, ok := c.schemas[figFamily.Definition.SchemaURI]
	c.mu.RUnlock()
	if !ok {
		// Fallback: Try to fetch the schema from the server on demand
		log.Printf("INFO Schema for URI %s not found in cache, fetching from server...", figFamily.Definition.SchemaURI)
		content, err := c.fetchSchemaByURI(figFamily.Definition.SchemaURI)
		if err != nil {
			return fmt.Errorf("schema not found for URI: %s and failed to fetch on-demand: %w", figFamily.Definition.SchemaURI, err)
		}
		c.mu.Lock()
		c.schemas[figFamily.Definition.SchemaURI] = content
		c.mu.Unlock()
		schemaContent = content
	}

	log.Printf("DEBUG SchemaContent for URI %s: %s", figFamily.Definition.SchemaURI, schemaContent)
	writerSchema, err := avro.Parse(schemaContent)
	if err != nil {
		return fmt.Errorf("failed to parse writer schema from SchemaContent: %w", err)
	}

	if err := avro.Unmarshal(writerSchema, payload, target); err != nil {
		return fmt.Errorf("failed to unmarshal avro with schema evolution: %w", err)
	}

	return nil
}

// GetFigRaw retrieves the raw (decrypted if necessary) payload for a specific key.
// This allows the caller to handle deserialization (e.g. using generated code directly).
func (c *Client) GetFigRaw(key string, ctx *evaluation.EvaluationContext) ([]byte, error) {
	// Assume single namespace for now or pick first
	if len(c.cfg.Namespaces) == 0 {
		return nil, fmt.Errorf("no namespaces configured")
	}
	namespace := c.cfg.Namespaces[0]

	figFamily, ok := c.store.Get(namespace, key)
	if !ok {
		return nil, fmt.Errorf("fig not found: %s", key)
	}

	fig, err := c.evaluator.Evaluate(figFamily, ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluation failed: %w", err)
	}
	if fig == nil {
		return nil, fmt.Errorf("no matching fig found for key: %s", key)
	}

	log.Printf("DEBUG GetFigRaw: key=%s, IsEncrypted=%v, PayloadLen=%d", key, fig.IsEncrypted, len(fig.Payload))

	// Decrypt
	payload := fig.Payload
	if fig.IsEncrypted {
		if c.encryptionService == nil {
			return nil, fmt.Errorf("received encrypted fig for key '%s' but client is not configured for decryption", key)
		}
		p, err := c.encryptionService.Decrypt(ctx, fig, namespace)
		if err != nil {
			log.Printf("Failed to decrypt fig with key '%s' in namespace '%s': %v", key, namespace, err)
			return nil, fmt.Errorf("failed to decrypt fig with key '%s' in namespace '%s': %w", key, namespace, err)
		}
		payload = p
	}

	return payload, nil
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
		resp, err := c.transport.FetchUpdate(c.ctx, req)
		if err != nil {
			if c.ctx.Err() == context.Canceled {
				// Shutdown in progress, exit quietly
				return
			}
			log.Printf("Failed to fetch updates for %s: %v", ns, err)
			// Prevent tight loop on error (backoff)
			select {
			case <-c.closeCh:
				return
			case <-time.After(c.cfg.PollingInterval):
				continue
			}
		}

		// Collect notifications
		var listenerNotifications []func()
		var watcherNotifications []func()

		if len(resp.FigFamilies) > 0 {
			c.mu.Lock()
			for _, ff := range resp.FigFamilies {
				c.store.Put(ff)

				// Collect listener notifications
				if callbacks, ok := c.listeners[ff.Definition.Key]; ok {
					for _, cb := range callbacks {
						// Capture ff
						ffCopy := ff
						listenerNotifications = append(listenerNotifications, func() { cb(ffCopy) })
					}
				}

				// Collect watcher notifications
				if chans, ok := c.watchers[ff.Definition.Key]; ok {
					for _, ch := range chans {
						// Capture ff
						ffCopy := ff
						watcherNotifications = append(watcherNotifications, func() {
							select {
							case ch <- ffCopy:
							default:
								// Drop update if channel is full
							}
						})
					}
				}
			}
			c.mu.Unlock()
		}

		if resp.Cursor != "" || len(resp.Schemas) > 0 {
			c.mu.Lock()
			if resp.Cursor != "" {
				c.namespaceCursors[ns] = resp.Cursor
			}
			if len(resp.Schemas) > 0 {
				for k, v := range resp.Schemas {
					c.schemas[k] = v
				}
			}
			c.mu.Unlock()
		}

		if len(resp.FigFamilies) > 0 {
			// Notify after updating schemas and cursors
			for _, notify := range listenerNotifications {
				notify()
			}
			for _, notify := range watcherNotifications {
				notify()
			}
		}
	}
}

func (c *Client) fetchSchemaByURI(schemaURI string) (string, error) {
	parsedURI, err := url.Parse(schemaURI)
	if err != nil {
		return "", fmt.Errorf("invalid schema URI: %w", err)
	}

	// URI format: tag:figchain.io,2025:namespace:schemaName:version
	// We expect the scheme to be "tag"
	if parsedURI.Scheme != "tag" {
		return "", fmt.Errorf("unsupported schema URI scheme: %s", parsedURI.Scheme)
	}

	parts := strings.Split(parsedURI.Opaque, ":")
	if len(parts) < 4 {
		return "", fmt.Errorf("invalid tag URI format for schema: %s", schemaURI)
	}

	// parts[0] is "figchain.io,2025"
	namespace, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid namespace in schema URI: %w", err)
	}
	schemaName, err := url.PathUnescape(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid schema name in schema URI: %w", err)
	}
	version, err := strconv.Atoi(parts[3])
	if err != nil {
		return "", fmt.Errorf("invalid schema version in URI: %s", parts[3])
	}

	return c.transport.FetchSchema(c.ctx, namespace, schemaName, version)
}

// RegisterListener registers a callback for updates to a specific key.
// The callback is invoked with the deserialized object when an update occurs.
//
// IMPORTANT: This feature should be used for SERVER-SCOPED configuration only (e.g. global flags).
// The update is evaluated with an empty context. If your rules depend on user-specific attributes
// (like request-scoped context), this listener may receive default values or fail to match rules.
// For request-scoped configuration, use GetFig() with the appropriate context when needed.
func (c *Client) RegisterListener(key string, prototype AvroRecord, callback func(AvroRecord)) {
	rawCallback := func(schemaURI string, payload []byte) {
		log.Printf("DEBUG rawCallback called with schemaURI %s, payload len %d", schemaURI, len(payload))
		// Create new instance of prototype type using reflection
		// prototype should be a pointer to a struct
		t := reflect.TypeOf(prototype)
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		targetVal := reflect.New(t)
		target := targetVal.Interface()

		// Get the writer schema from the schemas map
		c.mu.RLock()
		schemaContent, ok := c.schemas[schemaURI]
		c.mu.RUnlock()
		if !ok {
			// Fallback: Try to fetch on demand
			log.Printf("INFO Listener schema for URI %s not found in cache, fetching from server...", schemaURI)
			content, err := c.fetchSchemaByURI(schemaURI)
			if err != nil {
				log.Printf("ERROR Listener schema not found for URI: %s and failed to fetch: %v", schemaURI, err)
				return
			}
			c.mu.Lock()
			c.schemas[schemaURI] = content
			c.mu.Unlock()
			schemaContent = content
		}

		writerSchema, err := avro.Parse(schemaContent)
		if err != nil {
			log.Printf("ERROR Listener writer schema parse failed for %s: %v", key, err)
			return
		}

		if err := avro.Unmarshal(writerSchema, payload, target); err != nil {
			log.Printf("ERROR Listener unmarshal failed for %s: %v", key, err)
			return
		}

		// Callback with the new object (cast back to interface)
		if record, ok := target.(AvroRecord); ok {
			callback(record)
		} else {
			log.Printf("ERROR Listener callback failed for key %s: created object of type %T does not implement AvroRecord", key, target)
		}
	}

	c.RegisterRawListener(key, rawCallback)
}

// RegisterRawListener registers a callback for updates to a specific key, returning the raw payload.
// This allows the caller to handle deserialization.
//
// IMPORTANT: This feature should be used for SERVER-SCOPED configuration only (e.g. global flags).
// The update is evaluated with an empty context. If your rules depend on user-specific attributes
// (like request-scoped context), this listener may receive default values or fail to match rules.
// For request-scoped configuration, use GetFig() with the appropriate context when needed.
func (c *Client) RegisterRawListener(key string, callback func(string, []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// We create a wrapper func that handles the logic
	wrapper := func(ff model.FigFamily) {
		log.Printf("DEBUG Listener wrapper called for key %s, fig key %s", key, ff.Definition.Key)
		// Empty evaluation context (embeds context.Background())
		ctx := evaluation.NewEvaluationContext(nil)
		fig, err := c.evaluator.Evaluate(&ff, ctx)
		if err != nil || fig == nil {
			log.Printf("ERROR Listener evaluation failed for %s: %v", key, err)
			return
		}

		payload := fig.Payload
		if fig.IsEncrypted {
			if c.encryptionService == nil {
				log.Printf("ERROR Listener received encrypted fig for key '%s' but client is not configured for decryption", key)
				return
			}
			// Use the evaluation context (which implements context.Context)
			p, err := c.encryptionService.Decrypt(ctx, fig, ff.Definition.Namespace)
			if err != nil {
				log.Printf("ERROR Listener decryption failed for %s: %v", key, err)
				return
			}
			payload = p
		}

		log.Printf("DEBUG Listener calling callback for key %s", key)
		callback(ff.Definition.SchemaURI, payload)
	}

	c.listeners[key] = append(c.listeners[key], wrapper)
}
