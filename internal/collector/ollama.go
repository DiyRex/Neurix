package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/diyrex/ollama_exporter/internal/scraper"
)

const namespace = "ollama"

// OllamaCollector implements prometheus.Collector.
// It delegates data acquisition to a Scraper (mockable in tests).
type OllamaCollector struct {
	scraper scraper.Scraper
	gpu     *GPUCollector
	proc    *ProcessCollector
	timeout time.Duration
	logger  *slog.Logger

	upDesc              *prometheus.Desc
	versionInfoDesc     *prometheus.Desc
	modelsAvailableDesc *prometheus.Desc
	modelSizeBytesDesc  *prometheus.Desc
	modelsLoadedDesc    *prometheus.Desc
	modelMemBytesDesc   *prometheus.Desc
	modelVRAMBytesDesc  *prometheus.Desc
	scrapeDurationDesc  *prometheus.Desc
	scrapeSuccessDesc   *prometheus.Desc
}

func NewOllamaCollector(s scraper.Scraper, gpu *GPUCollector, proc *ProcessCollector, timeout time.Duration, logger *slog.Logger) *OllamaCollector {
	return &OllamaCollector{
		scraper: s,
		gpu:     gpu,
		proc:    proc,
		timeout: timeout,
		logger:  logger,
		upDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"1 if the Ollama API is reachable, 0 otherwise.",
			nil, nil,
		),
		versionInfoDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "version_info"),
			"Ollama version info. Value is always 1.",
			[]string{"version"}, nil,
		),
		modelsAvailableDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "models_available_total"),
			"Number of models available on disk.",
			nil, nil,
		),
		modelSizeBytesDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "model_size_bytes"),
			"On-disk size of a model in bytes.",
			[]string{"model"}, nil,
		),
		modelsLoadedDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "models_loaded_total"),
			"Number of models currently loaded in memory.",
			nil, nil,
		),
		modelMemBytesDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "model_memory_bytes"),
			"RAM used by a loaded model in bytes.",
			[]string{"model"}, nil,
		),
		modelVRAMBytesDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "model_vram_bytes"),
			"VRAM used by a loaded model in bytes.",
			[]string{"model"}, nil,
		),
		scrapeDurationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "scrape_duration_seconds"),
			"Duration of the last Ollama scrape in seconds.",
			nil, nil,
		),
		scrapeSuccessDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "scrape_success"),
			"1 if the last scrape of Ollama metrics succeeded.",
			nil, nil,
		),
	}
}

func (c *OllamaCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.upDesc
	ch <- c.versionInfoDesc
	ch <- c.modelsAvailableDesc
	ch <- c.modelSizeBytesDesc
	ch <- c.modelsLoadedDesc
	ch <- c.modelMemBytesDesc
	ch <- c.modelVRAMBytesDesc
	ch <- c.scrapeDurationDesc
	ch <- c.scrapeSuccessDesc
	c.gpu.Describe(ch)
	c.proc.Describe(ch)
}

func (c *OllamaCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	data, err := c.scraper.Scrape(ctx)
	duration := time.Since(start).Seconds()

	if err != nil {
		c.logger.Error("scrape failed", "err", err)
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccessDesc, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.scrapeDurationDesc, prometheus.GaugeValue, duration)
		ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, 0)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeDurationDesc, prometheus.GaugeValue, duration)
	ch <- prometheus.MustNewConstMetric(c.scrapeSuccessDesc, prometheus.GaugeValue, 1)

	upVal := 0.0
	if data.Up {
		upVal = 1.0
	}
	ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, upVal)

	if !data.Up {
		return
	}

	ch <- prometheus.MustNewConstMetric(c.versionInfoDesc, prometheus.GaugeValue, 1, data.Version)

	if data.Tags != nil {
		ch <- prometheus.MustNewConstMetric(c.modelsAvailableDesc, prometheus.GaugeValue, float64(len(data.Tags.Models)))
		for _, m := range data.Tags.Models {
			ch <- prometheus.MustNewConstMetric(c.modelSizeBytesDesc, prometheus.GaugeValue, float64(m.Size), m.Name)
		}
	}

	if data.PS != nil {
		ch <- prometheus.MustNewConstMetric(c.modelsLoadedDesc, prometheus.GaugeValue, float64(len(data.PS.Models)))
		for _, m := range data.PS.Models {
			ch <- prometheus.MustNewConstMetric(c.modelMemBytesDesc, prometheus.GaugeValue, float64(m.Size), m.Name)
			ch <- prometheus.MustNewConstMetric(c.modelVRAMBytesDesc, prometheus.GaugeValue, float64(m.SizeVRAM), m.Name)
		}
	}

	c.gpu.Collect(ch)
	c.proc.Collect(ch)
}
