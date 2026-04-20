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

// nvidiaQuery is the full set of fields requested from nvidia-smi.
// Fields order must match nvidiaRow struct parsing below.
const nvidiaQuery = "index,name,uuid," +
	"utilization.gpu,utilization.memory," +
	"memory.used,memory.total,memory.free," +
	"temperature.gpu,temperature.memory," +
	"power.draw,power.limit," +
	"fan.speed," +
	"clocks.current.graphics,clocks.current.memory,clocks.current.sm," +
	"encoder.stats.sessionCount,encoder.stats.averageFps,encoder.stats.averageLatency," +
	"pcie.link.gen.current,pcie.link.width.current," +
	"ecc.errors.corrected.volatile.total,ecc.errors.uncorrected.volatile.total," +
	"ecc.errors.corrected.aggregate.total,ecc.errors.uncorrected.aggregate.total"

// nvidiaFieldCount must match the number of comma-separated fields in nvidiaQuery.
const nvidiaFieldCount = 25

// GPUCollector emits per-GPU metrics.
// NVIDIA metrics use the nvidia_smi_* prefix to match standard Grafana dashboards.
// AMD metrics use the ollama_gpu_* prefix.
type GPUCollector struct {
	kind   gpuType
	logger *slog.Logger

	// NVIDIA — nvidia_smi_* namespace (compatible with standard dashboards)
	nvUtilGPU             *prometheus.Desc
	nvUtilMemory          *prometheus.Desc
	nvMemUsed             *prometheus.Desc
	nvMemTotal            *prometheus.Desc
	nvMemFree             *prometheus.Desc
	nvTempGPU             *prometheus.Desc
	nvTempMemory          *prometheus.Desc
	nvPowerDraw           *prometheus.Desc
	nvPowerLimit          *prometheus.Desc
	nvFanSpeed            *prometheus.Desc
	nvClockGraphics       *prometheus.Desc
	nvClockMemory         *prometheus.Desc
	nvClockSM             *prometheus.Desc
	nvEncoderSessions     *prometheus.Desc
	nvEncoderFPS          *prometheus.Desc
	nvEncoderLatency      *prometheus.Desc
	nvPCIeLinkGen         *prometheus.Desc
	nvPCIeLinkWidth       *prometheus.Desc
	nvECCCorrVolatile     *prometheus.Desc
	nvECCUncorrVolatile   *prometheus.Desc
	nvECCCorrAggregate    *prometheus.Desc
	nvECCUncorrAggregate  *prometheus.Desc

	// AMD — ollama_gpu_* namespace (basic; rocm-smi CSV varies by version)
	amdUtilization *prometheus.Desc
	amdMemUsed     *prometheus.Desc
	amdMemTotal    *prometheus.Desc
}

var nvLabels = []string{"gpu_index", "gpu_name", "gpu_uuid"}

func nvDesc(name, help string) *prometheus.Desc {
	return prometheus.NewDesc("nvidia_smi_"+name, help, nvLabels, nil)
}

func NewGPUCollector(logger *slog.Logger) *GPUCollector {
	return &GPUCollector{
		kind:   detectGPU(logger),
		logger: logger,

		nvUtilGPU:            nvDesc("utilization_gpu_ratio", "Fraction of time the GPU was executing (0–1)."),
		nvUtilMemory:         nvDesc("utilization_memory_ratio", "Fraction of time the memory controller was busy (0–1)."),
		nvMemUsed:            nvDesc("memory_used_bytes", "GPU framebuffer memory currently used in bytes."),
		nvMemTotal:           nvDesc("memory_total_bytes", "Total GPU framebuffer memory in bytes."),
		nvMemFree:            nvDesc("memory_free_bytes", "GPU framebuffer memory currently free in bytes."),
		nvTempGPU:            nvDesc("temperature_gpu", "GPU core temperature in degrees Celsius."),
		nvTempMemory:         nvDesc("temperature_memory", "GPU memory temperature in degrees Celsius."),
		nvPowerDraw:          nvDesc("power_draw_watts", "Current GPU power draw in watts."),
		nvPowerLimit:         nvDesc("power_limit_watts", "Enforced GPU power limit in watts."),
		nvFanSpeed:           nvDesc("fan_speed_ratio", "Fan speed as a fraction of maximum (0–1)."),
		nvClockGraphics:      nvDesc("clock_graphics_hz", "Current graphics (shader) clock in Hz."),
		nvClockMemory:        nvDesc("clock_memory_hz", "Current memory clock in Hz."),
		nvClockSM:            nvDesc("clock_sm_hz", "Current streaming-multiprocessor clock in Hz."),
		nvEncoderSessions:    nvDesc("encoder_session_count", "Number of active NVENC encoder sessions."),
		nvEncoderFPS:         nvDesc("encoder_fps", "Average NVENC encoder output frames per second."),
		nvEncoderLatency:     nvDesc("encoder_latency_us", "Average NVENC encoder latency in microseconds."),
		nvPCIeLinkGen:        nvDesc("pcie_link_gen_current", "Current PCIe link generation."),
		nvPCIeLinkWidth:      nvDesc("pcie_link_width_current", "Current PCIe link width (lanes)."),
		nvECCCorrVolatile:    nvDesc("ecc_errors_corrected_volatile_total", "ECC single-bit corrected errors since last driver reload."),
		nvECCUncorrVolatile:  nvDesc("ecc_errors_uncorrected_volatile_total", "ECC double-bit uncorrected errors since last driver reload."),
		nvECCCorrAggregate:   nvDesc("ecc_errors_corrected_aggregate_total", "ECC single-bit corrected errors since last reset."),
		nvECCUncorrAggregate: nvDesc("ecc_errors_uncorrected_aggregate_total", "ECC double-bit uncorrected errors since last reset."),

		amdUtilization: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "gpu", "utilization_ratio"),
			"AMD GPU utilization ratio (0–1).", []string{"index", "name"}, nil,
		),
		amdMemUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "gpu", "memory_used_bytes"),
			"AMD GPU memory currently used in bytes.", []string{"index", "name"}, nil,
		),
		amdMemTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "gpu", "memory_total_bytes"),
			"AMD total GPU memory in bytes.", []string{"index", "name"}, nil,
		),
	}
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
	switch g.kind {
	case gpuTypeNVIDIA:
		ch <- g.nvUtilGPU
		ch <- g.nvUtilMemory
		ch <- g.nvMemUsed
		ch <- g.nvMemTotal
		ch <- g.nvMemFree
		ch <- g.nvTempGPU
		ch <- g.nvTempMemory
		ch <- g.nvPowerDraw
		ch <- g.nvPowerLimit
		ch <- g.nvFanSpeed
		ch <- g.nvClockGraphics
		ch <- g.nvClockMemory
		ch <- g.nvClockSM
		ch <- g.nvEncoderSessions
		ch <- g.nvEncoderFPS
		ch <- g.nvEncoderLatency
		ch <- g.nvPCIeLinkGen
		ch <- g.nvPCIeLinkWidth
		ch <- g.nvECCCorrVolatile
		ch <- g.nvECCUncorrVolatile
		ch <- g.nvECCCorrAggregate
		ch <- g.nvECCUncorrAggregate
	case gpuTypeAMD:
		ch <- g.amdUtilization
		ch <- g.amdMemUsed
		ch <- g.amdMemTotal
	}
}

