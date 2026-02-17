# ADR-001: SMAUG Foundation Architecture

**Status:** Draft (Refined)
**Date:** 2026-02-11
**Updated:** 2026-02-11

## Context

Homelab servers running GPU-intensive services (Ollama, Marker, Whisper) consume significant power (200-400W) when idle. These servers should sleep when not in use and wake on-demand when requests arrive, reducing power consumption by 90%+ during idle periods.

### Problem Statement

Need a reverse proxy solution that:
- Routes requests to backend services transparently
- Wakes sleeping servers automatically via Wake-on-LAN
- Puts idle servers to sleep after configurable timeout
- Has minimal resource footprint for K8s deployment
- Provides observability (metrics, health status)
- Supports configuration changes without downtime
- Follows security best practices (privilege separation)

### Design Constraints

1. **Language:** Go (for learning value and K8s ecosystem fit)
2. **Deployment:** Kubernetes cluster in homelab
3. **Privilege Model:** Unprivileged proxy + privileged WoL service
4. **Configuration:** YAML-based, file-driven
5. **Target Services:** GPU workloads (Ollama, Marker, Whisper)
6. **Network:** L2 adjacency available for WoL broadcast

## Decision

Build **SMAUG** (Smart Messenger for Auto-waking Unwake GHz) as a **two-service architecture**:

