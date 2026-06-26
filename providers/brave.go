package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func init() {
	register(NewBrave(NewHTTPClient()))
}

type Brave struct {
	client     HttpProvider
	safeSearch uint8
}

func NewBrave(client HttpProvider) *Brave {
	return &Brave{
		client:     client,
		safeSearch: 0, // off by default
	}
}

func (*Brave) Name() string {
	return "brave"
}

func (b *Brave) Search(ctx context.Context, query string, page int) ([]SearchResult, error) {
	searchURL := b.buildURL(query, page)

	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("Referer", "https://search.brave.com/")
	header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	header.Set("Sec-Ch-Ua-Mobile", "?0")
	header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	header.Set("Sec-Fetch-Dest", "document")
	header.Set("Sec-Fetch-Mode", "navigate")
	header.Set("Sec-Fetch-Site", "same-origin")
	header.Set("Sec-Fetch-User", "?1")
	header.Set("Upgrade-Insecure-Requests", "1")
	header.Set("Cookie", b.buildCookie())

	body, err := b.client.Do(ctx, http.MethodGet, searchURL, header, nil)
	if err != nil {
		return nil, fmt.Errorf("request do: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, ErrMalformedResponse
	}

	results := b.parseResults(doc, page)

	if len(results) == 0 {
		return nil, nil
	}

	return results, nil
}

func (b *Brave) buildURL(query string, page int) string {
	offset := ""
	if page > 0 {
		offset = fmt.Sprintf("&offset=%d", page)
	}
	return fmt.Sprintf(
		"https://search.brave.com/search?q=%s&source=web%s",
		url.QueryEscape(query),
		offset,
	)
}

func (b *Brave) buildCookie() string {
	return strings.Join([]string{
		"safesearch=" + b.safeSearchCookie(),
		"useLocation=0",
		"summarizer=0",
	}, "; ")
}

func (b *Brave) safeSearchCookie() string {
	switch b.safeSearch {
	case 0:
		return "off"
	case 1:
		return "moderate"
	default:
		return "strict"
	}
}

func (b *Brave) parseResults(doc *goquery.Document, page int) []SearchResult {
	const (
		resultSelector = "#results .snippet"
		titleSelector  = ".search-snippet-title"
		urlSelector    = ".result-wrapper .l1"
		descSelector   = ".generic-snippet .content"
	)

	var results []SearchResult

	doc.Find(resultSelector).Each(func(i int, s *goquery.Selection) {
		title := strings.TrimSpace(s.Find(titleSelector).First().Text())
		if title == "" {
			return
		}

		href, ok := s.Find(urlSelector).First().Attr("href")
		if !ok || href == "" {
			return
		}

		description := strings.TrimSpace(s.Find(descSelector).First().Text())
		description = b.cleanDescription(description)

		results = append(results, SearchResult{
			Title:       title,
			URL:         strings.TrimSpace(href),
			Description: description,
			Page:        page,
			Position:    i,
			Provider:    b.Name(),
		})
	})

	return results
}

// cleanDescription removes date prefixes like "2 weeks ago -" or "January 20, 2025 -"
func (b *Brave) cleanDescription(desc string) string {
	re := regexp.MustCompile(`^[^-]+\s*-\s*`)
	desc = re.ReplaceAllString(desc, "")
	return strings.TrimSpace(desc)
}
