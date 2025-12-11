package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hamba/avro/v2"

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

func getRespSchema(name string) avro.Schema {
	scheme, _ := avro.Parse(model.Schema)
	if union, ok := scheme.(*avro.UnionSchema); ok {
		for _, s := range union.Types() {
			if ns, ok := s.(avro.NamedSchema); ok {
				if ns.FullName() == "io.figchain.avro.model."+name || ns.Name() == name {
					return s
				}
			}
		}
	}
	return scheme
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
		if r.URL.Path == "/data/initial" {
			schema := getRespSchema("InitialFetchResponse")
			data, _ := avro.Marshal(schema, mockInitialResp)
			w.Write(data)
			return
		}
		if r.URL.Path == "/data/updates" {
			schema := getRespSchema("UpdateFetchResponse")
			data, _ := avro.Marshal(schema, &model.UpdateFetchResponse{Cursor: "1"})
			w.Write(data)
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
		if r.URL.Path == "/data/initial" {
			schema := getRespSchema("InitialFetchResponse")
			data, _ := avro.Marshal(schema, mockInitialResp)
			w.Write(data)
			return
		}
		if r.URL.Path == "/data/updates" {
			updateMutex.Lock()
			defer updateMutex.Unlock()

			schema := getRespSchema("UpdateFetchResponse")
			if !params.updateServed {
				// Serve an update
				params.updateServed = true
				resp := &model.UpdateFetchResponse{
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
				}
				data, _ := avro.Marshal(schema, resp)
				w.Write(data)
			} else {
				// No more updates
				resp := &model.UpdateFetchResponse{Cursor: "2"}
				data, _ := avro.Marshal(schema, resp)
				w.Write(data)
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
