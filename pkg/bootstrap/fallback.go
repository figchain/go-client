package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/figchain/go-client/pkg/transport"
)

// FallbackStrategy implements bootstrapping from Server then S3 Backup if Server fails.
type FallbackStrategy struct {
	serverStrategy   Strategy
	s3BackupStrategy Strategy
}

// NewFallbackStrategy creates a new FallbackStrategy.
func NewFallbackStrategy(server Strategy, s3Backup Strategy) *FallbackStrategy {
	return &FallbackStrategy{
		serverStrategy:   server,
		s3BackupStrategy: s3Backup,
	}
}

// Bootstrap attempts to load from Server, falling back to S3 Backup on failure.
func (s *FallbackStrategy) Bootstrap(ctx context.Context, namespaces []string) (*Result, error) {
	// 1. Try Server
	result, serverErr := s.serverStrategy.Bootstrap(ctx, namespaces)
	if serverErr == nil {
		return result, nil
	}

	// A rejected credential is a configuration error, not an outage. Serving the
	// backup would hide it while every later update fails the same way.
	var statusErr *transport.StatusError
	if errors.As(serverErr, &statusErr) &&
		(statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden) {
		return nil, fmt.Errorf("server rejected the client credentials, not falling back to s3 backup: %w", serverErr)
	}

	log.Printf("WARN Server bootstrap failed: %v. Falling back to S3 Backup.", serverErr)

	// 2. Try S3 Backup
	result, s3BackupErr := s.s3BackupStrategy.Bootstrap(ctx, namespaces)
	if s3BackupErr != nil {
		return nil, fmt.Errorf("ERROR server bootstrap failed: %v; fallback to s3 backup also failed: %w", serverErr, s3BackupErr)
	}

	return result, nil
}

var _ Strategy = (*FallbackStrategy)(nil)
