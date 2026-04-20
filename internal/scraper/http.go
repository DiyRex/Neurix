package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/sync/errgroup"
)

// HTTPScraper implements Scraper against a live Ollama instance.
type HTTPScraper struct {
	client  *http.Client
	baseURL string
}

func NewHTTPScraper(baseURL string, client *http.Client) *HTTPScraper {
	return &HTTPScraper{baseURL: baseURL, client: client}
}

// Scrape fetches /api/version first (fast reachability check), then
// fetches /api/tags and /api/ps in parallel via errgroup.
// Returns Up=false (no error) when Ollama is unreachable — soft-fail.
func (s *HTTPScraper) Scrape(ctx context.Context) (*OllamaData, error) {
	data := &OllamaData{}

	var ver VersionResponse
	if err := s.getJSON(ctx, "/api/version", &ver); err != nil {
		data.Up = false
		return data, nil
	}
	data.Up = true
	data.Version = ver.Version

	var tags TagsResponse
	var ps PSResponse

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.getJSON(gctx, "/api/tags", &tags)
	})
	g.Go(func() error {
		return s.getJSON(gctx, "/api/ps", &ps)
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("ollama API: %w", err)
	}

	data.Tags = &tags
	data.PS = &ps
	return data, nil
}

func (s *HTTPScraper) getJSON(ctx context.Context, path string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, path)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}
