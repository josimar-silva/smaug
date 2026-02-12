<p align="center">
  <img src="docs/assets/logo.png" alt="SMAUG Logo" width="200"/>
</p>

<h1 align="center">SMAUG</h1>

<p align="center">
  <b>S</b>mart <b>M</b>essenger for <b>A</b>uto-waking <b>U</b>nwake <b>G</b>Hz
</p>

<p align="center">
  <i>Config-driven reverse proxy with automatic Wake-on-LAN for homelab services</i>
</p>

<p align="center">
  <i>"I am fire. I am... asleep until you need me."</i>
</p>

<div align="center"></div>

<div align="center">
  <!-- MIT License -->
  <a href="./LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License"></a>
  <!-- Go version -->
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go" alt="Go"></a>
 <!-- Version -->
  <a href="./">
    <img src="https://img.shields.io/badge/smaug-v0.1.0-orange.svg" alt="smaug" />
  </a>
  <!-- Go Report Card -->
  <a href="https://goreportcard.com/report/github.com/josimar-silva/smaug">
    <img src="https://goreportcard.com/badge/github.com/josimar-silva/smaug" alt="smaug go report card" />
  </a>
  <!-- OSSF Score Card -->
  <a href="https://scorecard.dev/viewer/?uri=github.com/josimar-silva/smaug">
    <img src="https://img.shields.io/ossf-scorecard/github.com/josimar-silva/smaug?label=openssf+scorecard" alt="OpenSSF Score Card">
  </a>
  <!-- Coverage -->
  <a href="https://sonarcloud.io/summary/new_code?id=josimar-silva_smaug">
    <img src="https://sonarcloud.io/api/project_badges/measure?project=josimar-silva_smaug&metric=coverage" alt="coverage" />
  </a>
  <!-- Smaug Health -->
  <a href="https://hello.from-gondor.com/">
    <img src="https://status.from-gondor.com/api/v1/endpoints/internal_smaug/health/badge.svg" alt="smaug Health" />
  </a>
  <!-- Smaug Uptime -->
  <a href="https://hello.from-gondor.com/">
    <img src="https://status.from-gondor.com/api/v1/endpoints/internal_smaug/uptimes/30d/badge.svg" alt="smaug Uptime" />
  </a>
  <!-- Smaug Response Time -->
  <a href="https://hello.from-gondor.com/">
    <img src="https://status.from-gondor.com/api/v1/endpoints/internal_smaug/response-times/30d/badge.svg" alt="smaug Response Time" />
  </a>
  <!-- CD -->
  <a href="https://github.com/josimar-silva/smaug/actions/workflows/cd.yaml">
    <img src="https://github.com/josimar-silva/smaug/actions/workflows/cd.yaml/badge.svg" alt="continuous delivery" />
  </a>
  <!-- CI -->
  <a href="https://github.com/josimar-silva/smaug/actions/workflows/ci.yaml">
    <img src="https://github.com/josimar-silva/smaug/actions/workflows/ci.yaml/badge.svg" alt="continuous integration" />
  </a>
  <!-- Docker -->
  <a href="https://github.com/josimar-silva/smaug/actions/workflows/docker.yaml">
    <img src="https://github.com/josimar-silva/smaug/actions/workflows/docker.yaml/badge.svg" alt="docker" />
  </a>
  <!-- CodeQL Advanced -->
  <a href="https://github.com/josimar-silva/smaug/actions/workflows/codeql.yaml">
    <img src="https://github.com/josimar-silva/smaug/actions/workflows/codeql.yaml/badge.svg" alt="CodeQL" />
  </a>
</div>

---

## Overview

Smaug is a lightweight reverse proxy that wakes backend servers on-demand via Wake-on-LAN.

Designed for homelab environments where GPU servers should sleep when idle to reduce power consumption.

**Key Features:**
- Routes requests to backend services transparently
- Wakes sleeping servers automatically when requests arrive
- YAML-based configuration
- Prometheus metrics for observability
- Minimal resource footprint for K8s deployment

## Architecture

```text
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│   Client    │────────▶│    Smaug    │────────▶│   Backend   │
└─────────────┘         └──────┬──────┘         └─────────────┘
                               │
                          WoL Packet
                          (if needed)
```

## Configuration

```yaml
servers:
  saruman:
    mac: "AA:BB:CC:DD:EE:FF"
    broadcast: "192.168.1.255"
    wake_timeout_secs: 60

routes:
  - name: ollama
    listen: 11434
    upstream: "http://saruman:11434"
    server: saruman
    health_path: "/api/tags"
```

## Getting Started

### Prerequisites

- Go 1.23+
- Backend servers with Wake-on-LAN enabled

### Installation

```bash
# Clone repository
git clone https://github.com/josimar-silva/smaug.git
cd smaug

# Build
go build -o smaug ./cmd/smaug

# Run
SMAUG_CONFIG=routes.yaml ./smaug
```

### Docker

```bash
docker run -d \
  --network host \
  -v /path/to/routes.yaml:/etc/smaug/routes.yaml \
  ghcr.io/josimar-silva/smaug:latest
```

## Documentation

- [ADR-001: Foundation](docs/spec/ADR-001-foundation.md) - Architecture decisions and specification

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

