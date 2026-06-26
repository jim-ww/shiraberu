package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func init() {
	register(NewStartpage(NewHTTPClient()))
}

type startpage struct {
	client   HttpProvider
	language string
	region   string
	// 0 = none, 1 = moderate, 2 = heavy
	safeSearch int
	scCache    struct {
		sync.RWMutex
		token string
		valid time.Time
	}
}

type StartpageOption func(*startpage)

// WithStartpageLanguage sets the UI and search language.
func WithStartpageLanguage(lang string) StartpageOption {
	return func(s *startpage) {
		s.language = lang
	}
}

// WithStartpageRegion sets the search results region (e.g., "us", "de", "all").
func WithStartpageRegion(region string) StartpageOption {
	return func(s *startpage) {
		s.region = region
	}
}

// WithStartpageSafeSearch sets the safe‑search level: 0 = none, 1 = moderate, 2 = heavy.
func WithStartpageSafeSearch(level int) StartpageOption {
	if level < 0 || level > 2 {
		level = 0
	}
	return func(s *startpage) {
		s.safeSearch = level
	}
}

func NewStartpage(client HttpProvider, opts ...StartpageOption) *startpage {
	s := &startpage{
		client:     client,
		language:   "english",
		region:     "all",
		safeSearch: 0,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (*startpage) Name() string {
	return "startpage"
}

func (s *startpage) Search(ctx context.Context, query string, page int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}

	// Get fresh SC token
	scToken, err := s.getScToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get SC token: %w", err)
	}

	formData := s.buildFormData(query, page, scToken)

	cookie := s.buildPreferencesCookie()

	header := http.Header{}
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	header.Set("Origin", "https://www.startpage.com")
	header.Set("Referer", "https://www.startpage.com/")
	header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("DNT", "1")
	header.Set("Connection", "keep-alive")
	header.Set("Upgrade-Insecure-Requests", "1")
	header.Set("Cookie", cookie)

	body, err := s.client.Do(ctx, http.MethodPost, "https://www.startpage.com/sp/search", header, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, ErrRequestBlocked
	}

	bodyStr := string(body)

	if strings.Contains(bodyStr, "captcha") || strings.Contains(bodyStr, "CAPTCHA") {
		return nil, ErrCaptcha
	}

	results, err := s.parseJSONResults(bodyStr, page)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	// Fallback to HTML parsing if JSON parsing fails
	return s.parseHTMLResults(bodyStr)
}

// getScToken fetches a fresh SC token from Startpage's homepage.
func (s *startpage) getScToken(ctx context.Context) (string, error) {
	// Check cache first
	s.scCache.RLock()
	if time.Now().Before(s.scCache.valid) && s.scCache.token != "" {
		token := s.scCache.token
		s.scCache.RUnlock()
		return token, nil
	}
	s.scCache.RUnlock()

	// Fetch new token
	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	header.Set("Accept-Language", "en-US,en;q=0.9")

	body, err := s.client.Do(ctx, http.MethodGet, "https://www.startpage.com/", header, nil)
	if err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}

	var token string
	doc.Find(`form#search input[name="sc"]`).Each(func(i int, sel *goquery.Selection) {
		if val, exists := sel.Attr("value"); exists && val != "" {
			token = val
		}
	})

	if token == "" {
		// Try alternative selector
		doc.Find(`form[action="/sp/search"] input[name="sc"]`).Each(func(i int, sel *goquery.Selection) {
			if val, exists := sel.Attr("value"); exists && val != "" {
				token = val
			}
		})
	}

	if token == "" {
		return "", errors.New("SC token not found in homepage")
	}

	// Cache the token
	s.scCache.Lock()
	s.scCache.token = token
	s.scCache.valid = time.Now().Add(30 * time.Minute)
	s.scCache.Unlock()

	return token, nil
}

// buildFormData constructs the POST form data.
func (s *startpage) buildFormData(query string, page int, scToken string) url.Values {
	data := url.Values{}
	data.Set("query", query)
	data.Set("cat", "web")
	data.Set("t", "device")
	data.Set("sc", scToken)
	data.Set("with_date", "")
	data.Set("abd", "1")
	data.Set("abe", "1")
	data.Set("qsr", "all")
	data.Set("qadf", s.safeSearchMap())
	data.Set("language", s.language)
	data.Set("lui", s.language)

	if page > 1 {
		data.Set("page", strconv.Itoa(page))
		data.Set("segment", "startpage.udog")
	}

	return data
}

