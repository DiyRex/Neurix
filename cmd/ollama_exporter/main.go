package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	gocollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	promslogflag "github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/diyrex/ollama_exporter/internal/collector"
	"github.com/diyrex/ollama_exporter/internal/scraper"
)

// Version is injected at build time via ldflags.
var Version = "dev"

func main() {
	var (
		ollamaHost    = kingpin.Flag("ollama.host", "Ollama API base URL.").Default("http://localhost:11434").Envar("OLLAMA_HOST").String()
		ollamaTimeout = kingpin.Flag("ollama.timeout", "Timeout for Ollama API calls.").Default("10s").Envar("OLLAMA_TIMEOUT").Duration()
		metricsPath   = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()

		// --web.listen-address, --web.config.file are wired by kingpinflag (exporter-toolkit).
		// web.config.file supports TLS and basic auth via a YAML file.
		toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, ":9400")
	)

	promslogConfig := &promslog.Config{}
	promslogflag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.Version(version.Print("ollama_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promslog.New(promslogConfig)
	logger.Info("Starting ollama_exporter", "version", Version, "host", *ollamaHost)

	// --- Collector chain ---
	httpClient := &http.Client{Timeout: *ollamaTimeout + 2*time.Second}
	s := scraper.NewHTTPScraper(*ollamaHost, httpClient)
	gpu := collector.NewGPUCollector(logger)
	proc := collector.NewProcessCollector(logger)
	ollamaCol := collector.NewOllamaCollector(s, gpu, proc, *ollamaTimeout, logger)

	// Separate registry: target metrics only, no self-contamination.
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		ollamaCol,
		gocollectors.NewGoCollector(),
		gocollectors.NewProcessCollector(gocollectors.ProcessCollectorOpts{}),
	)

	// --- HTTP routes ---
	http.Handle(*metricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}))

	if *metricsPath != "/" {
		landingPage, err := web.NewLandingPage(web.LandingConfig{
			Name:        "Ollama Exporter",
			Description: "Prometheus metrics exporter for the Ollama LLM runtime",
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{Address: *metricsPath, Text: "Metrics"},
			},
		})
		if err != nil {
			logger.Error("landing page error", "err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}

	// exporter-toolkit ListenAndServe wires up TLS + basic auth from --web.config.file
	srv := &http.Server{}
	if err := web.ListenAndServe(srv, toolkitFlags, logger); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}
