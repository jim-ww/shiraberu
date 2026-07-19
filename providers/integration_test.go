//go:build integration

package providers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestProvidersIntegration hits the real providers over the network and
// checks that each one returns usable results for a basic query.
//
// Run with: go test -tags=integration ./providers/...
func TestProvidersIntegration(t *testing.T) {
	for _, name := range All() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			provider, valid := GetProviderFromString(name)
			if !valid {
				t.Fatalf("provider %q not registered", name)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			results, err := provider.Search(ctx, "golang", 0)
			if errors.Is(err, ErrRequestBlocked) || errors.Is(err, ErrCaptcha) {
				t.Skipf("provider blocked this environment: %v", err)
			}
			if err != nil {
				t.Fatalf("search: %v", err)
			}

			if len(results) == 0 {
				t.Fatalf("expected at least one result, got none")
			}

			for i, r := range results {
				if strings.TrimSpace(r.URL) == "" {
					t.Errorf("result %d: empty URL", i)
				}
				if strings.TrimSpace(r.Title) == "" {
					t.Errorf("result %d (%s): empty title", i, r.URL)
				}
				if r.Provider != name {
					t.Errorf("result %d (%s): provider = %q, want %q", i, r.URL, r.Provider, name)
				}
			}
		})
	}
}
