package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/transport"
)

// ServerStrategy implements bootstrapping from the FigChain API.
type ServerStrategy struct {
	transport     transport.Transport
	environmentID string
	asOf          string
}

// NewServerStrategy creates a new ServerStrategy.
func NewServerStrategy(tr transport.Transport, environmentID string, asOf string) *ServerStrategy {
	return &ServerStrategy{
		transport:     tr,
		environmentID: environmentID,
		asOf:          asOf,
	}
}

// Bootstrap fetches initial data from the server.
func (s *ServerStrategy) Bootstrap(ctx context.Context, namespaces []string) (*Result, error) {
	var allFamilies []model.FigFamily
	cursors := make(map[string]string)

	for _, ns := range namespaces {
		req := &model.InitialFetchRequest{
			Namespace:     ns,
			EnvironmentID: s.environmentID,
		}
		if s.asOf != "" {
			t, err := time.Parse(time.RFC3339, s.asOf)
			if err == nil {
				req.AsOfTimestamp = &t
			}
		}

		resp, err := s.transport.FetchInitial(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch initial data for %s: %w", ns, err)
		}

		allFamilies = append(allFamilies, resp.FigFamilies...)
		if resp.Cursor != "" {
			cursors[ns] = resp.Cursor
		}
	}

	return &Result{
		FigFamilies: allFamilies,
		Cursors:     cursors,
	}, nil
}