1. **SMAUG** - Unprivileged reverse proxy (this specification)
2. **Gwaihir** - Privileged WoL messenger service (separate repo: https://github.com/josimar-silva/gwaihir)

This privilege separation follows the principle of least privilege: SMAUG handles all application logic while Gwaihir performs only privileged WoL operations.

### High-Level Architecture

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          K8s Cluster (ai namespace)                         │
│                                                                             │
│  ┌─────────────┐         ┌───────────────────────┐                          │
│  │  OpenWebUI  │────────▶│       SMAUG           │                          │
│  └─────────────┘         │   (unprivileged)      │                          │
│                          │                       │                          │
│  ┌─────────────┐         │  ┌─────────────────┐  │     ┌───────────────┐    │
│  │   Elrond    │────────▶│  │ Config Manager  │  │     │   Gwaihir     │    │
│  │   Agent     │         │  │  (hot-reload)   │  │     │ (privileged)  │    │
│  └─────────────┘         │  └────────┬────────┘  │     │               │    │
│                          │           │           │     │ ┌───────────┐ │    │
│                          │  ┌────────▼────────┐  │ API │ │    WoL    │ │    │
│                          │  │   Route Manager │  ├────▶│ │  Sender   │ │    │
│                          │  │ (listener/port) │  │     │ └─────┬─────┘ │    │
│                          │  └────────┬────────┘  │     └───────┼───────┘    │
│                          │           │           │             │            │
│                          │  ┌────────▼────────┐  │             │            │
│                          │  │  Proxy Handler  │  │             │            │
│                          │  │  (per request)  │  │             │            │
│                          │  └────┬────────────┘  │             │            │
│                          │       │               │             │            │
│                          │  ┌────▼────┐  ┌───────▼──────┐      │            │
│                          │  │ Health  │  │    Idle      │      │            │
│                          │  │ Checker │  │   Tracker    │      │            │
│                          │  └────┬────┘  └───────┬──────┘      │            │
│                          │       │               │             │            │
│                          │  ┌────▼───────────────▼──────┐      │            │
│                          │  │   Metrics Exporter        │      │            │
│                          │  │   (Prometheus :2112)      │      │            │
│                          │  └───────────────────────────┘      │            │
│                          └─────────────────────────────────────┘            │
│                                       │               │                     │
└───────────────────────────────────────┼───────────────┼─────────────────────┘
                                        │               │
                                  HTTP Proxy      UDP Port 9
                                        │         (WoL Magic Packet)
                                        │               │
                         ┌──────────────┼───────────────┼──────────────┐
                         │              ▼               ▼              │
                         │         ┌────────────────────────┐          │
                         │         │   Backend Server       │          │
                         │         │   (Saruman/Morgoth)    │          │
                         │         │                        │          │
                         │         │  • Ollama  (11434)     │          │
                         │         │  • Marker  (8080)      │          │
                         │         │  • Whisper (9000)      │          │
                         │         │  • Sleep API (8000)    │          │
                         │         └────────────────────────┘          │
                         │                                             │
                         │          AI Server (GPU, WoL-enabled)       │
                         └─────────────────────────────────────────────┘
```

### Request Flow (Wake-on-Demand)

```text
┌──────────┐     ┌──────────────┐     ┌───────────┐     ┌──────────┐     ┌──────────┐
│  Client  │────▶│    SMAUG     │────▶│  Backend  │────▶│ Response │────▶│  Client  │
└──────────┘     │              │     └─────▲─────┘     └──────────┘     └──────────┘
                 │ 1. Health?   │           │
                 └──────┬───────┘           │
                        │                   │
                   unhealthy?               │
                        │                   │
                        ▼                   │
                 ┌─────────────┐            │
                 │  Gwaihir    │            │
                 │  API Call   │            │
                 │  (POST /wol)│            │
                 └──────┬──────┘            │
                        │                   │
                        ▼                   │
                 ┌─────────────┐            │
                 │  Send WoL   │────────────┘ (magic packet)
                 │  (Gwaihir)  │
                 └──────┬──────┘
                        │
                        ▼
                 ┌─────────────┐
                 │ Poll Health │────────────▶ (repeat until healthy or timeout)
                 │  (2s / 60s) │              
                 └─────────────┘
```

### Request Flow (Auto-Sleep)

```text
┌─────────────┐     ┌──────────────┐     ┌─────────────┐     ┌──────────┐
│  No Traffic │     │ Idle Tracker │     │  Sleep API  │     │  Server  │
│  (5 min)    │────▶│  Detects     │────▶│  POST call  │────▶│ Shutdown │
└─────────────┘     └──────────────┘     └─────────────┘     └──────────┘
                          │
                          ▼
                    ┌──────────┐
                    │  Metric  │
                    │  Emitted │
                    └──────────┘
```

---

## Configuration Format

```yaml
# Global settings
settings:
  gwaihir:
    url: "http://gwaihir-service.ai.svc.cluster.local"
    apiKey: "${GWAIHIR_API_KEY}"  # Env var substitution
    timeout: 5s

  metrics:
    enabled: true
    port: 2112

  logging:
    level: info
    format: json  # structured logging

# Server definitions
servers:
  saruman:

    wakeOnLan:
      enabled: true
      timeout: 60s
      debounce: 5s  # Min interval between WoL attempts

    sleepOnLan:
      enabled: true
      endpoint: "http://saruman.from-gondor.com:8000/sleep"
      idleTimeout: 5m  # Sleep after 5 min idle
      allowedHosts:    # SSRF protection
        - "*.from-gondor.com"
        - "192.168.1.*"

  morgoth:

    wakeOnLan:
      enabled: true
      timeout: 90s
    sleepOnLan:
      enabled: false

# Route definitions
routes:
  - name: ollama
    listen: 11434
    upstream: "http://saruman.from-gondor.com:11434"
    server: saruman
    healthCheck:
      path: "/api/tags"
      interval: 2s
      timeout: 2s

  - name: marker
    listen: 8080
    upstream: "http://saruman.from-gondor.com:8080"
    server: saruman
    healthCheck:
      path: "/health"
      interval: 2s
      timeout: 2s

  - name: whisper
    listen: 9000
    upstream: "http://morgoth.from-gondor.com:9000"
    server: morgoth
    healthCheck:
      path: "/health"
      interval: 2s
      timeout: 2s
```

---

## Technical Design Decisions

### 1. HTTP Framework: Go stdlib

**Decision:** Use `net/http` and `httputil.ReverseProxy` (no third-party framework)

**Rationale:**

| Factor | Evaluation |
|--------|------------|
| **Learning Value** | Teaches core Go HTTP patterns, middleware implementation, concurrency |
| **Architecture Fit** | SMAUG uses port-per-service model; complex routers (Gin/Echo/Chi) solve path-based routing we don't need |
| **K8s Ecosystem** | All K8s tooling uses stdlib (client-go, controller-runtime); learning transfers directly |
| **Dependencies** | Zero external dependencies for HTTP = long-term maintainability |
| **Performance** | `httputil.ReverseProxy` is production-grade, used by major projects |

**Alternatives Considered:**
- **Gin/Echo:** High-performance frameworks with middleware ecosystems, but overkill for port-based routing
- **Chi:** Lightweight stdlib-compatible router, closer match but still unnecessary
- **Fiber:** Uses fasthttp (non-stdlib), incompatible with K8s libraries - rejected

**Trade-offs:**
- **Pro:** Minimal abstraction, teaches Go fundamentals, no dependency churn
- **Con:** Manual middleware implementation (logging, recovery, metrics)
- **Mitigation:** Implement thin middleware layer following stdlib patterns

### 2. Gwaihir Integration: REST API with Authentication

**Decision:** SMAUG commands Gwaihir via REST API (`POST /wol`) with shared secret authentication

**Architecture:**
```go
type GwaihirClient struct {
    baseURL string
    apiKey  string
    client  *http.Client
}

func (g *GwaihirClient) Wake(ctx context.Context, machineId string) error {
    req := WakeRequest{machineId: machineId}
    httpReq, _ := http.NewRequestWithContext(ctx, "POST", g.baseURL+"/wol", marshal(req))
    httpReq.Header.Set("X-API-Key", g.apiKey)  // Shared secret auth

    resp, err := g.client.Do(httpReq)
    // Handle response...
}
```

**Security Controls:**
1. **Authentication:** Shared API key in `X-API-Key` header (stored in K8s Secret)
2. **Rate Limiting:** Gwaihir enforces 1 request/10s per server
3. **Network Policy:** K8s NetworkPolicy restricts Gwaihir ingress to SMAUG pods only
4. **Audit Logging:** All WoL requests logged with source IP, server, timestamp

**Rationale:**
- Privilege separation: SMAUG cannot send raw network packets
- Single responsibility: Gwaihir only sends WoL, SMAUG handles all application logic
- Auditability: Centralized WoL logging in Gwaihir
- Testability: Mock Gwaihir API in SMAUG tests

**Alternatives Considered:**
- **SMAUG sends WoL directly:** Requires `CAP_NET_RAW` or `hostNetwork`, violates least privilege
- **Message queue (NATS/Redis):** Adds infrastructure dependency, overkill for homelab
- **gRPC:** More complex than REST for simple command API

### 3. Hot-Reload: File Watcher with Atomic Config Swap

**Decision:** Watch config file for changes using `fsnotify`, reload atomically

**Implementation Pattern:**
```go
type ConfigManager struct {
    mu       sync.RWMutex
    config   *Config
    reloadCh chan struct{}
}

func (cm *ConfigManager) WatchConfig(path string) {
    watcher, _ := fsnotify.NewWatcher()
    watcher.Add(path)

    for event := range watcher.Events {
        if event.Op&fsnotify.Write == fsnotify.Write {
            newConfig, err := LoadAndValidate(path)
            if err != nil {
                log.Error("config reload failed", "error", err)
                continue  // Keep old config
            }

            cm.mu.Lock()
            cm.config = newConfig
            cm.mu.Unlock()

            cm.reloadCh <- struct{}{}  // Signal route manager to restart listeners
            log.Info("config reloaded successfully")
        }
    }
}
```

**Reload Behavior:**
- Config changes trigger listener restart (spawn new goroutines on new ports)
- Old listeners drain existing connections before shutdown (graceful)
- Invalid config rejected; SMAUG continues with previous valid config
- Metrics: `smaug_config_reload_total{status="success|failure"}`

**Alternatives Considered:**
- **SIGHUP signal:** More traditional Unix pattern, but requires signal handling
- **Polling:** Simpler but wastes CPU cycles
- **K8s ConfigMap watch:** Ties SMAUG to K8s API, reduces portability

**Trade-off:** File watcher is modern, container-friendly, and provides better developer experience (edit local file → instant reload)

### 4. Graceful Shutdown: Context Cancellation + Server.Shutdown()

**Decision:** Use `Server.Shutdown()` with 30s timeout for graceful termination

**Implementation:**
```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Trap signals
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

    // Start listeners
    servers := startAllListeners(config.Routes)

    // Wait for shutdown signal
    <-sigCh
    log.Info("shutdown initiated")

    // Graceful shutdown with timeout
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    for _, srv := range servers {
        srv.Shutdown(shutdownCtx)  // Waits for in-flight requests
    }

    log.Info("shutdown complete")
}
```

**Shutdown Sequence:**
1. Receive SIGTERM (from K8s `kubectl delete pod`)
2. Stop accepting new requests
3. Wait for in-flight requests to complete (max 30s)
4. Close all listeners
5. Exit cleanly

**K8s Integration:**
- Set `terminationGracePeriodSeconds: 60` (> SMAUG shutdown timeout)
- K8s waits 60s before SIGKILL
- SMAUG completes shutdown in 30s, giving 30s buffer

### 5. Metrics: Prometheus Client (Standard Exporter)

**Decision:** Export Prometheus metrics on `:2112/metrics`

**Metrics Specification:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `smaug_requests_total` | Counter | route, status | Total HTTP requests by route and status code |
| `smaug_request_duration_seconds` | Histogram | route | Request duration in seconds |
| `smaug_wake_attempts_total` | Counter | server, status | WoL attempts (status: success, timeout, error) |
| `smaug_server_awake` | Gauge | server | Server status (1=awake, 0=asleep/unknown) |
| `smaug_health_check_failures_total` | Counter | upstream | Health check failures per upstream |
| `smaug_sleep_triggered_total` | Counter | route | Auto-sleep triggers per route |
| `smaug_config_reload_total` | Counter | status | Config reload attempts (success/failure) |
| `smaug_gwaihir_api_duration_seconds` | Histogram | operation | Gwaihir API call duration |

**Implementation:**
```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func serveMetrics() {
    http.Handle("/metrics", promhttp.Handler())
    log.Fatal(http.ListenAndServe(":2112", nil))
}
```

**K8s Integration:**
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: smaug
  namespace: ai
spec:
  selector:
    matchLabels:
      app: smaug
  endpoints:
    - port: metrics
      interval: 30s
```

