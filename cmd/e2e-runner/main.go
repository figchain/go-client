// Command e2e-runner is the Go client's runner for the cross-client e2e suite
// (figchain-clients/e2e/run.py). It reads its configuration from FC_* environment
// variables and prints one "E2E-RESULT {json}" line.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/figchain/go-client/pkg/client"
	"github.com/figchain/go-client/pkg/config"
	"github.com/hamba/avro/v2"
)

type CheckoutSettings struct {
	Currency string `avro:"currency" json:"currency"`
	MaxItems int    `avro:"maxItems" json:"maxItems"`
}

func (c *CheckoutSettings) Schema() avro.Schema {
	return avro.MustParse(`{"type":"record","name":"CheckoutSettings","fields":[
		{"name":"currency","type":"string"},{"name":"maxItems","type":"int"}]}`)
}

type result struct {
	Mode  string            `json:"mode"`
	OK    bool              `json:"ok"`
	Value *CheckoutSettings `json:"value,omitempty"`
	Type  string            `json:"type,omitempty"`
	Error string            `json:"error,omitempty"`
}

var strategies = map[string]config.BootstrapStrategy{
	"server-first": config.BootstrapStrategyServerFirst,
	"hybrid":       config.BootstrapStrategyHybrid,
	"backup-only":  config.BootstrapStrategyS3BackupOnly,
	"listen":       config.BootstrapStrategyServerFirst,
}

func emit(r result) {
	b, _ := json.Marshal(r)
	fmt.Println("E2E-RESULT " + string(b))
}

func typed(mode string, v *CheckoutSettings) result {
	return result{Mode: mode, OK: true, Value: v, Type: fmt.Sprintf("%T", v)}
}

func main() {
	mode := os.Args[1]
	env := os.Getenv
	c, err := client.New(
		config.WithBaseURL(env("FC_BASE_URL")),
		config.WithTenantID(env("FC_TENANT_ID")),
		config.WithEnvironmentID(env("FC_ENVIRONMENT_ID")),
		config.WithNamespaces(env("FC_NAMESPACE")),
		config.WithClientSecret(env("FC_CLIENT_SECRET")),
		config.WithPollingInterval(time.Second),
		config.WithMaxRetries(1),
		config.WithRetryDelay(100*time.Millisecond),
		config.WithBootstrapStrategy(strategies[mode]),
		config.WithS3BackupEnabled(true),
		config.WithS3BackupBucket(env("FC_S3_BUCKET")),
		config.WithS3BackupPrefix(env("FC_S3_PREFIX")),
		config.WithS3BackupRegion(env("AWS_REGION")),
		config.WithS3BackupEndpoint(env("FC_S3_ENDPOINT")),
		config.WithS3BackupPathStyle(true),
		func(cfg *config.Config) { cfg.EncryptionPrivateKey = env("FC_ENCRYPTION_PRIVATE_KEY") },
	)
	if err != nil {
		emit(result{Mode: mode, Error: "client.New: " + err.Error()})
		os.Exit(1)
	}
	defer c.Close()
	key := env("FC_FIG_KEY")

	var initial CheckoutSettings
	if err := c.GetFig(key, &initial, nil); err != nil {
		emit(result{Mode: mode, Error: "GetFig: " + err.Error()})
		return
	}
	if mode != "listen" {
		emit(typed(mode, &initial))
		return
	}

	changes := make(chan *CheckoutSettings, 10)
	c.RegisterListener(key, &CheckoutSettings{}, func(r client.AvroRecord) {
		if v, ok := r.(*CheckoutSettings); ok && *v != initial {
			changes <- v
		}
	})
	fmt.Println("E2E-READY")
	timeout, _ := strconv.Atoi(env("FC_LISTEN_TIMEOUT"))
	if timeout == 0 {
		timeout = 60
	}
	select {
	case v := <-changes:
		emit(typed(mode, v))
	case <-time.After(time.Duration(timeout) * time.Second):
		emit(result{Mode: mode, Error: "listener saw no change"})
	}
}
