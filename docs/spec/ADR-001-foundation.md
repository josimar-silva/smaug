# ADR-001: Smaug Foundation

**Status:** Accepted
**Date:** 2025-11-23

## Context

Homelab servers running GPU-intensive services (Ollama, Marker) consume significant power when idle. These servers should sleep when not in use and wake on-demand when requests arrive.

Need a reverse proxy that:
- Routes requests to backend services
- Wakes sleeping servers via Wake-on-LAN
- Is transparent to clients
- Has minimal resource footprint for K8s deployment

## Decision

Build **Smaug** - a config-driven reverse proxy with automatic WoL capability in **Go**.

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         K8s Cluster                             │
│                                                                 │
│  ┌─────────────┐         ┌────────────────────────────────┐     │
│  │  OpenWebUI  │────────▶│            Smaug               │     │
│  └─────────────┘         │                                │     │
│                          │  ┌──────────────────────────┐  │     │
│  ┌─────────────┐         │  │     Route Manager        │  │     │
│  │   Elrond    │────────▶│  │  (goroutine per port)    │  │     │
│  │   Agent     │         │  └────────────┬─────────────┘  │     │
│  └─────────────┘         │               │                │     │
│                          │  ┌────────────▼─────────────┐  │     │
│                          │  │     Health Checker       │  │     │
│                          │  │   (HTTP GET, 2s timeout) │  │     │
│                          │  └────────────┬─────────────┘  │     │
│                          │               │                │     │
│                          │       ┌───────┴───────┐        │     │
│                          │       │               │        │     │
│                          │  ┌────▼────┐    ┌─────▼─────┐  │     │
│                          │  │   WoL   │    │  Reverse  │  │     │
│                          │  │ Manager │    │   Proxy   │  │     │
│                          │  └────┬────┘    └─────┬─────┘  │     │
│                          │       │               │        │     │
│                          └───────┼───────────────┼────────┘     │
│                                  │               │              │
└──────────────────────────────────┼───────────────┼──────────────┘
                                   │               │
                          UDP Magic Packet    HTTP Request
                          (broadcast:9)            │
                                   │               │
                    ┌──────────────┼───────────────┼──────────────┐
                    │              ▼               ▼              │
                    │         ┌─────────────────────────┐         │
                    │         │    Backend Server       │         │
                    │         │    (Saruman/Morgoth)    │         │
                    │         │                         │         │
                    │         │  • Ollama  (11434)      │         │
                    │         │  • Marker  (8080)       │         │
                    │         │  • Whisper (9000)       │         │
                    │         └─────────────────────────┘         │
                    │                                             │
                    │              AI Server (GPU)                │
                    └─────────────────────────────────────────────┘
```

#### Request Flow

```
┌──────────┐     ┌───────────┐     ┌─────────────┐     ┌──────────┐
│  Client  │────▶│  Smaug    │────▶│  Backend    │────▶│ Response │
└──────────┘     └─────┬─────┘     └──────▲──────┘     └──────────┘
                       │                  │
                       │ unhealthy?       │
                       ▼                  │
                 ┌───────────┐            │
                 │  Send WoL │            │
                 └─────┬─────┘            │
                       │                  │
                       ▼                  │
                 ┌───────────┐            │
                 │   Poll    │────────────┘
                 │  health   │  (until healthy
                 └───────────┘   or timeout)
```

### Configuration Format

```yaml
servers:
  <name>:
    mac: "XX:XX:XX:XX:XX:XX"
    broadcast: "192.168.1.255"
    wake_timeout_secs: 60

routes:
  - name: <identifier>
    listen: <port>
    upstream: "http://host:port"
    server: <server-name>
    health_path: "/health"
```

### Core Components

| Component | Responsibility |
|-----------|----------------|
| Config Loader | Parse YAML, validate on startup |
| Route Manager | Spawn goroutine per listen port |
| Health Checker | HTTP GET with 2s timeout |
| WoL Manager | Send magic packets, debounce (5s) |
| Reverse Proxy | `httputil.ReverseProxy` to upstream |

### Request Flow

1. Request arrives on route port
2. Health check upstream
3. If unhealthy: send WoL, poll until healthy or timeout
4. Proxy request to upstream
5. Return response

### WoL Behavior

- Magic packet: 6×0xFF + 16×MAC via UDP port 9
- Debounce: 5s minimum between sends per server
- Timeout: configurable per server (default 60s)
- Failure: return 503 Service Unavailable

### Error Responses

| Scenario | Status |
|----------|--------|
| Wake timeout | 503 |
| Upstream unreachable | 502 |
| Config error | Exit |

### Environment Variables

- `SMAUG_CONFIG` - config path (default: `/etc/smaug/routes.yaml`)
- `SMAUG_LOG_LEVEL` - debug/info/warn/error (default: `info`)

## Consequences

### Positive

- Reduced power consumption (servers sleep when idle)
- Simple YAML config - easy to add/modify routes
- Go provides good concurrency primitives for multiple listeners
- Learning opportunity for Go networking patterns

### Negative

- Added latency on first request (wake time: 30-90s)
- Requires host networking in K8s for WoL broadcast
- Single point of failure (mitigate with replicas + leader election later)

### Risks

- WoL packets may not traverse network segments (requires L2 adjacency or relay)
- Backend server may fail to wake (BIOS/NIC misconfiguration)

## Future Work

- Prometheus metrics
- Config reload (SIGHUP)
- Graceful shutdown
- WebSocket/gRPC support
- Scheduled pre-warming

## References

- [Wake-on-LAN Specification](https://en.wikipedia.org/wiki/Wake-on-LAN)
- [Go httputil.ReverseProxy](https://pkg.go.dev/net/http/httputil#ReverseProxy)
