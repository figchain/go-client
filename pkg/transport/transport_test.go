package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/figchain/go-client/pkg/model"
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/v1/fig/initial" {
			t.Errorf("Expected path /v1/fig/initial, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Expected Authorization header Bearer secret, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-FigChain-Environment-ID") != "env-1" {
			t.Errorf("Expected X-FigChain-Environment-ID header env-1, got %s", r.Header.Get("X-FigChain-Environment-ID"))
		}

		json.NewEncoder(w).Encode(mockResp)
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/v1/fig/updates" {
			t.Errorf("Expected path /v1/fig/updates, got %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(mockResp)
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
