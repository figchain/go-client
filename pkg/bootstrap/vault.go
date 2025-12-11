package bootstrap

import (
	"context"

	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/vault"
)

// VaultStrategy implements bootstrapping from the FigChain Vault (S3).
type VaultStrategy struct {
	vaultService *vault.VaultService
}

// NewVaultStrategy creates a new VaultStrategy.
func NewVaultStrategy(vs *vault.VaultService) *VaultStrategy {
	return &VaultStrategy{vaultService: vs}
}

// Bootstrap loads data from the Vault.
func (s *VaultStrategy) Bootstrap(ctx context.Context, namespaces []string) (*Result, error) {
	payload, err := s.vaultService.LoadBackup(ctx)
	if err != nil {
		return nil, err
	}

	cursors := make(map[string]string)
	if payload.SyncToken != "" {
		for _, ns := range namespaces {
			cursors[ns] = payload.SyncToken
		}
	}

	for _, item := range payload.Items {
		ns := item.Definition.Namespace
		if _, ok := cursors[ns]; !ok {
			cursors[ns] = payload.SyncToken
		}
	}

	// Filter Items to relevant namespaces
	requestedNamespaces := make(map[string]struct{}, len(namespaces))
	for _, ns := range namespaces {
		requestedNamespaces[ns] = struct{}{}
	}

	filteredFamilies := make([]model.FigFamily, 0)
	for _, item := range payload.Items {
		if _, ok := requestedNamespaces[item.Definition.Namespace]; ok {
			filteredFamilies = append(filteredFamilies, item)
		}
	}

	return &Result{
		FigFamilies: filteredFamilies,
		Cursors:     cursors,
	}, nil
}
