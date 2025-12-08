package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/figchain/go-client/pkg/model"
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
	endpoint := fmt.Sprintf("%s/v1/fig/initial", t.baseURL)
	resp, err := t.doRequest(ctx, endpoint, req, &model.InitialFetchResponse{})
	if err != nil {
		return nil, err
	}
	return resp.(*model.InitialFetchResponse), nil
}

func (t *HTTPTransport) FetchUpdate(ctx context.Context, req *model.UpdateFetchRequest) (*model.UpdateFetchResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/fig/updates", t.baseURL)
	resp, err := t.doRequest(ctx, endpoint, req, &model.UpdateFetchResponse{})
	if err != nil {
		return nil, err
	}
	return resp.(*model.UpdateFetchResponse), nil
}

func (t *HTTPTransport) Close() error {
	return nil
}

func (t *HTTPTransport) doRequest(ctx context.Context, urlStr string, reqBody any, respBody any) (any, error) {
	// Ensure URL is valid
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	var bodyReader io.Reader
	if reqBody != nil {
		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if t.clientSecret != "" {
		req.Header.Set("Authorization", "Bearer "+t.clientSecret)
	}
	req.Header.Set("X-FigChain-Environment-ID", t.environmentID)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return respBody, nil
	}

	return nil, nil
}
