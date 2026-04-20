package main

import (
	"fmt"
	"log/slog"
	"net"
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

		// "auto" triggers port scanning in range 9101–9160.
		// Explicit value (e.g. :9400) bypasses scanning.
		toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, "auto")
	)

	promslogConfig := &promslog.Config{}
	promslogflag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.Version(version.Print("ollama_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promslog.New(promslogConfig)

	// Resolve listen address — auto-select port from 9101–9160 if needed.
	addrs := *toolkitFlags.WebListenAddresses
	if len(addrs) == 1 && addrs[0] == "auto" {
		port, err := findAvailablePort(9101, 9160)
		if err != nil {
			logger.Error("auto port selection failed", "err", err)
			os.Exit(1)
		}
		*toolkitFlags.WebListenAddresses = []string{fmt.Sprintf(":%d", port)}
	}

	listenAddr := (*toolkitFlags.WebListenAddresses)[0]
	url := resolveMetricsURL(listenAddr, *metricsPath)

	logger.Info("Starting ollama_exporter", "version", Version, "host", *ollamaHost)
	logger.Info("Metrics available", "url", url)

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
			Links:       []web.LandingLinks{{Address: *metricsPath, Text: "Metrics"}},
		})
		if err != nil {
			logger.Error("landing page error", "err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}

	srv := &http.Server{}
	if err := web.ListenAndServe(srv, toolkitFlags, logger); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}

// findAvailablePort returns the first TCP port in [start, end] that is not in use.
func findAvailablePort(start, end int) (int, error) {
	for p := start; p <= end; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			ln.Close()
			return p, nil
		}
	}
	return 0, fmt.Errorf("no available port in range %d–%d", start, end)
}

// resolveMetricsURL turns a listen address (":9101", "0.0.0.0:9101", "[::]:9101")
// into a human-readable http://localhost:PORT/path URL.
func resolveMetricsURL(addr, path string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://localhost" + addr + path
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%s%s", host, port, path)
}
