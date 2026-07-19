package shiraberu

import (
	"context"
	"fmt"
	"net/http"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"

	"github.com/jim-ww/shiraberu/providers"
)

// FetchURLAsMarkdown fetches the given URL and converts its HTML body into
// GitHub-flavored markdown.
func FetchURLAsMarkdown(ctx context.Context, client *providers.HTTPClient, url string) (string, error) {
	header := http.Header{
		"User-Agent": []string{"Mozilla/5.0 (compatible; shiraberu/1.0)"},
	}

	body, err := client.Do(ctx, http.MethodGet, url, header, nil)
	if err != nil {
		return "", fmt.Errorf("fetch url: %w", err)
	}

	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
			strikethrough.NewStrikethroughPlugin(),
		),
	)

	markdown, err := conv.ConvertString(string(body), converter.WithDomain(url))
	if err != nil {
		return "", fmt.Errorf("convert html to markdown: %w", err)
	}

	return markdown, nil
}
