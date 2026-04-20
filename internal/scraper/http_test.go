package scraper_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diyrex/ollama_exporter/internal/scraper"
)

func newMockOllama(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": "0.5.11"})
	})
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(scraper.TagsResponse{
			Models: []scraper.ModelInfo{
				{Name: "llama3.2:latest", Size: 2_000_000_000},
				{Name: "mistral:latest", Size: 4_000_000_000},
			},
		})
	})
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(scraper.PSResponse{
			Models: []scraper.RunningModel{
				{Name: "llama3.2:latest", Size: 2_000_000_000, SizeVRAM: 1_500_000_000},
			},
		})
	})
	return httptest.NewServer(mux)
}

func TestScrapeUp(t *testing.T) {
	srv := newMockOllama(t)
	defer srv.Close()

	s := scraper.NewHTTPScraper(srv.URL, srv.Client())
	data, err := s.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Scrape returned unexpected error: %v", err)
	}

	if !data.Up {
		t.Error("expected Up=true when Ollama is running")
	}
	if data.Version != "0.5.11" {
		t.Errorf("version: got %q, want 0.5.11", data.Version)
	}
	if data.Tags == nil || len(data.Tags.Models) != 2 {
		t.Errorf("tags: got %v models, want 2", len(data.Tags.Models))
	}
	if data.PS == nil || len(data.PS.Models) != 1 {
		t.Errorf("ps: got %v models, want 1", len(data.PS.Models))
	}
}

func TestScrapeDown(t *testing.T) {
	// Nothing listening on port 1 — Ollama is "down"
	s := scraper.NewHTTPScraper("http://127.0.0.1:1", &http.Client{})
	data, err := s.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Scrape should soft-fail, not error: %v", err)
	}
	if data.Up {
		t.Error("expected Up=false when Ollama is unreachable")
	}
}

func TestScrapeHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	s := scraper.NewHTTPScraper(srv.URL, srv.Client())
	data, err := s.Scrape(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Up {
		t.Error("expected Up=false on HTTP 500 from /api/version")
	}
}
