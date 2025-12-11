package bootstrap

import (
	"context"

	"github.com/figchain/go-client/pkg/model"
)

// Result holds the result of a bootstrap operation.
type Result struct {
	FigFamilies []model.FigFamily
	Cursors     map[string]string
}

// Strategy defines the interface for bootstrapping the client.
type Strategy interface {
	Bootstrap(ctx context.Context, namespaces []string) (*Result, error)
}
