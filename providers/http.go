package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
)

type HTTPClient struct {
	client *http.Client
}

type HTTPClientOption func(*HTTPClient)

func NewHTTPClient(opts ...HTTPClientOption) *HTTPClient {
	provider := &HTTPClient{
		client: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(provider)
	}
	return provider
}

func WithCustomClient(client *http.Client) HTTPClientOption {
	return func(hc *HTTPClient) {
		hc.client = client
	}
}

func (p *HTTPClient) Do(ctx context.Context, method, url string, header http.Header, reqBody io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("http new request: %w", err)
	}

	if header != nil {
		req.Header = header.Clone()
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request do: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if slices.Contains([]int{http.StatusForbidden, http.StatusTooManyRequests}, resp.StatusCode) {
		return nil, ErrRequestBlocked
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http response body readAll: %w", err)
	}

	if len(body) == 0 {
		return nil, ErrMalformedResponse
	}

	// os.WriteFile("index.html", body, 0o700)

	return body, nil
}
