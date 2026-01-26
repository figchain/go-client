package bootstrap

import (
	"context"
	"fmt"
	"log"
	"maps"

	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/transport"
)

// HybridStrategy implements bootstrapping from S3Backup then Server.
type HybridStrategy struct {
	backupStrategy  Strategy
	serverStrategy Strategy
	transport      transport.Transport
	environmentID  string
}

// NewHybridStrategy creates a new HybridStrategy.
func NewHybridStrategy(backup Strategy, server Strategy, tr transport.Transport, environmentID string) *HybridStrategy {
	return &HybridStrategy{
		backupStrategy:  backup,
		serverStrategy: server,
		transport:      tr,
		environmentID:  environmentID,
	}
}

// Bootstrap loads from S3Backup, identifies missing namespaces, fetches them from Server, and catches up.
func (s *HybridStrategy) Bootstrap(ctx context.Context, namespaces []string) (*Result, error) {
	// 1. Load from S3Backup
	backupResult, err := s.backupStrategy.Bootstrap(ctx, namespaces)
	if err != nil {
		log.Printf("WARN S3Backup bootstrap failed: %v. Falling back to full server fetch.", err)
		backupResult = &Result{}
	}

	var allFamilies []model.FigFamily
	if backupResult.FigFamilies != nil {
		allFamilies = append(allFamilies, backupResult.FigFamilies...)
	}

	finalCursors := make(map[string]string)
	if backupResult.Cursors != nil {
		maps.Copy(finalCursors, backupResult.Cursors)
	}

	// 2. Identify missing namespaces
	var missingNamespaces []string
	for _, ns := range namespaces {
		if _, ok := finalCursors[ns]; !ok {
			missingNamespaces = append(missingNamespaces, ns)
		}
	}

	// 3. Fetch missing from Server
	if len(missingNamespaces) > 0 {
		log.Printf("INFO Fetching missing namespaces from server: %v", missingNamespaces)
		serverResult, err := s.serverStrategy.Bootstrap(ctx, missingNamespaces)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch missing namespaces from server: %w", err)
		}
		if serverResult.FigFamilies != nil {
			allFamilies = append(allFamilies, serverResult.FigFamilies...)
		}
		if serverResult.Cursors != nil {
			maps.Copy(finalCursors, serverResult.Cursors)
		}
	}

	// 4. Catch up from Server for namespaces that WERE in S3Backup
	// Create a set of missing namespaces for O(1) lookup
	missingMap := make(map[string]struct{}, len(missingNamespaces))
	for _, ns := range missingNamespaces {
		missingMap[ns] = struct{}{}
	}

	// 4. Catch up
	for _, ns := range namespaces {
		_, isFresh := missingMap[ns]
		if isFresh {
			// Just fetched from server, so it's fresh
			continue
		}

		// It was in backup (or potentially missing but not fetched? No, missingNamespaces handles that)
		// If it was in backup, it is in finalCursors.
		cursor, ok := finalCursors[ns]
		if !ok {
			// Should have been missingNamespaces if not in finalCursors
			continue
		}

		req := &model.UpdateFetchRequest{
			Namespace:     ns,
			Cursor:        cursor,
			EnvironmentID: s.environmentID,
		}
		resp, err := s.transport.FetchUpdate(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("WARN failed to catch up for %s: %w", ns, err)
		}

		if len(resp.FigFamilies) > 0 {
			allFamilies = append(allFamilies, resp.FigFamilies...)
		}
		if resp.Cursor != "" {
			finalCursors[ns] = resp.Cursor
		}
	}

	return &Result{
		FigFamilies: allFamilies,
		Cursors:     finalCursors,
	}, nil
}

func (s *HybridStrategy) String() string {
	return "HybridStrategy"
}

var _ Strategy = (*HybridStrategy)(nil)
