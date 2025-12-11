package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/figchain/go-client/pkg/model"
	"github.com/hamba/avro/v2"
)

// Transport defines the interface for fetching data from the FigChain API.
type Transport interface {
	FetchInitial(ctx context.Context, req *model.InitialFetchRequest) (*model.InitialFetchResponse, error)
	FetchUpdate(ctx context.Context, req *model.UpdateFetchRequest) (*model.UpdateFetchResponse, error)
	Close() error
}

// HTTPTransport is an HTTP implementation of the Transport interface.
type HTTPTransport struct {
	client        *http.Client
	baseURL       string
	clientSecret  string
	environmentID string
}

// NewHTTPTransport creates a new HTTPTransport.
func NewHTTPTransport(client *http.Client, baseURL, clientSecret, environmentID string) *HTTPTransport {
	return &HTTPTransport{
		client:        client,
		baseURL:       baseURL,
		clientSecret:  clientSecret,
		environmentID: environmentID,
	}
}

func (t *HTTPTransport) FetchInitial(ctx context.Context, req *model.InitialFetchRequest) (*model.InitialFetchResponse, error) {
	endpoint := fmt.Sprintf("%s/data/initial", t.baseURL)
	scheme, err := avro.Parse(model.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}

	reqSchema := findSchemaByName(scheme, "InitialFetchRequest")

	reqBytes, err := avro.Marshal(reqSchema, req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	respBytes, err := t.doRequest(ctx, endpoint, reqBytes)
	if err != nil {
		return nil, err
	}

	respSchema := findSchemaByName(scheme, "InitialFetchResponse")

	var resp model.InitialFetchResponse
	if err := avro.Unmarshal(respSchema, respBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &resp, nil
}

func (t *HTTPTransport) FetchUpdate(ctx context.Context, req *model.UpdateFetchRequest) (*model.UpdateFetchResponse, error) {
	endpoint := fmt.Sprintf("%s/data/updates", t.baseURL)
	scheme, err := avro.Parse(model.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}

	reqSchema := findSchemaByName(scheme, "UpdateFetchRequest")

	reqBytes, err := avro.Marshal(reqSchema, req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	respBytes, err := t.doRequest(ctx, endpoint, reqBytes)
	if err != nil {
		return nil, err
	}

	respSchema := findSchemaByName(scheme, "UpdateFetchResponse")

	var resp model.UpdateFetchResponse
	if err := avro.Unmarshal(respSchema, respBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &resp, nil
}

func (t *HTTPTransport) Close() error {
	return nil
}

func (t *HTTPTransport) doRequest(ctx context.Context, urlStr string, reqBytes []byte) ([]byte, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	if t.clientSecret != "" {
		req.Header.Set("Authorization", "Bearer "+t.clientSecret)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return bodyBytes, nil
}

func findSchemaByName(root avro.Schema, name string) avro.Schema {
	if union, ok := root.(*avro.UnionSchema); ok {
		for _, s := range union.Types() {
			if ns, ok := s.(avro.NamedSchema); ok {
				if ns.FullName() == "io.figchain.avro.model."+name || ns.Name() == name {
					return s
				}
			}
		}
	}
	return root
}
