package bootstrap

import (
	"context"
	"fmt"
	"log"
	"maps"

	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/transport"
)

// HybridStrategy implements bootstrapping from Vault then Server.
type HybridStrategy struct {
	vaultStrategy  Strategy
	serverStrategy Strategy
	transport      transport.Transport
	environmentID  string
}

// NewHybridStrategy creates a new HybridStrategy.
func NewHybridStrategy(vault Strategy, server Strategy, tr transport.Transport, environmentID string) *HybridStrategy {
	return &HybridStrategy{
		vaultStrategy:  vault,
		serverStrategy: server,
		transport:      tr,
		environmentID:  environmentID,
	}
}

// Bootstrap loads from Vault, identifies missing namespaces, fetches them from Server, and catches up.
func (s *HybridStrategy) Bootstrap(ctx context.Context, namespaces []string) (*Result, error) {
	// 1. Load from Vault
	vaultResult, err := s.vaultStrategy.Bootstrap(ctx, namespaces)
	if err != nil {
		log.Printf("Vault bootstrap failed: %v. Falling back to full server fetch.", err)
		// Fallback to full server fetch? Or fail?
		// Java client seems to proceed or allow fallback logic.
		// If explicit "Hybrid", maybe we should try server for everything?
		// For now, let's treat Vault failure as "no data from vault"
		vaultResult = &Result{}
	}

	var allFamilies []model.FigFamily
	if vaultResult.FigFamilies != nil {
		allFamilies = append(allFamilies, vaultResult.FigFamilies...)
	}

	finalCursors := make(map[string]string)
	if vaultResult.Cursors != nil {
		for k, v := range vaultResult.Cursors {
			finalCursors[k] = v
		}
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
		log.Printf("Fetching missing namespaces from server: %v", missingNamespaces)
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

	// 4. Catch up from Server for namespaces that WERE in Vault
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

		// It was in vault (or potentially missing but not fetched? No, missingNamespaces handles that)
		// If it was in vault, it is in finalCursors.
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
			return nil, fmt.Errorf("failed to catch up for %s: %w", ns, err)
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
