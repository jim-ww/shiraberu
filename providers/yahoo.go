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
	register(NewYahoo(NewHTTPClient()))
}

var yahooLanguages = map[string]string{
	"all": "any", "ar": "ar", "bg": "bg", "cs": "cs", "da": "da",
	"de": "de", "el": "el", "en": "en", "es": "es", "et": "et",
	"fi": "fi", "fr": "fr", "he": "he", "hr": "hr", "hu": "hu",
	"it": "it", "ja": "ja", "ko": "ko", "lt": "lt", "lv": "lv",
	"nl": "nl", "no": "no", "pl": "pl", "pt": "pt", "ro": "ro",
	"ru": "ru", "sk": "sk", "sl": "sl", "sv": "sv", "th": "th",
	"tr": "tr", "zh": "zh_chs", "zh_Hans": "zh_chs", "zh-CN": "zh_chs",
	"zh_Hant": "zh_cht", "zh-HK": "zh_cht", "zh-TW": "zh_cht",
}

var yahooRegion2domain = map[string]string{
	"CO": "co.search.yahoo.com", "TH": "th.search.yahoo.com",
	"VE": "ve.search.yahoo.com", "CL": "cl.search.yahoo.com",
	"HK": "hk.search.yahoo.com", "PE": "pe.search.yahoo.com",
	"CA": "ca.search.yahoo.com", "DE": "de.search.yahoo.com",
	"FR": "fr.search.yahoo.com", "TW": "tw.search.yahoo.com",
	"GB": "uk.search.yahoo.com", "UK": "uk.search.yahoo.com",
	"BR": "br.search.yahoo.com", "IN": "in.search.yahoo.com",
	"ES": "espanol.search.yahoo.com", "PH": "ph.search.yahoo.com",
	"AR": "ar.search.yahoo.com", "MX": "mx.search.yahoo.com",
	"SG": "sg.search.yahoo.com",
}

var yahooLang2domain = map[string]string{
	"zh_chs": "hk.search.yahoo.com", "zh_cht": "tw.search.yahoo.com",
	"any": "search.yahoo.com", "en": "search.yahoo.com",
	"bg": "search.yahoo.com", "cs": "search.yahoo.com",
	"da": "search.yahoo.com", "el": "search.yahoo.com",
	"et": "search.yahoo.com", "he": "search.yahoo.com",
	"hr": "search.yahoo.com", "ja": "search.yahoo.com",
	"ko": "search.yahoo.com", "sk": "search.yahoo.com",
	"sl": "search.yahoo.com",
}

type Yahoo struct {
	client     HttpProvider
	language   string
	region     string
	safeSearch int
	domain     string
}

type YahooOption func(*Yahoo)

// WithLanguage sets the search language (ISO 639‑1 code).
func WithYahooLanguage(lang string) YahooOption {
	return func(y *Yahoo) { y.language = lang }
}

// WithRegion sets the region (two‑letter country code).
func WithYahooRegion(region string) YahooOption {
	return func(y *Yahoo) { y.region = region }
}

// WithSafeSearch sets the safe‑search level (0=off, 1=moderate, 2=strict).
func WithYahooSafeSearch(level int) YahooOption {
	return func(y *Yahoo) { y.safeSearch = level }
}

func NewYahoo(client HttpProvider, opts ...YahooOption) *Yahoo {
	y := &Yahoo{
		client:     client,
		language:   "en",
		region:     "",
		safeSearch: 0,
	}
	for _, opt := range opts {
		opt(y)
	}
	y.domain = y.computeDomain()
	return y
}

func (*Yahoo) Name() string {
	return "yahoo"
}

func (y *Yahoo) Search(ctx context.Context, query string, page int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}

	searchURL := y.buildURL(query, page)

	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	header.Set("Referer", "https://google.com/")
	header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("Cookie", "sB="+y.buildSBCookie())

	body, err := y.client.Do(ctx, http.MethodGet, searchURL, header, nil)
	if err != nil {
		return nil, ErrRequestBlocked
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, ErrMalformedResponse
	}

	const noResultsSelector = ".compNoResult"
	if doc.Find(noResultsSelector).Length() > 0 {
		return nil, nil
	}

	results := y.parseResults(doc, page)

	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}

func (y *Yahoo) buildURL(query string, page int) string {
	q := url.QueryEscape(query)
	if page <= 1 {
		return fmt.Sprintf("https://%s/search/?p=%s", y.domain, q)
	}
	b := (page-1)*10 + 1
	return fmt.Sprintf("https://%s/search/?p=%s&b=%d", y.domain, q, b)
}

func (y *Yahoo) computeDomain() string {
	langCode := yahooLanguages[y.language]
	if langCode == "" {
		langCode = "any"
	}
	if y.region != "" {
		if d, ok := yahooRegion2domain[y.region]; ok {
			return d
		}
	}
	if d, ok := yahooLang2domain[langCode]; ok {
		return d
	}
	return fmt.Sprintf("%s.search.yahoo.com", langCode)
}

func (y *Yahoo) buildSBCookie() string {
	langCode := yahooLanguages[y.language]
	if langCode == "" {
		langCode = "any"
	}
	vm := map[int]string{0: "p", 1: "i", 2: "r"}[y.safeSearch]
	vl := fmt.Sprintf("lang_%s", langCode)
	return fmt.Sprintf("v=1&vm=%s&fl=1&vl=%s&pn=10&rw=new&userset=1", vm, vl)
}

func (y *Yahoo) parseResults(doc *goquery.Document, page int) []SearchResult {
	const (
		resultSelector = "div.algo"
		titleSelector  = "h3.title"
		urlSelector    = ".compTitle a"
		descSelector   = ".compText.aAbs p"
	)

	var results []SearchResult

	doc.Find(resultSelector).Each(func(i int, sel *goquery.Selection) {
		urlSel := sel.Find(urlSelector).First()
		rawURL, exists := urlSel.Attr("href")
		if !exists || rawURL == "" {
			return
		}
		fullURL := parseYahooRedirectURL(rawURL)

		titleSel := sel.Find(titleSelector).First()
		title := strings.TrimSpace(titleSel.Text())
		// fallback to aria-label if title empty
		if title == "" {
			if aria, exists := titleSel.Attr("aria-label"); exists {
				title = strings.TrimSpace(aria)
			}
		}

		description := strings.TrimSpace(sel.Find(descSelector).First().Text())

		if title == "" && fullURL == "" {
			return
		}

		results = append(results, SearchResult{
			Title:       title,
			URL:         fullURL,
			Description: description,
			Provider:    y.Name(),
			Page:        page,
			Position:    i,
		})
	})

	return results
}

// parseYahooRedirectURL extracts the real URL from Yahoo's redirect wrapper.
func parseYahooRedirectURL(rawURL string) string {
	startIdx := strings.Index(rawURL, "/RU=")
	if startIdx == -1 {
		return rawURL
	}
	encodedStart := rawURL[startIdx+4:]
	endMarkers := []string{"/RS", "/RK"}
	endIdx := len(encodedStart)
	for _, marker := range endMarkers {
		if idx := strings.Index(encodedStart, marker); idx != -1 && idx < endIdx {
			endIdx = idx
		}
	}
	if endIdx == 0 {
		return rawURL
	}
	encodedURL := encodedStart[:endIdx]
	decoded, err := url.QueryUnescape(encodedURL)
	if err != nil {
		return rawURL
	}
	return decoded
}