// buildPreferencesCookie constructs the preferences cookie.
func (s *startpage) buildPreferencesCookie() string {
	safe := s.safeSearchMap()
	cookie := map[string]string{
		"date_time":                   "world",
		"disable_family_filter":       safe,
		"disable_open_in_new_window":  "0",
		"enable_post_method":          "1",
		"enable_proxy_safety_suggest": "1",
		"enable_stay_control":         "1",
		"instant_answers":             "1",
		"lang_homepage":               "s/device/en/",
		"num_of_results":              "10",
		"suggestions":                 "1",
		"wt_unit":                     "celsius",
	}
	if s.language != "" {
		cookie["language"] = s.language
		cookie["language_ui"] = s.language
	}
	if s.region != "" && s.region != "all" {
		cookie["search_results_region"] = s.region
	}

	var parts []string
	for k, v := range cookie {
		parts = append(parts, k+"EEE"+v)
	}
	return "preferences=" + strings.Join(parts, "N1N")
}

func (s *startpage) safeSearchMap() string {
	switch s.safeSearch {
	case 1:
		return "moderate"
	case 2:
		return "heavy"
	default:
		return "none"
	}
}

// parseJSONResults extracts results from the JSON embedded in Startpage's response.
func (s *startpage) parseJSONResults(body string, page int) ([]SearchResult, error) {
	re := regexp.MustCompile(`React\.createElement\(UIStartpage\.AppSerpWeb,\s*({.*?})\)`)
	matches := re.FindStringSubmatch(body)
	if len(matches) < 2 {
		return nil, errors.New("no serp data found")
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(matches[1]), &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	render, ok := data["render"].(map[string]any)
	if !ok {
		return nil, errors.New("render not found")
	}
	presenter, ok := render["presenter"].(map[string]any)
	if !ok {
		return nil, errors.New("presenter not found")
	}
	regions, ok := presenter["regions"].(map[string]any)
	if !ok {
		return nil, errors.New("regions not found")
	}
	mainline, ok := regions["mainline"].([]any)
	if !ok {
		return nil, errors.New("mainline not found")
	}

	var results []SearchResult

	for _, categ := range mainline {
		category, ok := categ.(map[string]any)
		if !ok {
			continue
		}
		displayType, _ := category["display_type"].(string)
		if displayType != "web-google" {
			continue
		}
		items, ok := category["results"].([]any)
		if !ok {
			continue
		}
		for i, item := range items {
			result, ok := item.(map[string]any)
			if !ok {
				continue
			}
			title, _ := result["title"].(string)
			clickURL, _ := result["clickUrl"].(string)
			description, _ := result["description"].(string)
			if clickURL == "" {
				continue
			}
			title = s.cleanHTML(title)
			description = s.cleanHTML(description)

			results = append(results, SearchResult{
				Title:       title,
				URL:         clickURL,
				Description: description,
				Provider:    s.Name(),
				Position:    i,
				Page:        page,
			})
		}
	}

	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}

// parseHTMLResults is a fallback parser for when JSON parsing fails.
func (s *startpage) parseHTMLResults(body string) ([]SearchResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, ErrMalformedResponse
	}

	const noResultsSelector = ".no-results"
	if doc.Find(noResultsSelector).Length() > 0 {
		return nil, nil
	}

	const (
		resultSelector = ".w-gl__result__main"
		titleSelector  = ".w-gl__result-second-line-container>.w-gl__result-title>h3"
		urlSelector    = ".w-gl__result-url"
		descSelector   = ".w-gl__description"
	)

	var results []SearchResult
	doc.Find(resultSelector).Each(func(i int, sel *goquery.Selection) {
		title := strings.TrimSpace(sel.Find(titleSelector).First().Text())
		u := strings.TrimSpace(sel.Find(urlSelector).First().Text())
		desc := strings.TrimSpace(sel.Find(descSelector).First().Text())
		if u == "" {
			return
		}
		results = append(results, SearchResult{
			Title:       title,
			URL:         u,
			Description: desc,
			Provider:    "startpage",
			Position:    i,
			Page:        0, // fallback doesn't know page
		})
	})

	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}

// cleanHTML removes HTML tags from a string.
func (s *startpage) cleanHTML(text string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(text))
	if err != nil {
		re := regexp.MustCompile(`<[^>]*>`)
		return strings.TrimSpace(re.ReplaceAllString(text, ""))
	}
	return strings.TrimSpace(doc.Text())
}
