package collector_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/diyrex/ollama_exporter/internal/collector"
	"github.com/diyrex/ollama_exporter/internal/scraper"
)

// mockScraper is an in-process Scraper for tests — no network required.
type mockScraper struct{ data *scraper.OllamaData }

func (m *mockScraper) Scrape(_ context.Context) (*scraper.OllamaData, error) {
	return m.data, nil
}

func newTestCollector(data *scraper.OllamaData) *collector.OllamaCollector {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &mockScraper{data: data}
	gpu := collector.NewGPUCollector(logger)
	proc := collector.NewProcessCollector(logger)
	return collector.NewOllamaCollector(s, gpu, proc, 5*time.Second, logger)
}

func TestCollectorWhenUp(t *testing.T) {
	c := newTestCollector(&scraper.OllamaData{
		Up:      true,
		Version: "0.5.11",
		Tags: &scraper.TagsResponse{Models: []scraper.ModelInfo{
			{Name: "llama3.2:latest", Size: 2_000_000_000},
		}},
		PS: &scraper.PSResponse{Models: []scraper.RunningModel{
			{Name: "llama3.2:latest", Size: 2_000_000_000, SizeVRAM: 1_500_000_000},
		}},
	})

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	found := map[string]bool{}
	for _, mf := range mfs {
		found[mf.GetName()] = true
	}

	required := []string{
		"ollama_up",
		"ollama_version_info",
		"ollama_models_available_total",
		"ollama_models_loaded_total",
		"ollama_model_size_bytes",
		"ollama_model_memory_bytes",
		"ollama_model_vram_bytes",
		"ollama_scrape_duration_seconds",
		"ollama_scrape_success",
	}
	for _, name := range required {
		if !found[name] {
			t.Errorf("expected metric %q not found", name)
		}
	}
}

func TestCollectorWhenDown(t *testing.T) {
	c := newTestCollector(&scraper.OllamaData{Up: false})

	expected := `
# HELP ollama_up 1 if the Ollama API is reachable, 0 otherwise.
# TYPE ollama_up gauge
ollama_up 0
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "ollama_up"); err != nil {
		t.Errorf("ollama_up mismatch: %v", err)
	}
}

func TestCollectorScrapeSuccess(t *testing.T) {
	c := newTestCollector(&scraper.OllamaData{Up: true, Version: "1.0.0",
		Tags: &scraper.TagsResponse{}, PS: &scraper.PSResponse{}})

	expected := `
# HELP ollama_scrape_success 1 if the last scrape of Ollama metrics succeeded.
# TYPE ollama_scrape_success gauge
ollama_scrape_success 1
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "ollama_scrape_success"); err != nil {
		t.Errorf("scrape_success mismatch: %v", err)
	}
}
