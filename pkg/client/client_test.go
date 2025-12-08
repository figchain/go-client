package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/figchain/go-client/pkg/client"
	"github.com/figchain/go-client/pkg/config"
	"github.com/figchain/go-client/pkg/evaluation"
	"github.com/figchain/go-client/pkg/model"
)

// MockAvroRecord implements AvroRecord for testing
type MockAvroRecord struct {
	Value string `avro:"value"`
}

func (m *MockAvroRecord) Schema() string {
	return `{
		"type": "record",
		"name": "MockAvroRecord",
		"fields": [{"name": "value", "type": "string"}]
	}`
}

func TestClient_GetFig(t *testing.T) {
	// Setup mock server
	mockInitialResp := &model.InitialFetchResponse{
		Cursor: "1",
		FigFamilies: []model.FigFamily{
			{
				Definition: model.FigDefinition{Key: "test-key", Namespace: "default"},
				Figs: []model.Fig{
					{Version: "v1", Payload: []byte("\x06foo")}, // Avro string "foo"
				},
				DefaultVersion: ptr("v1"),
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/fig/initial" {
			json.NewEncoder(w).Encode(mockInitialResp)
			return
		}
		if r.URL.Path == "/v1/fig/updates" {
			json.NewEncoder(w).Encode(&model.UpdateFetchResponse{Cursor: "1"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Initialise client
	c, err := client.New(
		config.WithBaseURL(server.URL),
		config.WithEnvironmentID("env-1"),
		config.WithNamespaces("default"),
		config.WithPollingInterval(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Test GetFig
	var record MockAvroRecord
	ctx := evaluation.NewEvaluationContext(nil)
	if err := c.GetFig("test-key", &record, ctx); err != nil {
		t.Fatalf("GetFig failed: %v", err)
	}

	if record.Value != "foo" {
		t.Errorf("Expected value 'foo', got '%s'", record.Value)
	}
}

func TestClient_Watch(t *testing.T) {
	// Setup mock server handling initial fetch and one update
	mockInitialResp := &model.InitialFetchResponse{
		Cursor: "1",
		FigFamilies: []model.FigFamily{
			{
				Definition: model.FigDefinition{Key: "watch-key", Namespace: "default"},
				Figs: []model.Fig{
					{Version: "v1", Payload: []byte("\x06foo")},
				},
				DefaultVersion: ptr("v1"),
			},
		},
	}

	var updateMutex sync.Mutex
	params := struct {
		updateServed bool
	}{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/fig/initial" {
			json.NewEncoder(w).Encode(mockInitialResp)
			return
		}
		if r.URL.Path == "/v1/fig/updates" {
			updateMutex.Lock()
			defer updateMutex.Unlock()

			if !params.updateServed {
				// Serve an update
				params.updateServed = true
				json.NewEncoder(w).Encode(&model.UpdateFetchResponse{
					Cursor: "2",
					FigFamilies: []model.FigFamily{
						{
							Definition: model.FigDefinition{Key: "watch-key", Namespace: "default"},
							Figs: []model.Fig{
								{Version: "v2", Payload: []byte("\x06bar")},
							},
							DefaultVersion: ptr("v2"),
						},
					},
				})
			} else {
				// No more updates
				json.NewEncoder(w).Encode(&model.UpdateFetchResponse{Cursor: "2"})
			}
			return
		}
	}))
	defer server.Close()

	c, err := client.New(
		config.WithBaseURL(server.URL),
		config.WithEnvironmentID("env-1"),
		config.WithNamespaces("default"),
		config.WithPollingInterval(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Start watcher
	ch := c.Watch(context.Background(), "watch-key")

	// Wait for update
	select {
	case ff := <-ch:
		if *ff.DefaultVersion != "v2" {
			t.Errorf("Expected version v2, got %s", *ff.DefaultVersion)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for update")
	}
}

func ptr(s string) *string {
	return &s
}
