package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func init() {
	register(NewYandex(NewHTTPClient()))
}

type yandex struct {
	client HttpProvider
	// searchType: "web" or "images"
	searchType string
	language   string
}

type YandexOption func(*yandex)

// WithYandexSearchType sets the search type ("web" or "images").
func WithYandexSearchType(searchType string) YandexOption {
	return func(y *yandex) {
		if searchType == "web" || searchType == "images" {
			y.searchType = searchType
		}
	}
}

// WithYandexLanguage sets the language (e.g., "en", "ru").
func WithYandexLanguage(lang string) YandexOption {
	return func(y *yandex) {
		y.language = lang
	}
}

func NewYandex(client HttpProvider, opts ...YandexOption) *yandex {
	y := &yandex{
		client:     client,
		searchType: "web",
		language:   "en",
	}
	for _, opt := range opts {
		opt(y)
	}
	return y
}

func (*yandex) Name() string {
	return "yandex"
}

func (y *yandex) Search(ctx context.Context, query string, page int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}

	searchURL := y.buildURL(query, page)

	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("Referer", "https://yandex.com/")
	header.Set("Cookie", "yp=1716337604.sp.family%3A0#1685406411.szm.1:1920x1080:1920x999")

	body, err := y.client.Do(ctx, http.MethodGet, searchURL, header, nil)
	if err != nil {
		return nil, ErrRequestBlocked
	}

	if strings.Contains(string(body), "captcha") || strings.Contains(string(body), "CAPTCHA") {
		return nil, ErrCaptcha
	}

	if y.searchType == "web" {
		return y.parseWebResults(body, page)
	}
	return y.parseImageResults(body, page)
}

func (y *yandex) buildURL(query string, page int) string {
	if y.searchType == "web" {
		params := url.Values{}
		params.Set("tmpl_version", "releases")
		params.Set("text", query)
		params.Set("web", "1")
		params.Set("frame", "1")
		params.Set("searchid", "3131712")
		if y.language != "" {
			params.Set("lang", y.language)
		}
		if page > 1 {
			params.Set("p", strconv.Itoa(page-1))
		}
		return "https://yandex.com/search/site/?" + params.Encode()
	}

	// Images
	params := url.Values{}
	params.Set("text", query)
	params.Set("uinfo", "sw-1920-sh-1080-ww-1125-wh-999")
	if page > 1 {
		params.Set("p", strconv.Itoa(page-1))
	}
	return "https://yandex.com/images/search?" + params.Encode()
}

func (y *yandex) parseWebResults(body []byte, page int) ([]SearchResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, ErrMalformedResponse
	}

	var results []SearchResult

	// Selector: li.serp-item
	doc.Find(`li[class*="serp-item"]`).Each(func(i int, s *goquery.Selection) {
		// URL: .b-serp-item__title-link
		urlSel := s.Find(`a.b-serp-item__title-link`)
		url, exists := urlSel.Attr("href")
		if !exists || url == "" {
			return
		}

		title := strings.TrimSpace(s.Find(`h3.b-serp-item__title a.b-serp-item__title-link span`).First().Text())
		if title == "" {
			// Fallback: try the anchor's text
			title = strings.TrimSpace(urlSel.First().Text())
		}

		description := strings.TrimSpace(s.Find(`div.b-serp-item__content div.b-serp-item__text`).First().Text())

		results = append(results, SearchResult{
			Title:       title,
			URL:         url,
			Description: description,
			Provider:    y.Name(),
			Page:        page,
			Position:    i,
		})
	})

	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}

// parseImageResults parses Yandex image search results (HTML with embedded JSON).
func (y *yandex) parseImageResults(body []byte, page int) ([]SearchResult, error) {
	htmlStr := string(body)

	// Extract the JSON data embedded in the page
	// Pattern: {"location":"/images/search/...advRsyaSearchColumn":null}}
	// or {"location":"/images/search/...false}}}
	jsonData := y.extractImageJSON(htmlStr)
	if jsonData == "" {
		return nil, ErrMalformedResponse
	}

	var data yandexImageResponse
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return nil, ErrMalformedResponse
	}

	var results []SearchResult

	for _, item := range data.InitialState.SerpList.Items.Entities {
		title := item.Snippet.Title
		sourceURL := item.Snippet.URL

		// Find the best image source
		imageSource := item.ViewerData.Thumb
		for _, dup := range item.ViewerData.Dups {
			if dup.H > imageSource.H {
				imageSource = dup
			}
		}
		for _, preview := range item.ViewerData.Preview {
			if preview.H > imageSource.H {
				imageSource = preview
			}
		}

		results = append(results, SearchResult{
			Title:       title,
			URL:         sourceURL,
			Description: fmt.Sprintf("%d x %d", imageSource.W, imageSource.H),
			Provider:    "yandex",
			Page:        page,
			Position:    len(results),
		})
		_ = imageSource.URL // could be used for thumbnail
		_ = item.Image      // could be used for thumbnail
	}

	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}

// extractImageJSON extracts the JSON data from Yandex image search HTML.
func (y *yandex) extractImageJSON(htmlStr string) string {
	// Try first pattern: {"location":"/images/search/...advRsyaSearchColumn":null}}
	re := regexp.MustCompile(`{"location":"/images/search/.*?advRsyaSearchColumn":null}}`)
	match := re.FindString(htmlStr)
	if match != "" {
		return match
	}

	// Try second pattern: {"location":"/images/search/...false}}}
	re2 := regexp.MustCompile(`{"location":"/images/search/.*?false}}}`)
	match = re2.FindString(htmlStr)
	if match != "" {
		return match
	}

	return ""
}

// JSON structs for Yandex image search.
type yandexImageResponse struct {
	InitialState struct {
		SerpList struct {
			Items struct {
				Entities map[string]yandexImageItem `json:"entities"`
			} `json:"items"`
		} `json:"serpList"`
	} `json:"initialState"`
}

type yandexImageItem struct {
	Snippet struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"snippet"`
	Image      string `json:"image"`
	ViewerData struct {
		Thumb   yandexImageSize   `json:"thumb"`
		Dups    []yandexImageSize `json:"dups"`
		Preview []yandexImageSize `json:"preview"`
	} `json:"viewerData"`
}

type yandexImageSize struct {
	URL string `json:"url"`
	W   int    `json:"w"`
	H   int    `json:"h"`
}
