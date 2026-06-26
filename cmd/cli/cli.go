package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/jim-ww/shiraberu"
	"github.com/jim-ww/shiraberu/cache/jsonfile"
	"github.com/jim-ww/shiraberu/providers"
	"golang.org/x/time/rate"
)

func main() {
	flag.Usage = func() {
		fmt.Println("Usage: shiraberu [flags] query")
		fmt.Println()
		flag.PrintDefaults()
	}
	rateLimit := flag.Duration("r", time.Millisecond*200, "rate limit between requests (e.g., 200ms)")
	enabledProviders := flag.String("p", "ddg", fmt.Sprintf("comma-separated providers (e.g, %s)", strings.Join(providers.All(), ",")))
	minResults := flag.Int("min", 0, "minimum results per search")
	maxResults := flag.Int("max", 0, "maximum results per search")
	startFromPage := flag.Int("page", 0, "starting page number")
	timeout := flag.Duration("t", 0, "request timeout (e.g., 30s)")
	format := flag.String("f", "plaintext", "output format (e.g., plaintext/json)")
	omitFields := flag.String("omit", "", "fields to omit (title,desc,url). only with plaintext format")
	entriesSep := flag.String("separator", "\n", "result entry separator")
	fieldsSep := flag.String("field-separator", "|", "entry fields separator")
	enableCache := flag.Bool("C", true, "enable caching")
	cachePath := flag.String("d", filepath.Join(getDataHome(), "shiraberu"), "cache directory path")
	cacheTTL := flag.Duration("ttl", 15*time.Minute, "cache entry TTL (e.g., 15m)")
	cacheMaxEntries := flag.Int("max-cache", 1000, "maximum cache entries")
	debug := flag.Bool("v", false, "verbose output")
	flag.Parse()

	if len(flag.Args()) == 0 {
		log.Fatal("query cannot be empty")
	}
	query := strings.Join(flag.Args(), " ")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if *timeout > 0 {
		var stop func()
		ctx, stop = context.WithDeadline(ctx, time.Now().Add(*timeout))
		defer stop()
	}

	opts := []shiraberu.Option{}

	if *debug {
		opts = append(opts, shiraberu.WithDebugLogging())
	}

	if *enableCache {
		cache, err := jsonfile.New(*cachePath, *cacheTTL, *cacheMaxEntries)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cache close:", err)
		}
		defer func() {
			if err := cache.Close(); err != nil {
				log.Fatal(err)
			}
		}()
		opts = append(opts, shiraberu.WithCache(cache))
	}

	providersList := []providers.SearchProvider{}
	for str := range strings.SplitSeq(*enabledProviders, ",") {
		provider, valid := providers.GetProviderFromString(str)
		if valid {
			providersList = append(providersList, provider)
		} else {
			log.Fatalf("unknown provider: %s", str)
		}
	}
	if len(providersList) == 0 {
		log.Fatal("no providers set, check if -providers format is correct")
	}
	opts = append(opts, shiraberu.WithProviders(providersList...))

	if *rateLimit > 0 {
		ratelimiters := map[string]*rate.Limiter{}
		for _, provider := range providersList {
			ratelimiters[provider.Name()] = rate.NewLimiter(rate.Every(*rateLimit), 1)
		}
		opts = append(opts, shiraberu.WithRatelimiters(ratelimiters))
	}

	req := shiraberu.SearchRequest{
		Query:         query,
		MinResults:    *minResults,
		MaxResults:    *maxResults,
		StartFromPage: *startFromPage,
	}

	results, err := shiraberu.Search(ctx, req, opts...)
	if err != nil {
		log.Fatal(err)
	}

	if len(results.Results) == 0 {
		select {
		case <-ctx.Done():
			fmt.Println("requests timeout")
			os.Exit(0)
		default:
		}
		log.Fatal("no results found")
	}

	switch *format {
	case "json":
		json, err := json.Marshal(results)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(string(json))
		os.Exit(0)
	default:
		fmt.Print(formatResults(results.Results, *omitFields, *fieldsSep, *entriesSep))
	}
}
