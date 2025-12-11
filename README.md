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

// Define your configuration struct
type MyConfig struct {
	FeatureEnabled bool `avro:"feature_enabled"`
	MaxItems       int  `avro:"max_items"`
}

// Implement the AvroRecord interface
func (c *MyConfig) Schema() string {
	return `{
		"type": "record",
		"name": "MyConfig",
		"fields": [
			{"name": "feature_enabled", "type": "boolean"},
			{"name": "max_items", "type": "int"}
		]
	}`
}

func main() {
	// Build the client
	c, err := client.New(
		config.WithBaseURL("https://api.figchain.io"),
		config.WithClientSecret("your-api-key"),
		config.WithEnvironmentID("your-environment-id"),
		config.WithNamespaces("default"),
	)
	if err != nil {
		log.Fatal(err)
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
