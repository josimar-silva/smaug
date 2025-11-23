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

<p align="center">
  <a href="https://github.com/josimar-silva/smaug/blob/main/LICENSE"><img src="https://img.shields.io/github/license/josimar-silva/smaug?style=flat-square&color=blue" alt="License"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go" alt="Go"></a>
</p>

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

```
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

