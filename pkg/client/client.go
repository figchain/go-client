package client

import (
	"context"
	"fmt"
	"log"
	"maps"
	"sync"
	"time"

	"github.com/hamba/avro/v2"

	"github.com/figchain/go-client/pkg/config"
	"github.com/figchain/go-client/pkg/evaluation"
	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/store"
	"github.com/figchain/go-client/pkg/transport"
)

// AvroRecord is an interface that provides the Avro schema.
type AvroRecord interface {
	Schema() string
}

// Client is the main entry point for the FigChain client.
type Client struct {
	cfg              *config.Config
	store            store.Store
	evaluator        evaluation.Evaluator
	transport        transport.Transport
	namespaceCursors map[string]string
	watchers         map[string][]chan model.FigFamily
	mu               sync.RWMutex
	wg               sync.WaitGroup
	closeCh          chan struct{}
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

	tr := transport.NewHTTPTransport(cfg.HTTPClient, cfg.BaseURL, cfg.ClientSecret, cfg.EnvironmentID)

	c := &Client{
		cfg:              cfg,
		store:            store.NewMemoryStore(),
		evaluator:        evaluation.NewRuleBasedEvaluator(),
		transport:        tr,
		namespaceCursors: make(map[string]string),
		watchers:         make(map[string][]chan model.FigFamily),
		closeCh:          make(chan struct{}),
	}

	// Initial fetch
	if err := c.fetchInitial(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to fetch initial data: %w", err)
	}

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

	// Deserialize Avro
	record, ok := target.(AvroRecord)
	if !ok {
		return fmt.Errorf("target must implement AvroRecord interface with Schema() string method")
	}

	schema, err := avro.Parse(record.Schema())
	if err != nil {
		return fmt.Errorf("failed to parse schema from target: %w", err)
	}

	if err := avro.Unmarshal(schema, fig.Payload, target); err != nil {
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

func (c *Client) fetchInitial(ctx context.Context) error {
	for _, ns := range c.cfg.Namespaces {
		req := &model.InitialFetchRequest{
			Namespace:     ns,
			EnvironmentID: c.cfg.EnvironmentID,
		}
		if c.cfg.AsOfTimestamp != "" {
			t, err := time.Parse(time.RFC3339, c.cfg.AsOfTimestamp)
			if err == nil {
				req.AsOfTimestamp = &t
			}
		}

		resp, err := c.transport.FetchInitial(ctx, req)
		if err != nil {
			return err
		}

		for _, ff := range resp.FigFamilies {
			c.store.Put(ff)
		}
		c.mu.Lock()
		c.namespaceCursors[ns] = resp.Cursor
		c.mu.Unlock()
	}
	return nil
}

func (c *Client) pollLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.cfg.PollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ticker.C:
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
			continue
		}

		if len(resp.FigFamilies) > 0 {
			c.mu.Lock()
			for _, ff := range resp.FigFamilies {
				c.store.Put(ff)
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