**Rationale:**
- Industry standard for K8s monitoring
- Native Grafana integration
- Low overhead (pull-based, not push)

### 6. Sleep-on-LAN: Idle Tracker + HTTP POST

**Decision:** Background goroutine monitors idle time, triggers REST API call

**Architecture:**
```go
type IdleTracker struct {
    mu          sync.RWMutex
    lastRequest map[string]time.Time  // route -> timestamp
}

func (it *IdleTracker) StartMonitor(cfg *Config) {
    ticker := time.NewTicker(1 * time.Minute)
    for range ticker.C {
        for routeName, route := range cfg.Routes {
            if !route.SleepOnLan.Enabled {
                continue
            }

            idleTime := time.Since(it.lastRequest[routeName])
            if idleTime > route.SleepOnLan.IdleTimeout {
                go triggerSleep(route.SleepOnLan.Endpoint)
                it.lastRequest[routeName] = time.Now()  // Reset to avoid repeat
            }
        }
    }
}
```

**Security Considerations:**
- **SSRF Protection:** Validate endpoint against `allowedHosts` pattern
- **Scheme Validation:** Only allow `http://` and `https://` (reject `file://`, `ftp://`, etc.)
- **Rate Limiting:** Avoid repeated sleep triggers (cooldown period = idle timeout)
- **Audit Logging:** Log all sleep triggers with endpoint and idle duration

