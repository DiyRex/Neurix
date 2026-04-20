package collector

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v3/process"
)

// ProcessCollector emits CPU and RSS metrics for the Ollama OS process.
// Uses gopsutil for cross-platform compatibility (Linux, macOS, Windows).
type ProcessCollector struct {
	cpuSecondsDesc *prometheus.Desc
	rssDesc        *prometheus.Desc
	logger         *slog.Logger
}

func NewProcessCollector(logger *slog.Logger) *ProcessCollector {
	return &ProcessCollector{
		cpuSecondsDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "process", "cpu_seconds_total"),
			"Total CPU seconds consumed by the Ollama process.",
			nil, nil,
		),
		rssDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "process", "resident_memory_bytes"),
			"Resident set size of the Ollama process in bytes.",
			nil, nil,
		),
		logger: logger,
	}
}

func (p *ProcessCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- p.cpuSecondsDesc
	ch <- p.rssDesc
}

func (p *ProcessCollector) Collect(ch chan<- prometheus.Metric) {
	pid, err := findOllamaPID()
	if err != nil {
		p.logger.Debug("ollama process not found", "err", err)
		return
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		p.logger.Debug("could not open ollama process", "pid", pid, "err", err)
		return
	}

	if times, err := proc.Times(); err == nil {
		ch <- prometheus.MustNewConstMetric(p.cpuSecondsDesc, prometheus.CounterValue, times.User+times.System)
	}

	if mem, err := proc.MemoryInfo(); err == nil {
		ch <- prometheus.MustNewConstMetric(p.rssDesc, prometheus.GaugeValue, float64(mem.RSS))
	}
}

func findOllamaPID() (int32, error) {
	procs, err := process.Processes()
	if err != nil {
		return 0, err
	}
	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}
		if strings.HasPrefix(strings.ToLower(name), "ollama") {
			return p.Pid, nil
		}
	}
	return 0, fmt.Errorf("ollama process not found")
}
