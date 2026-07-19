# 調べる · shiraberu

> *"to search"* — a multi-engine search aggregator written in Go

Query DuckDuckGo, Startpage, Bing, Brave, and more in parallel. Results are deduplicated, merged, and returned as a clean sorted list. **No API keys required** — it scrapes the engines' web interfaces directly.

Use it as a CLI tool or import it as a Go library.

---

## Features

- **Parallel search** across multiple providers simultaneously
- **Built-in providers:** DuckDuckGo, Startpage, Bing, Yahoo, Brave, Reddit, SepiaSearch, Wikipedia, Yandex
- **Deduplication** across provider results
- **Caching** with configurable TTL and max entries
- **Rate limiting** per provider
- **Flexible output:** human-readable plaintext or machine-readable JSON

---

## Quick Start

```bash
go install github.com/jim-ww/shiraberu/cmd/shiraberu@latest
shiraberu golang concurrency
```

Or with Nix:

```bash
nix run github:jim-ww/shiraberu golang concurrency
```

---

## CLI Usage

```
Usage: shiraberu [flags] query

Flags:
  -p string          comma-separated providers (default "ddg")
                     available: ddg, startpage, bing, yahoo, brave, reddit, sepiasearch, wikipedia, yandex
  -f string          output format: plaintext or json (default "plaintext")
  -max int           maximum results to return (0 = unlimited)
  -min int           minimum results before stopping requests (0 = unlimited)
  -C                 enable caching (default true)
  -d string          cache directory (default: XDG_DATA_HOME/shiraberu)
  -ttl duration      cache entry TTL (default 15m)
  -max-cache int     maximum cache entries (default 1000)
  -r duration        rate limit between requests (default 200ms)
  -t duration        request timeout
  -separator string        result separator (default "\n")
  -field-separator string  field separator within a result (default "|")
  -emit string       fields to show: title, desc, url (plaintext only) (default "url")
  -url string        fetch this URL and print its content as GitHub-flavored markdown, skipping search
  -v                 verbose output
```

### Examples

Search with two providers, return 10 results as JSON:

```bash
shiraberu -p ddg,startpage -max 10 -f json "learn go"
```

Throttle to 1 req/sec, show only title and URL:

```bash
shiraberu -p ddg -r 1s -emit title,url -field-separator " -> " "learn go"
```

Fetch a page and print it as GitHub-flavored markdown:

```bash
shiraberu -url https://go.dev/
```

### Output Format

**Plaintext** (default) — one result per line:

```
The Go Programming Language | Official site for the Go language | https://go.dev/
```

Control layout with `-separator` (between results) and `-field-separator` (between fields). Choose which fields to show with `-emit url,title,desc`.

**JSON** — single object with results array plus metadata:

```json
{
  "results": [
    {
      "title": "The Go Programming Language",
      "url": "https://go.dev/",
      "description": "Go is an open source programming language...",
      "page": 0,
      "position": 0,
      "provider": "duckduckgo"
    }
  ],
  "has_more": true,
  "requests_made": 1
}
```

---

## Installation

**Via `go install`:**

```bash
go install github.com/jim-ww/shiraberu/cmd/shiraberu@latest
```

**Build from source:**

```bash
git clone https://github.com/jim-ww/shiraberu.git
cd shiraberu
go build ./cmd/shiraberu
sudo mv shiraberu /usr/local/bin/
```

**Pre-built binaries** for Linux, macOS, and Windows are available on the [Releases](https://github.com/jim-ww/shiraberu/releases) page.

---

## Library Usage

### Aggregated search across multiple providers

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/jim-ww/shiraberu"
    "github.com/jim-ww/shiraberu/cache/jsonfile"
    "github.com/jim-ww/shiraberu/providers"
    "golang.org/x/time/rate"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    cache, _ := jsonfile.New("./cache", 15*time.Minute, 1000)
    defer cache.Close()

    ddg := providers.NewDuckDuckGo(providers.NewHTTPClient())

    lims := map[string]*rate.Limiter{
        ddg.Name(): rate.NewLimiter(rate.Every(1*time.Second), 1),
    }

    resp, err := shiraberu.Search(ctx, shiraberu.SearchRequest{
        Query:      "golang",
        MaxResults: 10,
        MinResults: 3,
    },
        shiraberu.WithProviders(ddg),
        shiraberu.WithCache(cache),
        shiraberu.WithRatelimiters(lims),
    )
    if err != nil {
        panic(err)
    }

    for _, r := range resp.Results {
        fmt.Printf("%s\n  %s\n", r.Title, r.URL)
    }
}
```

> **Note:** Caching, rate limiting, and debug logging are disabled by default in library mode. Attach them explicitly via option functions.

### Using a single provider directly

Every provider can be used standalone without the aggregator:

```go
package main

import (
    "context"
    "fmt"

    "github.com/jim-ww/shiraberu/providers"
)

func main() {
    client := providers.NewHTTPClient()
    startpage := providers.NewStartpage(
        client,
        providers.WithStartpageLanguage("english"),
        providers.WithStartpageSafeSearch(1),
    )

    results, err := startpage.Search(context.Background(), "hacker news", 0)
    if err != nil {
        panic(err)
    }

    for _, r := range results {
        fmt.Printf("%s - %s\n", r.Title, r.URL)
    }
}
```

### Fetching a URL as markdown

```go
package main

import (
    "context"
    "fmt"

    "github.com/jim-ww/shiraberu"
    "github.com/jim-ww/shiraberu/providers"
)

func main() {
    markdown, err := shiraberu.FetchURLAsMarkdown(context.Background(), providers.NewHTTPClient(), "https://go.dev/")
    if err != nil {
        panic(err)
    }

    fmt.Println(markdown)
}
```

Each provider may expose its own configuration options — see the [`providers`](./providers) package for details.

---

## Available Providers

| Flag         | Provider     |
|--------------|--------------|
| `ddg`        | DuckDuckGo   |
| `startpage`  | Startpage    |
| `bing`       | Bing         |
| `yahoo`      | Yahoo        |
| `brave`      | Brave Search |
| `reddit`     | Reddit       |
| `sepiasearch`| SepiaSearch  |
| `wikipedia`  | Wikipedia    |
| `yandex`     | Yandex       |

---

## Contributing

Adding a new provider means implementing the `SearchProvider` interface. See the existing providers in [`providers/`](./providers) for examples. Pull requests are welcome.

---

## Support the Project

If shiraberu saves you time, consider a small donation:

**Monero (XMR)**
```
83YGRqP8uHed6NeegZQeX9ccCxbzoRHHEEi7pTwk4aqdJZEVXXA6NWtetnsEM2v33zFBBt3Rp6DNhU9qhJEGPspU14yN8t7
```

---

## License

Licensed under the [GNU Affero General Public License v3 (AGPLv3)](./LICENSE).
Free to use, study, share, and modify — provided you keep the same freedoms for others.