**Endpoint Contract:**
- SMAUG sends `POST {endpoint}` (empty body)
- Endpoint should be idempotent (safe to call multiple times)
- Expected response: 200-299 status code
- Typical implementation: Systemd service on backend server (`systemctl suspend`)

**Alternatives Considered:**
- **SSH command:** Requires key management, more complex
- **IPMI/BMC API:** Not all homelab servers have BMC
- **Custom agent:** Lightweight REST API is simpler

### 7. Configuration Validation: Schema + Runtime Checks

**Decision:** Validate config on load with fail-fast behavior

**Validation Rules:**
```go
func (c *Config) Validate() error {
    // Validate server configs
    for name, server := range c.Servers {

        // Timeout bounds (10s - 300s)
        if server.WakeTimeout < 10 || server.WakeTimeout > 300 {
            return fmt.Errorf("server %s: timeout must be 10-300s", name)
        }

        // Sleep endpoint validation
        if server.SleepOnLan.Enabled {
            u, err := url.Parse(server.SleepOnLan.Endpoint)
            if err != nil {
                return fmt.Errorf("server %s: invalid sleep endpoint", name)
            }
            if u.Scheme != "http" && u.Scheme != "https" {
                return fmt.Errorf("server %s: sleep endpoint must use http/https", name)
            }
        }
    }

    // Validate route configs
    for _, route := range c.Routes {
        // Upstream URL format
        if _, err := url.Parse(route.Upstream); err != nil {
            return fmt.Errorf("route %s: invalid upstream URL", route.Name)
        }

        // Server reference exists
        if _, exists := c.Servers[route.Server]; !exists {
            return fmt.Errorf("route %s: unknown server %s", route.Name, route.Server)
        }

        // Port uniqueness (no conflicts)
        // Validate health check path starts with /
    }

    return nil
}
```

