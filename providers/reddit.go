package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	register(NewReddit(NewHTTPClient()))
}

type reddit struct {
	client HttpProvider
}

type RedditOption func(*reddit)

func NewReddit(client HttpProvider, opts ...RedditOption) *reddit {
	r := &reddit{client: client}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (*reddit) Name() string {
	return "reddit"
}

func (r *reddit) Search(ctx context.Context, query string, page int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}

	searchURL := r.buildURL(query, page)

	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	header.Set("Accept", "application/json, text/plain, */*")
	header.Set("Accept-Language", "en-US,en;q=0.9")

	body, err := r.client.Do(ctx, http.MethodGet, searchURL, header, nil)
	if err != nil {
		return nil, ErrRequestBlocked
	}

	results, err := r.parseResponse(body, page)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}

func (r *reddit) buildURL(query string, page int) string {
	params := url.Values{}
	params.Set("q", query)
	params.Set("limit", "25")
	if page > 1 {
		params.Set("after", fmt.Sprintf("t%d", (page-1)*25))
	}
	return "https://www.reddit.com/search.json?" + params.Encode()
}

func (r *reddit) parseResponse(body []byte, page int) ([]SearchResult, error) {
	type redditPost struct {
		Data struct {
			Title      string  `json:"title"`
			Permalink  string  `json:"permalink"`
			Thumbnail  string  `json:"thumbnail"`
			URL        string  `json:"url"`
			Selftext   string  `json:"selftext"`
			CreatedUtc float64 `json:"created_utc"`
		} `json:"data"`
	}
	type redditResponse struct {
		Data struct {
			Children []redditPost `json:"children"`
		} `json:"data"`
	}

	var resp redditResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, ErrMalformedResponse
	}

	var results []SearchResult

	for _, post := range resp.Data.Children {
		data := post.Data
		if data.Title == "" {
			continue
		}

		permalink := "https://www.reddit.com" + data.Permalink
		title := data.Title

		// Check if thumbnail is a valid URL
		thumbnail := data.Thumbnail
		if strings.HasPrefix(thumbnail, "http://") || strings.HasPrefix(thumbnail, "https://") {
			results = append(results, SearchResult{
				Title:       title,
				URL:         data.URL,
				Description: thumbnail, // store thumbnail URL in description for now
				Provider:    r.Name(),
				Page:        page,
				Position:    len(results),
			})
		} else {
			content := data.Selftext
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			t := time.Unix(int64(data.CreatedUtc), 0)
			dateStr := t.Format("2006-01-02 15:04:05")

			results = append(results, SearchResult{
				Title:       title,
				URL:         permalink,
				Description: content + "\n\nPosted: " + dateStr,
				Provider:    r.Name(),
				Page:        page,
				Position:    len(results),
			})
		}
	}

	return results, nil
}
