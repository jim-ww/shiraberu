package providers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func init() {
	register(NewBing(NewHTTPClient()))
}

type Bing struct {
	client HttpProvider
}

func NewBing(client HttpProvider) *Bing {
	return &Bing{client: client}
}

func (*Bing) Name() string {
	return "bing"
}

func (b *Bing) Search(ctx context.Context, query string, page int) ([]SearchResult, error) {
	searchURL := b.buildURL(query, page)

	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("Referer", "https://www.bing.com/")
	header.Set("Cookie", b.buildBingCookie())

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

func (*Bing) buildURL(query string, page int) string {
	q := url.QueryEscape(query)
	if page == 0 {
		return fmt.Sprintf("https://www.bing.com/search?q=%s", q)
	}
	return fmt.Sprintf("https://www.bing.com/search?q=%s&first=%d", q, page*10+1)
}

func (b *Bing) parseResults(doc *goquery.Document, page int) []SearchResult {
	const (
		resultSelector = ".b_algo"
		titleSelector  = "h2 a"
		descSelector   = ".b_caption p"
	)

	var results []SearchResult

	doc.Find(resultSelector).Each(func(i int, s *goquery.Selection) {
		anchor := s.Find(titleSelector).First()
		if anchor.Length() == 0 {
			return
		}

		title := strings.TrimSpace(anchor.Text())
		if title == "" {
			return
		}

		href, ok := anchor.Attr("href")
		if !ok || href == "" {
			return
		}

		realURL := b.decodeBingURL(href)
		description := strings.TrimSpace(s.Find(descSelector).First().Text())

		results = append(results, SearchResult{
			Title:       title,
			URL:         realURL,
			Description: description,
			Page:        page,
			Position:    i,
			Provider:    b.Name(),
		})
	})

	return results
}

func (*Bing) buildBingCookie() string {
	return strings.Join([]string{
		"_EDGE_V=1",
		"SRCHD=AF=NOFORM",
		"_Rwho=u=d",
		"bngps=s=0",
		"_UR=QS=0&TQS=0",
	}, "; ")
}

// decodeBingURL extracts the real URL from a Bing tracking link.
func (*Bing) decodeBingURL(bingURL string) string {
	decode := func() (string, error) {
		parsed, err := url.Parse(bingURL)
		if err != nil {
			return "", err
		}

		u := parsed.Query().Get("u")
		if u == "" {
			return "", fmt.Errorf("no 'u' parameter found")
		}

		// Bing prefixes the base64 with "a1" — strip it
		u = strings.TrimPrefix(u, "a1")

		// Base64 URL decoding (Bing uses URL-safe base64)
		decoded, err := base64.URLEncoding.DecodeString(u)
		if err != nil {
			// Try without padding too
			decoded, err = base64.RawURLEncoding.DecodeString(u)
			if err != nil {
				return "", err
			}
		}

		return string(decoded), nil
	}

	decoded, err := decode()
	if err != nil {
		// Log error but return original URL as fallback
		return bingURL
	}
	return decoded
}