func (g *GPUCollector) Collect(ch chan<- prometheus.Metric) {
	switch g.kind {
	case gpuTypeNVIDIA:
		g.collectNvidia(ch)
	case gpuTypeAMD:
		g.collectAMD(ch)
	}
}

// parseField returns (value, true) or (0, false) when nvidia-smi returns "[N/A]" or "N/A".
func parseField(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "N/A" || s == "[N/A]" || s == "Not Supported" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func emit(ch chan<- prometheus.Metric, desc *prometheus.Desc, v float64, labels ...string) {
	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, labels...)
}

func (g *GPUCollector) collectNvidia(ch chan<- prometheus.Metric) {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu="+nvidiaQuery,
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		g.logger.Debug("nvidia-smi failed", "err", err)
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		// nvidia-smi uses ", " (comma-space) as separator
		parts := strings.Split(line, ", ")
		if len(parts) < nvidiaFieldCount {
			// fall back to splitting on "," without space
			parts = strings.Split(line, ",")
		}
		if len(parts) < nvidiaFieldCount {
			g.logger.Debug("nvidia-smi: unexpected field count", "line", line)
			continue
		}

		idx := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		uuid := strings.TrimSpace(parts[2])
		lbls := []string{idx, name, uuid}

		type field struct {
			desc  *prometheus.Desc
			raw   string
			scale float64 // multiplier applied before emitting; 1.0 = identity
		}
		fields := []field{
			{g.nvUtilGPU, parts[3], 1.0 / 100.0},
			{g.nvUtilMemory, parts[4], 1.0 / 100.0},
			{g.nvMemUsed, parts[5], 1024 * 1024},  // MiB → bytes
			{g.nvMemTotal, parts[6], 1024 * 1024},
			{g.nvMemFree, parts[7], 1024 * 1024},
			{g.nvTempGPU, parts[8], 1},
			{g.nvTempMemory, parts[9], 1},
			{g.nvPowerDraw, parts[10], 1},
			{g.nvPowerLimit, parts[11], 1},
			{g.nvFanSpeed, parts[12], 1.0 / 100.0},
			{g.nvClockGraphics, parts[13], 1_000_000}, // MHz → Hz
			{g.nvClockMemory, parts[14], 1_000_000},
			{g.nvClockSM, parts[15], 1_000_000},
			{g.nvEncoderSessions, parts[16], 1},
			{g.nvEncoderFPS, parts[17], 1},
			{g.nvEncoderLatency, parts[18], 1},
			{g.nvPCIeLinkGen, parts[19], 1},
			{g.nvPCIeLinkWidth, parts[20], 1},
			{g.nvECCCorrVolatile, parts[21], 1},
			{g.nvECCUncorrVolatile, parts[22], 1},
			{g.nvECCCorrAggregate, parts[23], 1},
			{g.nvECCUncorrAggregate, parts[24], 1},
		}

		for _, f := range fields {
			if v, ok := parseField(f.raw); ok {
				emit(ch, f.desc, v*f.scale, lbls...)
			}
		}
	}
}

// collectAMD collects basic AMD GPU metrics via rocm-smi CSV.
// CSV format varies by rocm-smi version; adjust column indices if needed.
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
		idx := strconv.Itoa(i)
		if util, ok := parseField(parts[1]); ok {
			emit(ch, g.amdUtilization, util/100.0, idx, name)
		}
		if used, ok := parseField(parts[2]); ok {
			emit(ch, g.amdMemUsed, used, idx, name)
		}
		if total, ok := parseField(parts[3]); ok {
			emit(ch, g.amdMemTotal, total, idx, name)
		}
	}
}
