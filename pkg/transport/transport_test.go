package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/figchain/go-client/pkg/model"
	"github.com/hamba/avro/v2"
)

func TestHTTPTransport_FetchInitial(t *testing.T) {
	mockResp := &model.InitialFetchResponse{
		Cursor: "cursor-123",
		FigFamilies: []model.FigFamily{
			{
				Definition: model.FigDefinition{Key: "fig-1", Namespace: "ns-1"},
			},
		},
	}

	scheme, _ := avro.Parse(model.Schema)
	// Find response schema
	var respSchema avro.Schema
	if union, ok := scheme.(*avro.UnionSchema); ok {
		for _, s := range union.Types() {
			if ns, ok := s.(avro.NamedSchema); ok {
				if ns.FullName() == "io.figchain.avro.model.InitialFetchResponse" || ns.Name() == "InitialFetchResponse" {
					respSchema = s
					break
				}
			}
		}
	}
	if respSchema == nil {
		respSchema = scheme
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/data/initial" {
			t.Errorf("Expected path /data/initial, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Expected Authorization header Bearer secret, got %s", r.Header.Get("Authorization"))
		}

		data, err := avro.Marshal(respSchema, mockResp)
		if err != nil {
			t.Errorf("Failed to marshal mock response: %v", err)
			return
		}
		w.Write(data)
	}))
	defer server.Close()

	tr := NewHTTPTransport(server.Client(), server.URL, "secret", "env-1")

	resp, err := tr.FetchInitial(context.Background(), &model.InitialFetchRequest{
		Namespace:     "ns-1",
		EnvironmentID: "env-1",
	})
	if err != nil {
		t.Fatalf("FetchInitial failed: %v", err)
	}

	if resp.Cursor != mockResp.Cursor {
		t.Errorf("Expected cursor %s, got %s", mockResp.Cursor, resp.Cursor)
	}
	if len(resp.FigFamilies) != 1 {
		t.Errorf("Expected 1 fig family, got %d", len(resp.FigFamilies))
	}
}

func TestHTTPTransport_FetchUpdate(t *testing.T) {
	mockResp := &model.UpdateFetchResponse{
		Cursor: "cursor-456",
		FigFamilies: []model.FigFamily{
			{
				Definition: model.FigDefinition{Key: "fig-1", Namespace: "ns-1"},
			},
		},
	}

	scheme, _ := avro.Parse(model.Schema)
	// Find response schema
	var respSchema avro.Schema
	if union, ok := scheme.(*avro.UnionSchema); ok {
		for _, s := range union.Types() {
			if ns, ok := s.(avro.NamedSchema); ok {
				if ns.FullName() == "io.figchain.avro.model.UpdateFetchResponse" || ns.Name() == "UpdateFetchResponse" {
					respSchema = s
					break
				}
			}
		}
	}
	if respSchema == nil {
		respSchema = scheme
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/data/updates" {
			t.Errorf("Expected path /data/updates, got %s", r.URL.Path)
		}

		data, err := avro.Marshal(respSchema, mockResp)
		if err != nil {
			t.Errorf("Failed to marshal mock response: %v", err)
			return
		}
		w.Write(data)
	}))
	defer server.Close()

	tr := NewHTTPTransport(server.Client(), server.URL, "secret", "env-1")

	resp, err := tr.FetchUpdate(context.Background(), &model.UpdateFetchRequest{
		Namespace:     "ns-1",
		EnvironmentID: "env-1",
		Cursor:        "cursor-123",
	})
	if err != nil {
		t.Fatalf("FetchUpdate failed: %v", err)
	}

	if resp.Cursor != mockResp.Cursor {
		t.Errorf("Expected cursor %s, got %s", mockResp.Cursor, resp.Cursor)
	}
}

func TestHTTPTransport_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	tr := NewHTTPTransport(server.Client(), server.URL, "secret", "env-1")

	_, err := tr.FetchInitial(context.Background(), &model.InitialFetchRequest{})
	if err == nil {
		t.Error("Expected error, got nil")
	}
}