**Behavior:**
- **Startup:** Fail fast if config invalid (log error, exit 1)
- **Reload:** Reject invalid config, keep previous valid config, log warning
- **Metrics:** Increment `smaug_config_reload_total{status="failure"}`

---

## Core Components

| Component | Responsibility | Concurrency Model |
|-----------|----------------|-------------------|
| **Config Manager** | Load, validate, watch config file | Single goroutine for file watcher |
| **Route Manager** | Spawn HTTP listener per route | One goroutine per listening port |
| **Proxy Handler** | Handle incoming requests, coordinate wake/proxy | Goroutine per request (stdlib) |
| **Health Checker** | Poll upstream health endpoints | Shared rate-limited client |
| **Gwaihir Client** | Send WoL commands via REST API | Synchronous calls with timeout |
| **Idle Tracker** | Monitor last request time, trigger sleep | Single background goroutine |
| **Metrics Exporter** | Serve Prometheus metrics | Single goroutine on :2112 |

---

## Error Handling

| Scenario | HTTP Status | SMAUG Behavior | Client Experience |
|----------|-------------|----------------|-------------------|
| Backend sleeping, wake succeeds | 200 | Wait for wake (30-90s), proxy request | Slow response (first request) |
| Backend sleeping, wake timeout | 503 Service Unavailable | Log error, return 503 | Retry recommended |
| Backend awake, unreachable | 502 Bad Gateway | Proxy fails, return 502 | Service down |
| Config invalid on reload | N/A | Keep old config, log warning | No impact (old config active) |
| Gwaihir API unreachable | 503 Service Unavailable | Retry 3x with backoff, then 503 | Retry recommended |
| Health check fails | N/A | Trigger wake sequence | Transparent |
| Sleep API unreachable | N/A | Log error, continue | No impact on clients |

---

## Deployment Architecture

### Kubernetes Manifests

**Namespace:** `ai`

**Components:**
1. **Gwaihir Deployment** (1 replica, `hostNetwork: true`, privileged)
2. **Gwaihir Service** (ClusterIP, port 80 → 8080)
3. **SMAUG Deployment** (1 replica, unprivileged)
4. **SMAUG Services** (one per route: `ollama`, `marker`, `whisper`)
5. **SMAUG Metrics Service** (for Prometheus scraping)
6. **ConfigMap** (`smaug-config`)
7. **Secret** (`gwaihir-auth` - API key)
8. **NetworkPolicies** (restrict Gwaihir access, metrics access)
9. **ServiceMonitor** (Prometheus metrics collection)

**SMAUG Deployment:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: smaug
  namespace: ai
spec:
  replicas: 1
  selector:
    matchLabels:
      app: smaug
  template:
    metadata:
      labels:
        app: smaug
    spec:
      serviceAccountName: smaug
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        fsGroup: 65532
      containers:
        - name: smaug
          image: ghcr.io/josimar-silva/smaug:v1.0.0
          imagePullPolicy: IfNotPresent
          env:
            - name: SMAUG_CONFIG
              value: /etc/smaug/config.yaml
            - name: GWAIHIR_API_KEY
              valueFrom:
                secretKeyRef:
                  name: gwaihir-auth
                  key: api-key
            - name: SMAUG_LOG_LEVEL
              value: info
            - name: SMAUG_LOG_FORMAT
              value: json
          ports:
            - name: ollama
              containerPort: 11434
            - name: marker
              containerPort: 8080
            - name: whisper
              containerPort: 9000
            - name: metrics
              containerPort: 2112
          volumeMounts:
            - name: config
              mountPath: /etc/smaug
              readOnly: true
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
          livenessProbe:
            httpGet:
              path: /metrics
              port: 2112
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /metrics
              port: 2112
            initialDelaySeconds: 5
            periodSeconds: 10
      volumes:
        - name: config
          configMap:
            name: smaug-config
      terminationGracePeriodSeconds: 60
