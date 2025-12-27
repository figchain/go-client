package bootstrap

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/transport"
)

// ServerStrategy implements bootstrapping from the FigChain API.
type ServerStrategy struct {
	transport     transport.Transport
	environmentID string
	asOf          *time.Time
}

// NewServerStrategy creates a new ServerStrategy.
func NewServerStrategy(tr transport.Transport, environmentID string, asOf string) *ServerStrategy {
	var asOfTime *time.Time
	if asOf != "" {
		t, err := time.Parse(time.RFC3339, asOf)
		if err == nil {
			asOfTime = &t
		} else {
			log.Printf("Invalid AsOfTimestamp format ignored: %s", asOf)
		}
	}
	return &ServerStrategy{
		transport:     tr,
		environmentID: environmentID,
		asOf:          asOfTime,
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
			AsOfTimestamp: s.asOf,
		}

		resp, err := s.transport.FetchInitial(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch initial data for %s: %w", ns, err)
		}

		allFamilies = append(allFamilies, resp.FigFamilies...)
		if resp.Cursor != "" {
			cursors[ns] = resp.Cursor
		}
		log.Printf("Bootstrap: Fetched %d families for namespace %s, Cursor: %s", len(resp.FigFamilies), ns, resp.Cursor)
	}

	return &Result{
		FigFamilies: allFamilies,
		Cursors:     cursors,
	}, nil
}
