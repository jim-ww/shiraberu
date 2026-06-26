package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func init() {
	register(NewWikipedia(NewHTTPClient()))
}

type wikipedia struct {
	client   HttpProvider
	language string
	host     string
}

type WikipediaOption func(*wikipedia)

// WithLanguage sets the Wikipedia language (e.g., "en", "de", "fr").
func WithWikipediaLanguage(lang string) WikipediaOption {
	return func(w *wikipedia) {
		w.language = lang
		w.host = fmt.Sprintf("https://%s.wikipedia.org", lang)
	}
}

func NewWikipedia(client HttpProvider, opts ...WikipediaOption) *wikipedia {
	w := &wikipedia{
		client:   client,
		language: "en",
		host:     "https://en.wikipedia.org",
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (*wikipedia) Name() string {
	return "wikipedia"
}

func (w *wikipedia) Search(ctx context.Context, query string, page int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}

	searchURL := w.buildURL(query, page)

	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	header.Set("Referer", w.host)
	header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	header.Set("Accept-Language", "en-US,en;q=0.9")

	body, err := w.client.Do(ctx, http.MethodGet, searchURL, header, nil)
	if err != nil {
		return nil, ErrRequestBlocked
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, ErrMalformedResponse
	}

	const noResultsSelector = "p.mw-search-nonefound"
	if doc.Find(noResultsSelector).Length() > 0 {
		return nil, nil
	}

	results := w.parseResults(doc, page)

	if len(results) == 0 {
		return nil, nil
	}

	return results, nil
}

func (w *wikipedia) buildURL(query string, page int) string {
	const defaultLimit = 20
	offset := int(page-1) * defaultLimit

	params := url.Values{}
	params.Set("limit", fmt.Sprintf("%d", defaultLimit))
	params.Set("offset", fmt.Sprintf("%d", offset))
	params.Set("profile", "default")
	params.Set("search", query)
	params.Set("title", "Special:Search")
	params.Set("ns0", "1")

	return fmt.Sprintf("%s/w/index.php?%s", w.host, params.Encode())
}

func (w *wikipedia) parseResults(doc *goquery.Document, page int) []SearchResult {
	const (
		resultSelector = ".mw-search-results li.mw-search-result"
		titleSelector  = ".mw-search-result-heading a"
		descSelector   = ".searchresult"
	)

	var results []SearchResult

	doc.Find(resultSelector).Each(func(i int, sel *goquery.Selection) {
		titleSel := sel.Find(titleSelector).First()
		title := strings.TrimSpace(titleSel.Text())
		if title == "" {
			return
		}

		relativeURL, exists := titleSel.Attr("href")
		if !exists || relativeURL == "" {
			return
		}

		fullURL := w.host + relativeURL
		description := strings.TrimSpace(sel.Find(descSelector).First().Text())

		results = append(results, SearchResult{
			Title:       title,
			URL:         fullURL,
			Description: description,
			Provider:    w.Name(),
			Page:        page,
			Position:    i,
		})
	})

	return results
}
