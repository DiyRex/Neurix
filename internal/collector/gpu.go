package collector

import (
	"bufio"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

type gpuType int

const (
	gpuTypeNone   gpuType = iota
	gpuTypeNVIDIA
	gpuTypeAMD
)

// GPUCollector emits per-GPU utilization and memory metrics.
// It auto-detects nvidia-smi → rocm-smi → disabled.
type GPUCollector struct {
	kind            gpuType
	utilizationDesc *prometheus.Desc
	memUsedDesc     *prometheus.Desc
	memTotalDesc    *prometheus.Desc
	logger          *slog.Logger
}

func NewGPUCollector(logger *slog.Logger) *GPUCollector {
	g := &GPUCollector{
		kind:   detectGPU(logger),
		logger: logger,
		utilizationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "gpu", "utilization_ratio"),
			"GPU utilization ratio (0–1).",
			[]string{"index", "name"}, nil,
		),
		memUsedDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "gpu", "memory_used_bytes"),
			"GPU memory currently used in bytes.",
			[]string{"index", "name"}, nil,
		),
		memTotalDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "gpu", "memory_total_bytes"),
			"Total GPU memory in bytes.",
			[]string{"index", "name"}, nil,
		),
	}
	return g
}

func detectGPU(logger *slog.Logger) gpuType {
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		logger.Info("GPU: nvidia-smi detected")
		return gpuTypeNVIDIA
	}
	if _, err := exec.LookPath("rocm-smi"); err == nil {
		logger.Info("GPU: rocm-smi detected")
		return gpuTypeAMD
	}
	logger.Info("GPU: no GPU tooling found, GPU metrics disabled")
	return gpuTypeNone
}

func (g *GPUCollector) Describe(ch chan<- *prometheus.Desc) {
	if g.kind == gpuTypeNone {
		return
	}
	ch <- g.utilizationDesc
	ch <- g.memUsedDesc
	ch <- g.memTotalDesc
}

func (g *GPUCollector) Collect(ch chan<- prometheus.Metric) {
	switch g.kind {
	case gpuTypeNVIDIA:
		g.collectNvidia(ch)
	case gpuTypeAMD:
		g.collectAMD(ch)
	}
}

// nvidia-smi output: "0, NVIDIA RTX 3090, 45, 8192, 24576"
func (g *GPUCollector) collectNvidia(ch chan<- prometheus.Metric) {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=index,name,utilization.gpu,memory.used,memory.total",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		g.logger.Debug("nvidia-smi failed", "err", err)
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ", ", 5)
		if len(parts) != 5 {
			continue
		}
		idx := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		util, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		memUsed, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		memTotal, _ := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)

		ch <- prometheus.MustNewConstMetric(g.utilizationDesc, prometheus.GaugeValue, util/100.0, idx, name)
		ch <- prometheus.MustNewConstMetric(g.memUsedDesc, prometheus.GaugeValue, memUsed*1024*1024, idx, name)
		ch <- prometheus.MustNewConstMetric(g.memTotalDesc, prometheus.GaugeValue, memTotal*1024*1024, idx, name)
	}
}

// rocm-smi CSV: header line, then one row per GPU.
func (g *GPUCollector) collectAMD(ch chan<- prometheus.Metric) {
	out, err := exec.Command("rocm-smi", "--showuse", "--showmeminfo", "vram", "--csv").Output()
	if err != nil {
		g.logger.Debug("rocm-smi failed", "err", err)
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Scan() // skip header
	for i := 0; scanner.Scan(); i++ {
		parts := strings.Split(scanner.Text(), ",")
		if len(parts) < 4 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		util, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		memUsed, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		memTotal, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		idx := strconv.Itoa(i)

		ch <- prometheus.MustNewConstMetric(g.utilizationDesc, prometheus.GaugeValue, util/100.0, idx, name)
		ch <- prometheus.MustNewConstMetric(g.memUsedDesc, prometheus.GaugeValue, memUsed, idx, name)
		ch <- prometheus.MustNewConstMetric(g.memTotalDesc, prometheus.GaugeValue, memTotal, idx, name)
	}
}
