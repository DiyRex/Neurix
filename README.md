# Neurix Ollama NVIDIA GPU & Temperature Stats Exporter

Prometheus metrics exporter for the [Ollama](https://ollama.ai) LLM runtime.

Exports Ollama model inventory, loaded model memory, process stats, and full NVIDIA GPU telemetry — compatible with standard `nvidia_smi_*` Grafana dashboards out of the box.

## Quick Start

```bash
# Download a release binary (Linux amd64)
tar xzf ollama_exporter_<version>_linux_amd64.tar.gz
./ollama_exporter
# Auto-selects a free port from 9101–9160 and prints:
# {"level":"INFO","msg":"Metrics available","url":"http://localhost:9101/metrics"}
```

Override the port:

```bash
./ollama_exporter --web.listen-address=:9400
```

Docker:

```bash
docker pull diyrex5224/ollama_exporter:latest
docker run -p 9101:9101 diyrex5224/ollama_exporter:latest
```

## Flags

| Flag | Default | Env | Description |
|------|---------|-----|-------------|
| `--ollama.host` | `http://localhost:11434` | `OLLAMA_HOST` | Ollama API base URL |
| `--ollama.timeout` | `10s` | `OLLAMA_TIMEOUT` | Timeout for Ollama API calls |
| `--web.listen-address` | `auto` | — | Listen address. `auto` picks first free port in 9101–9160 |
| `--web.telemetry-path` | `/metrics` | — | Path to expose metrics on |
| `--web.config.file` | — | — | TLS / basic-auth config file (see `web.config.yml`) |
| `--log.level` | `info` | — | Log level (debug, info, warn, error) |

## Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: ollama
    static_configs:
      - targets: ['localhost:9101']
```

## Metrics Reference

### Ollama

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ollama_up` | Gauge | — | 1 if Ollama API is reachable |
| `ollama_version_info` | Gauge=1 | `version` | Ollama build version |
| `ollama_models_available_total` | Gauge | — | Number of models on disk |
| `ollama_model_size_bytes` | Gauge | `model` | On-disk size per model |
| `ollama_models_loaded_total` | Gauge | — | Number of models loaded in memory |
| `ollama_model_memory_bytes` | Gauge | `model` | RAM used by each loaded model |
| `ollama_model_vram_bytes` | Gauge | `model` | VRAM used by each loaded model |
| `ollama_process_cpu_seconds_total` | Counter | — | CPU time consumed by Ollama process |
| `ollama_process_resident_memory_bytes` | Gauge | — | RSS of Ollama process |
| `ollama_scrape_duration_seconds` | Gauge | — | Duration of last scrape |
| `ollama_scrape_success` | Gauge | — | 1 if last scrape succeeded |

When Ollama is not running, `ollama_up 0` is emitted and the exporter continues running.

### NVIDIA GPU (`nvidia_smi_*`)

Labels on all NVIDIA metrics: `gpu_index`, `gpu_name`, `gpu_uuid`

| Metric | Description |
|--------|-------------|
| `nvidia_smi_utilization_gpu_ratio` | GPU utilization (0–1) |
| `nvidia_smi_utilization_memory_ratio` | Memory controller utilization (0–1) |
| `nvidia_smi_memory_used_bytes` | VRAM used |
| `nvidia_smi_memory_total_bytes` | Total VRAM |
| `nvidia_smi_memory_free_bytes` | VRAM free |
| `nvidia_smi_temperature_gpu` | GPU core temperature °C |
| `nvidia_smi_temperature_memory` | Memory temperature °C |
| `nvidia_smi_power_draw_watts` | Current power draw W |
| `nvidia_smi_power_limit_watts` | Power limit W |
| `nvidia_smi_fan_speed_ratio` | Fan speed (0–1) |
| `nvidia_smi_clock_graphics_hz` | Graphics clock Hz |
| `nvidia_smi_clock_memory_hz` | Memory clock Hz |
| `nvidia_smi_clock_sm_hz` | SM clock Hz |
| `nvidia_smi_encoder_session_count` | Active NVENC sessions |
| `nvidia_smi_encoder_fps` | NVENC output FPS |
| `nvidia_smi_encoder_latency_us` | NVENC latency µs |
| `nvidia_smi_pcie_link_gen_current` | PCIe generation |
| `nvidia_smi_pcie_link_width_current` | PCIe link width (lanes) |
| `nvidia_smi_ecc_errors_corrected_volatile_total` | Corrected ECC errors (volatile) |
| `nvidia_smi_ecc_errors_uncorrected_volatile_total` | Uncorrected ECC errors (volatile) |
| `nvidia_smi_ecc_errors_corrected_aggregate_total` | Corrected ECC errors (aggregate) |
| `nvidia_smi_ecc_errors_uncorrected_aggregate_total` | Uncorrected ECC errors (aggregate) |

GPU metrics are auto-detected: NVIDIA (`nvidia-smi`) → AMD (`rocm-smi`) → disabled. No configuration needed.

Fields that return `N/A` or `Not Supported` from nvidia-smi are silently skipped rather than emitting zero.

### AMD GPU

| Metric | Labels | Description |
|--------|--------|-------------|
| `ollama_gpu_utilization_ratio` | `index`, `name` | GPU utilization (0–1) |
| `ollama_gpu_memory_used_bytes` | `index`, `name` | VRAM used |
| `ollama_gpu_memory_total_bytes` | `index`, `name` | Total VRAM |

## TLS and Basic Auth

Copy and edit `web.config.yml` from the release archive, then:

```bash
./ollama_exporter --web.config.file=web.config.yml
```

Example `web.config.yml`:

```yaml
tls_server_config:
  cert_file: server.crt
  key_file:  server.key

basic_auth_users:
  prometheus: $2y$10$...   # bcrypt hash
```

## Building from Source

Requirements: Go 1.23+

```bash
git clone https://github.com/DiyRex/Neurix.git
cd Neurix
make build          # produces ./ollama_exporter
make test           # go test -race -cover ./...
make lint           # golangci-lint (if installed)
```

## Deployment

### Kubernetes

```bash
kubectl apply -f deploy/kubernetes/deployment.yaml
```

A `ServiceMonitor` for Prometheus Operator is at `deploy/kubernetes/servicemonitor.yaml`.

### systemd

```bash
sudo cp ollama_exporter /usr/local/bin/
sudo cp deploy/systemd/ollama-exporter.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ollama-exporter
```

## Release

Releases are published automatically when a `v*.*.*` tag is pushed:

```bash
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions runs `goreleaser` to produce Linux/macOS/Windows binaries and uploads them to GitHub Releases. Docker images for `linux/amd64` and `linux/arm64` are pushed to `ghcr.io/diyrex/ollama_exporter`.

## Architecture

```
cmd/ollama_exporter/main.go   — flag wiring, auto-port, web server
internal/scraper/             — HTTP client for Ollama API (mockable interface)
internal/collector/           — Prometheus collectors (ollama, gpu, process)
deploy/                       — Docker, Kubernetes, systemd artifacts
```

Ollama API calls to `/api/version`, `/api/tags`, and `/api/ps` are made in parallel using `errgroup` to minimise scrape latency.
