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
	"github.com/hamba/avro/v2"
	"github.com/hamba/avro/v2/ocf"
)

// Transport defines the interface for fetching data from the FigChain API.
type Transport interface {
	FetchInitial(ctx context.Context, req *model.InitialFetchRequest) (*model.InitialFetchResponse, error)
	FetchUpdate(ctx context.Context, req *model.UpdateFetchRequest) (*model.UpdateFetchResponse, error)
	GetNamespaceKey(ctx context.Context, namespace string) ([]*model.NamespaceKey, error)
	UploadPublicKey(ctx context.Context, key *model.UserPublicKey) error
	Close() error
}

// HTTPTransport is an HTTP implementation of the Transport interface.
type HTTPTransport struct {
	client        *http.Client
	baseURL       string
	tokenProvider TokenProvider
	environmentID string
}

// NewHTTPTransport creates a new HTTPTransport.
func NewHTTPTransport(client *http.Client, baseURL string, tokenProvider TokenProvider, environmentID string) *HTTPTransport {
	return &HTTPTransport{
		client:        client,
		baseURL:       baseURL,
		tokenProvider: tokenProvider,
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

	// Use OCF for request
	var buf bytes.Buffer
	enc, err := ocf.NewEncoder(reqSchema.String(), &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCF encoder: %w", err)
	}
	if err := enc.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush OCF encoder: %w", err)
	}
	// OCF encoder writes to buf

	respBytes, err := t.doRequest(ctx, endpoint, buf.Bytes())
	if err != nil {
		return nil, err
	}

	// Use OCF for response
	dec, err := ocf.NewDecoder(bytes.NewReader(respBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create OCF decoder: %w", err)
	}

	var resp model.InitialFetchResponse
	if dec.HasNext() {
		if err := dec.Decode(&resp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
	} else {
		return nil, fmt.Errorf("empty response")
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

	// Use OCF for request
	var buf bytes.Buffer
	enc, err := ocf.NewEncoder(reqSchema.String(), &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCF encoder: %w", err)
	}
	if err := enc.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush OCF encoder: %w", err)
	}

	respBytes, err := t.doRequest(ctx, endpoint, buf.Bytes())
	if err != nil {
		return nil, err
	}

	// Use OCF for response
	dec, err := ocf.NewDecoder(bytes.NewReader(respBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create OCF decoder: %w", err)
	}

	var resp model.UpdateFetchResponse
	if dec.HasNext() {
		if err := dec.Decode(&resp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
	} else {
		return nil, fmt.Errorf("empty response")
	}

	return &resp, nil
}

func (t *HTTPTransport) GetNamespaceKey(ctx context.Context, namespace string) ([]*model.NamespaceKey, error) {
	endpoint := fmt.Sprintf("%s/keys/namespace/%s", t.baseURL, url.PathEscape(namespace))
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	token, err := t.tokenProvider.GetToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
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

	var nsKeys []*model.NamespaceKey
	if err := json.Unmarshal(bodyBytes, &nsKeys); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return nsKeys, nil
}

func (t *HTTPTransport) UploadPublicKey(ctx context.Context, key *model.UserPublicKey) error {
	endpoint := fmt.Sprintf("%s/keys/public", t.baseURL)
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	jsonBytes, err := json.Marshal(key)
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", u.String(), bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	token, err := t.tokenProvider.GetToken()
	if err != nil {
		return fmt.Errorf("failed to get auth token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned error %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
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
	token, err := t.tokenProvider.GetToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

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
