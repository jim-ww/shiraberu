package providers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func init() {
	register(NewDuckDuckGo(NewHTTPClient()))
}

type DuckDuckGo struct {
	client HttpProvider
}

func NewDuckDuckGo(client HttpProvider) *DuckDuckGo {
	return &DuckDuckGo{client: client}
}

func (*DuckDuckGo) Name() string {
	return "ddg"
}

func (d *DuckDuckGo) Search(
	ctx context.Context,
	query string,
	page int,
) ([]SearchResult, error) {
	searchURL := d.buildURL(query, page)

	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0")
	header.Set("Referer", "https://google.com/")
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	header.Set("Cookie", "kl=wt-wt")

	body, err := d.client.Do(ctx, http.MethodGet, searchURL, header, nil)
	if err != nil {
		return nil, fmt.Errorf("request do: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewBuffer(body))
	if err != nil {
		return nil, ErrMalformedResponse
	}

	const noResultsSelector = ".no-results"

	if doc.Find(noResultsSelector).Length() > 0 {
		return nil, nil
	}

	results := d.parseResults(doc, page)

	if len(results) == 0 {
		return nil, nil
	}

	return results, nil
}

func (d *DuckDuckGo) buildURL(query string, page int) string {
	q := url.QueryEscape(query)

	if page == 0 {
		return fmt.Sprintf(
			"https://html.duckduckgo.com/html/?q=%s&s=&dc=&v=1&o=json&api=/d.js",
			q,
		)
	}

	offset := page * 30

	return fmt.Sprintf(
		"https://duckduckgo.com/html/?q=%s&s=%d&dc=%d&v=1&o=json&api=/d.js",
		q,
		offset,
		offset+1,
	)
}

func (d *DuckDuckGo) parseResults(doc *goquery.Document, page int) []SearchResult {
	const (
		resultSelector = ".results>.result"
		titleSelector  = ".result__title>.result__a"
		urlSelector    = ".result__url"
		descSelector   = ".result__snippet"
	)

	var results []SearchResult

	doc.Find(resultSelector).Each(func(i int, s *goquery.Selection) {
		title := strings.TrimSpace(
			s.Find(titleSelector).First().Text(),
		)

		resultURL := strings.TrimSpace(
			s.Find(urlSelector).First().Text(),
		)

		if resultURL == "" {
			return
		}

		description := strings.TrimSpace(
			s.Find(descSelector).First().Text(),
		)

		if !strings.HasPrefix(resultURL, "https://") {
			resultURL = "https://" + resultURL
		}

		results = append(results, SearchResult{
			Title:       title,
			URL:         resultURL,
			Description: description,
			Page:        page,
			Position:    i,
			Provider:    d.Name(),
		})
	})

	return results
}
