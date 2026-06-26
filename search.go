package shiraberu

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jim-ww/shiraberu/providers"
	"golang.org/x/time/rate"
)

type SearchRequest struct {
	Query           string
	MinResults      int
	MaxResults      int
	StartFromPage   int
	providers       []providers.SearchProvider
	providerLimiter map[string]*rate.Limiter
	normalizeURL    func(url string) (normalized string)
	cache           providers.Cache
	debug           bool
}

type SearchResponse struct {
	Results      []providers.SearchResult `json:"results"`
	HasMore      bool                     `json:"has_more"`
	RequestsMade uint64                   `json:"requests_made"`
}

type Option func(SearchRequest) SearchRequest

func WithProviders(providers ...providers.SearchProvider) Option {
	return func(sr SearchRequest) SearchRequest {
		sr.providers = providers
		return sr
	}
}

func WithRatelimiters(providerLimiter map[string]*rate.Limiter) Option {
	return func(sr SearchRequest) SearchRequest {
		sr.providerLimiter = providerLimiter
		return sr
	}
}

func WithCustomNormalizeURL(normalizeURL func(url string) (normalized string)) Option {
	return func(sr SearchRequest) SearchRequest {
		sr.normalizeURL = normalizeURL
		return sr
	}
}

func WithCache(cache providers.Cache) Option {
	return func(sr SearchRequest) SearchRequest {
		sr.cache = cache
		return sr
	}
}

func WithDebugLogging() Option {
	return func(sr SearchRequest) SearchRequest {
		sr.debug = true
		return sr
	}
}

func cacheKey(req SearchRequest, providers []string) string {
	encodedQuery := base64.StdEncoding.EncodeToString([]byte(strings.ToLower(req.Query)))
	return fmt.Sprintf("page:%d|min:%d|max:%d|providers:%s|query:%s", req.StartFromPage, req.MinResults, req.MaxResults, strings.Join(providers, ","), encodedQuery)
}

func Search(ctx context.Context, req SearchRequest, opts ...Option) (SearchResponse, error) {
	for _, opt := range opts {
		req = opt(req)
	}
	if len(req.providers) == 0 {
		req.providers = []providers.SearchProvider{providers.NewDuckDuckGo(providers.NewHTTPClient())}
	}
	if req.normalizeURL == nil {
		req.normalizeURL = defaultNormalizeURL
	}

	cacheKey := func() string {
		providers := make([]string, 0, len(req.providers))
		for i := range req.providers {
			providers = append(providers, req.providers[i].Name())
		}
		return cacheKey(req, providers)
	}()

	var resp SearchResponse

	if req.cache != nil {
		if v, exists := req.cache.Get(cacheKey); exists {
			if req.debug {
				fmt.Println("returning cached results")
			}
			err := json.Unmarshal(v, &resp)
			if err == nil {
				return resp, nil
			}
			if req.debug {
				fmt.Fprintln(os.Stderr, "cache get: json unmarshal:", err)
			}
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultsChan := make(chan providers.SearchResult)
	wg := new(sync.WaitGroup)
	requestsMade := uint64(0)
	providerHasMorePages := new(sync.Map)
	for i := range req.providers {
		providerHasMorePages.Store(req.providers[i].Name(), true)
	}

	queryPage := func(page int) {
		defer close(resultsChan)
		for _, provider := range req.providers {
			wg.Go(func() {
				if hasMorePages, ok := providerHasMorePages.Load(provider.Name()); ok && !hasMorePages.(bool) {
					return
				}
				if req.providerLimiter != nil {
					if limiter := req.providerLimiter[provider.Name()]; limiter != nil {
						_ = limiter.Wait(ctx)
					}
				}
				if req.debug {
					log.Println("sending request...", "provider:", provider.Name())
				}
				atomic.AddUint64(&requestsMade, 1)
				results, err := provider.Search(ctx, req.Query, page)
				if err != nil {
					if req.debug {
						fmt.Fprintln(os.Stderr, provider.Name(), "err:", err)
					}
					return
				}
				if len(results) == 0 {
					providerHasMorePages.Store(provider.Name(), false)
				}
				for i := range results {
					select {
					case <-ctx.Done():
						return
					case resultsChan <- results[i]:
					}
				}
			})
		}
		wg.Wait()
	}

	page := req.StartFromPage
	resp.HasMore = true

	go queryPage(page)

	// url:result
	results := map[string]providers.SearchResult{}

OUTER:
	for {
		select {
		case v, ok := <-resultsChan:
			if !ok {
				resp.HasMore = false
				if len(results) >= req.MinResults {
					break OUTER
				}
				resultsChan = make(chan providers.SearchResult)
				page++
				// if at least one provider has more pages, we query another page
				providerHasMorePages.Range(func(key, value any) bool {
					if hasMorePages := value.(bool); hasMorePages {
						go queryPage(page)
					}
					return false
				})
			}
			v.URL = req.normalizeURL(v.URL)
			results[v.URL] = v

			if req.MaxResults > 0 && len(results) >= req.MaxResults {
				break OUTER
			}

		case <-ctx.Done():
			break OUTER
		}
	}

	resp.RequestsMade = requestsMade
	resp.Results = slices.SortedStableFunc(maps.Values(results), func(a, b providers.SearchResult) int {
		if a.Page < b.Page {
			return -1
		} else if a.Page > b.Page {
			return 1
		}
		if a.Position < b.Position {
			return -1
		} else if a.Position > b.Position {
			return 1
		}
		return 0
	})

	if req.cache != nil && len(resp.Results) != 0 {
		data, err := json.Marshal(resp)
		if err != nil {
			if req.debug {
				fmt.Fprintln(os.Stderr, "cache get: json unmarshal:", err)
			}
		}
		req.cache.Set(cacheKey, data)
	}

	select {
	case <-ctx.Done():
		return resp, ctx.Err()
	default:
	}

	return resp, nil
}

// Normalizes URLs to avoid URL duplicates. e.g:
// https://example.com
// https://example.com/
// https://example.com/index.html
// https://example.com/?utm_source=
// all become:
// https://example.com
func defaultNormalizeURL(raw string) string {
	u, err := url.Parse(strings.ToLower(raw))
	if err != nil {
		return raw
	}
	u.Fragment = ""
	if strings.HasSuffix(u.Path, "/index.html") {
		u.Path = strings.TrimSuffix(u.Path, "index.html")
	}
	if u.Path != "/" {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	return u.String()
}
