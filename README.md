# FigChain Go Client

Official Go client library for [FigChain](https://figchain.io) configuration management.

## Installation

<a href="https://github.com/figchain/go-client/releases">
	<img src="https://img.shields.io/github/v/release/figchain/go-client" alt="GitHub Release" />
</a>

```bash
go get github.com/figchain/go-client
```

## Quick Start

```go
package main

import (
	"fmt"
	"log"

	"github.com/figchain/go-client/pkg/client"
	"github.com/figchain/go-client/pkg/config"
	"github.com/figchain/go-client/pkg/evaluation"
)

func main() {
	configPath := "path/to/your/client-config.json" // The config that was downloaded in the UI
	// Build the client
	c, err := client.NewClientFromConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to create FigChain client: %v", err)
	}
	defer c.Close()

	// Evaluate a configuration (for traffic split support)
	ctx := evaluation.NewEvaluationContext(map[string]string{
		"userId": "user123",
		"plan":   "premium",
	})

	// Retrieve and decode the configuration
	var cfg MyConfig
	if err := c.GetFig("your-fig-key", &cfg, ctx); err != nil {
		log.Printf("Error getting fig: %v", err)
		return
	}

	fmt.Printf("Feature Enabled: %v\n", cfg.FeatureEnabled)
}
```
