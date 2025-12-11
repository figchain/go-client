package bootstrap

import (
	"context"
	"log"

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

	// 3. Fetch full initial data for missing namespaces
	if len(missingNamespaces) > 0 {
		log.Printf("Fetching missing namespaces from server: %v", missingNamespaces)
		serverResult, err := s.serverStrategy.Bootstrap(ctx, missingNamespaces)
		if err != nil {
			log.Printf("Server bootstrap failed for missing namespaces: %v", err)
			// Partial failure
		} else {
			if serverResult.FigFamilies != nil {
				allFamilies = append(allFamilies, serverResult.FigFamilies...)
			}
			if serverResult.Cursors != nil {
				for k, v := range serverResult.Cursors {
					finalCursors[k] = v
				}
			}
		}
	}

	// 4. Catch up from Server for namespaces that WERE in Vault
	for _, ns := range namespaces {
		cursor, wasInVault := finalCursors[ns]
		if wasInVault {
			// Check if it was ONLY in vault (meaning it wasn't just fetched from server)
			// Actually, if it's in finalCursors, we need to know if it needs catch-up.
			// Namespaces from serverStrategy are fresh.
			// Namespaces from vaultStrategy are stale.

			// We can check if it was in missingNamespaces to skip
			isFresh := false
			for _, missing := range missingNamespaces {
				if missing == ns {
					isFresh = true
					break
				}
			}

			if !isFresh {
				// Needs catch-up
				req := &model.UpdateFetchRequest{
					Namespace:     ns,
					Cursor:        cursor,
					EnvironmentID: s.environmentID,
				}
				resp, err := s.transport.FetchUpdate(ctx, req)
				if err != nil {
					log.Printf("Failed to catch up for %s: %v", ns, err)
					continue
				}

				if len(resp.FigFamilies) > 0 {
					allFamilies = append(allFamilies, resp.FigFamilies...)
				}
				if resp.Cursor != "" {
					finalCursors[ns] = resp.Cursor
				}
			}
		}
	}

	return &Result{
		FigFamilies: allFamilies,
		Cursors:     finalCursors,
	}, nil
}
