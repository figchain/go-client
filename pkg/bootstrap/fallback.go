package bootstrap

import (
	"context"
	"fmt"
	"log"
)

// FallbackStrategy implements bootstrapping from Server then Vault if Server fails.
type FallbackStrategy struct {
	serverStrategy Strategy
	vaultStrategy  Strategy
}

// NewFallbackStrategy creates a new FallbackStrategy.
func NewFallbackStrategy(server Strategy, vault Strategy) *FallbackStrategy {
	return &FallbackStrategy{
		serverStrategy: server,
		vaultStrategy:  vault,
	}
}

// Bootstrap attempts to load from Server, falling back to Vault on failure.
func (s *FallbackStrategy) Bootstrap(ctx context.Context, namespaces []string) (*Result, error) {
	// 1. Try Server
	result, err := s.serverStrategy.Bootstrap(ctx, namespaces)
	if err == nil {
		return result, nil
	}

	log.Printf("Server bootstrap failed: %v. Falling back to Vault.", err)

	// 2. Try Vault
	result, err = s.vaultStrategy.Bootstrap(ctx, namespaces)
	if err != nil {
		return nil, fmt.Errorf("both server and vault bootstrap failed: %w", err)
	}

	return result, nil
}
