package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func init() {
	register(NewSepiaSearch(NewHTTPClient()))
}

type sepiaSearch struct {
	client HttpProvider
}

type SepiaSearchOption func(*sepiaSearch)

func NewSepiaSearch(client HttpProvider, opts ...SepiaSearchOption) *sepiaSearch {
	s := &sepiaSearch{
		client: client,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (*sepiaSearch) Name() string {
	return "sepiasearch"
}

func (s *sepiaSearch) Search(ctx context.Context, query string, page int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}

	searchURL := s.buildURL(query, page)

	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0")
	header.Set("Referer", "https://sepiasearch.org/")

	body, err := s.client.Do(ctx, http.MethodGet, searchURL, header, nil)
	if err != nil {
		return nil, ErrRequestBlocked
	}

	bodyStr := string(body)
	if strings.Contains(bodyStr, "captcha") {
		return nil, ErrCaptcha
	}
	if strings.Contains(bodyStr, "<html") {
		return nil, ErrMalformedResponse
	}

	results, err := s.parseJSONResponse(body, page)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}

func (s *sepiaSearch) buildURL(query string, page int) string {
	start := page * 10
	q := url.Values{}
	q.Set("search", query)
	q.Set("start", fmt.Sprint(start))
	q.Set("count", "10")
	q.Set("sort", "-match")
	q.Set("nsfw", "false")

	return fmt.Sprintf(
		"https://sepiasearch.org/api/v1/search/videos?%s",
		q.Encode(),
	)
}

func (s *sepiaSearch) parseJSONResponse(body []byte, page int) ([]SearchResult, error) {
	type video struct {
		Name        string  `json:"name"`
		URL         string  `json:"url"`
		Description *string `json:"description"`
	}
	type response struct {
		Data  []video `json:"data"`
		Total *int    `json:"total"`
		Error *string `json:"error"`
	}

	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, ErrMalformedResponse
	}

	if resp.Error != nil && *resp.Error != "" {
		return nil, ErrBadResponse
	}

	results := make([]SearchResult, 0, len(resp.Data))
	for i, v := range resp.Data {
		desc := ""
		if v.Description != nil {
			desc = *v.Description
		}

		results = append(results, SearchResult{
			Title:       strings.TrimSpace(v.Name),
			URL:         strings.TrimSpace(v.URL),
			Description: strings.TrimSpace(desc),
			Provider:    s.Name(),
			Position:    i,
			Page:        page,
		})
	}

	return results, nil
}
