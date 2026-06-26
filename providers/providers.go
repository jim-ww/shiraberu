package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

var (
	// bot protection
	ErrCaptcha = fmt.Errorf("captcha detected")
	// http 403, 429 errors
	ErrRequestBlocked = fmt.Errorf("request blocked")
	// invalid format
	ErrMalformedResponse = fmt.Errorf("malformed response")
	// API errors, etc
	ErrBadResponse = fmt.Errorf("bad response")
)

type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Page        int    `json:"page"`
	Position    int    `json:"position"`
	Provider    string `json:"provider"`
}

type HttpProvider interface {
	Do(ctx context.Context, method, url string, header http.Header, reqBody io.Reader) ([]byte, error)
}

type SearchProvider interface {
	Name() string
	Search(ctx context.Context, query string, page int) ([]SearchResult, error)
}

type Cache interface {
	Get(key string) (value []byte, exists bool)
	Set(key string, value []byte)
	Close() error
}