```

**NetworkPolicy (Gwaihir Protection):**
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gwaihir-ingress
  namespace: ai
spec:
  podSelector:
    matchLabels:
      app: gwaihir
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: smaug
      ports:
        - protocol: TCP
          port: 8080
```

**NetworkPolicy (Metrics Protection):**
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: smaug-metrics
  namespace: ai
spec:
  podSelector:
    matchLabels:
      app: smaug
  policyTypes:
    - Ingress
  ingress:
    # Allow application traffic from ai namespace
    - from:
        - namespaceSelector:
            matchLabels:
              name: ai
      ports:
        - protocol: TCP
          port: 11434
        - protocol: TCP
          port: 8080
        - protocol: TCP
          port: 9000

    # Allow metrics only from monitoring namespace
    - from:
        - namespaceSelector:
            matchLabels:
              name: monitoring
          podSelector:
            matchLabels:
              app: prometheus
      ports:
        - protocol: TCP
          port: 2112
```

---

## Security Considerations

### Threat Model

| Threat | Mitigation |
|--------|-----------|
| **Gwaihir API abuse** | API key authentication + NetworkPolicy + rate limiting |
| **Config tampering** | RBAC (read-only ConfigMap access) + validation + hash verification |
| **Wake flooding DoS** | Rate limiting (1/10s per machine) + debounce (5s) |
| **Sleep API SSRF** | Endpoint validation (allowlist + scheme check) |
| **Metrics info disclosure** | NetworkPolicy (Prometheus only) + sanitized labels |
| **Supply chain attack** | Image signing (cosign) + dependency scanning (govulncheck) |

### Security Controls

1. **Authentication:**
   - Gwaihir API key stored in K8s Secret (not ConfigMap)
   - Env var injection into SMAUG container

2. **Network Isolation:**
   - Gwaihir: Ingress only from SMAUG pods
   - SMAUG: Metrics ingress only from Prometheus

3. **Privilege Separation:**
   - SMAUG: `runAsNonRoot: true`, no capabilities, no hostNetwork
   - Gwaihir: `hostNetwork: true` (necessary for WoL broadcast)

4. **Input Validation:**
   - Config schema validation (fail fast)
   - URL parsing and scheme validation
   - Sleep endpoint allowlist

5. **Audit Logging:**
   - All WoL requests logged (server, source IP, timestamp)
   - All sleep triggers logged (route, endpoint, idle duration)
   - Config reloads logged (success/failure)
   - Structured JSON logs for SIEM integration

6. **Rate Limiting:**
   - Gwaihir: 1 WoL request / 10s per server
   - SMAUG: 10 health checks / second per upstream
   - Idle tracker: Cooldown = idle timeout (avoid sleep loops)

---

## Observability

### Structured Logging

**Format:** JSON (for log aggregation)

**Log Levels:**
- **DEBUG:** Health check attempts, config reload triggers
- **INFO:** Request proxied, WoL sent, sleep triggered, config reloaded
- **WARN:** Wake timeout, health check failures, rate limit exceeded
- **ERROR:** Gwaihir API error, config validation failed, proxy error

**Example Log:**
```json
{
  "level": "info",
  "timestamp": "2026-02-11T10:30:45Z",
  "msg": "wol_sent",
  "server": "saruman",
  "duration_ms": 120
}
```

### Metrics (Prometheus)

**Dashboards (Grafana):**
1. **Request Overview:** Request rate, duration (p50/p95/p99), error rate by route
2. **Wake Performance:** Wake attempts, success rate, duration by server
3. **Power Management:** Server awake status, sleep triggers, idle time
4. **Health:** Health check failures, Gwaihir API errors, config reload status

**Alerts:**
- `SmaugHighWakeFailureRate`: Wake success rate < 90% over 5min
- `SmaugBackendDown`: Server unhealthy for > 5min (not due to sleep)
- `SmaugGwaihirUnreachable`: Gwaihir API errors > 5 in 1min
- `SmaugConfigReloadFailed`: Config reload failures > 0

---

## Performance Characteristics

### Resource Usage (Estimated)

| Metric | Idle | Active (10 req/s) |
|--------|------|-------------------|
| CPU | 10m | 100m |
| Memory | 50Mi | 128Mi |
| Network | <1 Mbps | Depends on backend response size |

### Latency

| Scenario | Latency |
|----------|---------|
| Backend awake, healthy | ~5ms (proxy overhead) |
| Backend sleeping, wake succeeds | 30-90s (WoL + boot time) |
| Backend unreachable | 2s (health check timeout) |

### Scalability

- **Concurrent Requests:** Limited by backend capacity (SMAUG adds minimal overhead)
- **Routes:** No practical limit (each route is independent listener)
- **Servers:** Tested with 10 servers, no issues (Gwaihir is stateless)

---

## Consequences

### Positive

1. **Power Savings:** 90%+ reduction in idle power consumption (400W → 40W overhead)
2. **Transparent to Clients:** Standard reverse proxy interface, no client changes
3. **Privilege Separation:** SMAUG is unprivileged, security boundary at Gwaihir
4. **Learning Value:** Deep Go education (HTTP, concurrency, config management, observability)
5. **K8s Native:** Metrics, health checks, graceful shutdown follow K8s patterns
6. **Operational Simplicity:** YAML config, hot-reload, comprehensive metrics
7. **Auto-Sleep:** Reduces power consumption further with idle shutdown

### Negative

1. **First Request Latency:** 30-90s delay when waking server (UX impact)
2. **Single Point of Failure:** SMAUG down = all backends unreachable (mitigated by K8s replica)
3. **L2 Dependency:** WoL requires broadcast domain (same subnet/VLAN)
4. **Complexity:** Two-service architecture adds deployment complexity vs single-service

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|-----------|
| **WoL unreliable** | Medium | High | Add retry logic, alert on failures, fallback to manual wake |
| **Network segmentation breaks WoL** | Low | High | Document L2 requirement, test in staging |
| **Config error breaks routing** | Medium | Medium | Validation on load, keep old config on error |
| **Gwaihir unreachable** | Low | High | Retry with backoff, alert on sustained failures |
| **Backend fails to sleep** | Low | Low | Log error, continue operation (no client impact) |

---

## Future Enhancements

### Phase 2 (Post-MVP)
1. **Request Buffering:** Queue incoming requests during wake-up instead of blocking (better UX)
2. **Circuit Breaker:** Avoid repeated wake attempts if server persistently fails (prevent resource waste)
3. **WebSocket Support:** Extend proxy to handle WS connections (requires connection hijacking)

### Phase 3 (Advanced)
1. **Pre-warming:** Scheduled wake-up before predicted usage (ML-based analysis of request patterns)
2. **Leader Election:** Run multiple SMAUG replicas with leader election (only leader sends WoL, others proxy)
3. **gRPC Proxying:** Support gRPC services (requires `grpc-go` reverse proxy)
4. **Dynamic Configuration:** Watch K8s Service resources, auto-generate routes (operator pattern)

### Phase 4 (Optimization)
1. **Smart Wake Prediction:** Wake server 30s before request based on access patterns
2. **Multi-tier Sleep:** Different sleep states (suspend-to-RAM vs full shutdown)
3. **Energy Dashboard:** Track power savings, carbon footprint, cost reduction
4. **Health Check Caching:** Cache health status to reduce backend load

---

## Testing Strategy

### Unit Tests
- Config validation (valid/invalid YAML)
- Health checker (mock HTTP responses)
- Gwaihir client (mock API)
- Idle tracker logic

### Integration Tests
- End-to-end: Client → SMAUG → Gwaihir → Backend
- Config hot-reload (file modification)
- Graceful shutdown (SIGTERM handling)
- Metrics emission (validate Prometheus output)

### Security Tests
- Gwaihir API auth (invalid key rejected)
- NetworkPolicy enforcement (unauthorized access blocked)
- Config injection (malicious YAML rejected)
- Sleep endpoint SSRF (invalid schemes rejected)

### Performance Tests
- Proxy overhead measurement (baseline vs SMAUG)
- Concurrent request handling (10/100/1000 req/s)
- Wake performance (latency distribution)

---

## Migration Path

### Phase 1: Gwaihir Deployment
1. Deploy Gwaihir service in K8s
2. Verify WoL functionality manually (curl `/wol`)
3. Apply NetworkPolicy

### Phase 2: SMAUG MVP (No Sleep)
1. Implement core proxy + wake functionality
2. Deploy in K8s alongside existing direct routes
3. Test with one route (Ollama), validate latency
4. Gradually migrate other routes

### Phase 3: Add Sleep-on-LAN
1. Implement idle tracker
2. Add sleep API to backend servers
3. Enable sleep for one server, monitor
4. Roll out to all servers

### Phase 4: Observability
1. Add metrics exporter
2. Create Grafana dashboards
3. Configure Prometheus alerts
4. Integrate logs with Loki

---

## References

### Technical Documentation
- [Go net/http](https://pkg.go.dev/net/http)
- [httputil.ReverseProxy](https://pkg.go.dev/net/http/httputil#ReverseProxy)
- [Prometheus Go Client](https://github.com/prometheus/client_golang)
- [fsnotify](https://github.com/fsnotify/fsnotify)

### Standards
- [Wake-on-LAN Specification](https://en.wikipedia.org/wiki/Wake-on-LAN)
- [HTTP Reverse Proxy Best Practices](https://www.rfc-editor.org/rfc/rfc7230)

### Security
- [CIS Kubernetes Benchmark](https://www.cisecurity.org/benchmark/kubernetes)
- [OWASP API Security Top 10](https://owasp.org/www-project-api-security/)
- [NSA/CISA Kubernetes Hardening Guide](https://media.defense.gov/2022/Aug/29/2003066362/-1/-1/0/CTR_KUBERNETES_HARDENING_GUIDANCE_1.2_20220829.PDF)

### Related Projects
- [Gwaihir](https://github.com/josimar-silva/gwaihir) - WoL messenger service

---

## Appendix: NGINX Comparison

**NGINX Strengths (Baseline Inspiration):**
- Battle-tested reverse proxy (proven performance, security)
- Rich feature set (load balancing, caching, SSL termination)
- Declarative config (nginx.conf)
- Extensive ecosystem (modules, tooling)

**SMAUG Differentiation (Smart Features):**
- **Auto-wake:** NGINX cannot wake sleeping servers (not power-aware)
- **Auto-sleep:** NGINX has no idle detection / power management
- **K8s Native:** NGINX requires external controller (ingress-nginx), SMAUG is native
- **Observability:** SMAUG metrics are power-focused (wake time, sleep triggers, server status)
- **Simplicity:** SMAUG is single-purpose (power-aware proxy), not general-purpose

**When to Use NGINX Instead:**
- Production internet-facing services (battle-tested, DoS protection)
- Complex routing (path-based, regex, rewrite rules)
- SSL termination, caching, compression
- High throughput requirements (10k+ req/s)

**When to Use SMAUG:**
- Homelab GPU servers with idle power management
- Learning Go / K8s patterns
- Power-aware routing (wake/sleep automation)
- Simple port-based proxying

**Hybrid Approach:**
- NGINX as internet-facing reverse proxy (SSL termination, rate limiting)
- SMAUG as internal power-aware proxy (wake/sleep GPU servers)
- NGINX → SMAUG → Backend

---

## Appendix: Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2025-11-23 | Use Go over Rust | Learning value, K8s ecosystem fit |
| 2025-11-23 | Two-service architecture (SMAUG + Gwaihir) | Privilege separation, security best practice |
| 2026-02-11 | Go stdlib over frameworks | Port-based routing, learning value, zero dependencies |
| 2026-02-11 | Hot-reload via fsnotify | Modern, container-friendly DX |
| 2026-02-11 | Prometheus metrics on :2112 | K8s standard, Grafana integration |
| 2026-02-11 | Sleep-on-LAN via REST API | Flexible, server-agnostic, auditability |
| 2026-02-11 | Graceful shutdown (30s timeout) | K8s compatibility, data integrity |

---

**End of Document**
