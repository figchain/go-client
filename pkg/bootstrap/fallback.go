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
	result, serverErr := s.serverStrategy.Bootstrap(ctx, namespaces)
	if serverErr == nil {
		return result, nil
	}

	log.Printf("Server bootstrap failed: %v. Falling back to Vault.", serverErr)

	// 2. Try Vault
	result, vaultErr := s.vaultStrategy.Bootstrap(ctx, namespaces)
	if vaultErr != nil {
		return nil, fmt.Errorf("server bootstrap failed: %v; fallback to vault also failed: %w", serverErr, vaultErr)
	}

	return result, nil
}
