# AGENTS.md

This document provides guidance for AI coding agents working with the Smaug codebase.

## Project Overview

**Smaug** is a production-ready config-driven reverse proxy written in Go with automatic Wake-on-LAN capabilities for homelab services. It intelligently routes requests and wakes sleeping servers on demand, enabling efficient resource management in self-hosted environments.

**Key Characteristics:**
- Language: Go 1.26+
- Architecture: Simplified layered architecture (pragmatic, not over-engineered)
- Logging: Structured JSON logging with `log/slog`
- Metrics: Prometheus metrics
- Test Coverage: 90%+ across all packages

## Architecture

Smaug uses a **simplified layered architecture** that's pragmatic and maintainable without unnecessary abstraction:

```
internal/
├── proxy/            # Proxy, route manager, idle tracker, wake/sleep coordinators
├── store/            # In-memory health store
├── health/           # Server health checker and health manager
├── config/           # Configuration loading, parsing, and hot-reload
├── middleware/       # HTTP middleware (logging, recovery, metrics)
├── infrastructure/   # Logging (logger) and metrics (Prometheus registry)
├── management/       # Management HTTP servers (health endpoints, metrics endpoint)
└── client/           # External service clients (Gwaihir WoL, sleep endpoint)

cmd/smaug/            # Application entrypoint
tests/                # Integration tests
config/               # Example configuration files
```

## Code Conventions

### SOLID Principles (Pragmatic Application)

Apply SOLID principles where they provide value, not everywhere:

- **Single Responsibility Principle (SRP):** Each type/function should have one reason to change
- **Open/Closed Principle (OCP):** Open for extension, closed for modification (through interfaces when needed)
- **Liskov Substitution Principle (LSP):** Subtypes are substitutable for base types (matters for interfaces)
- **Interface Segregation Principle (ISP):** Keep interfaces small and focused
- **Dependency Inversion Principle (DIP):** Depend on abstractions for swappable implementations (stores, external services)

Use interfaces strategically:
- Database/storage backends (swap YAML for database)
- External service integrations (WoL senders, health checkers)
- Avoid interfaces for one-off implementations

### Functional Programming Principles

Apply functional principles where appropriate:

- **Pure Functions:** Functions without side effects; same input = same output
- **Composition:** Build behavior from reusable functions
- **Avoid Global State:** Use dependency injection
- **Immutability:** Prefer immutable data where practical

### File Organization

- Each layer has its own package
- Test files use `_test.go` suffix and live alongside source files
- Mocks are generated in the same package they mock
- Integration tests live in `tests/` directory

### Naming Conventions

- **Interfaces:** Descriptive names (e.g., `HealthStore`, `WoLSender`, `ActivityRecorder`)
- **Implementations:** Include implementation detail (e.g., `InMemoryHealthStore`)
- **Methods:** Clear action verbs (e.g., `SendWoL`, `Get`, `RecordActivity`)
- **Variables:** Descriptive, avoid single letters except in very short scopes
- **Errors:** Use domain-specific errors (see Error Handling section below)

### Error Handling

**Domain Errors Pattern:**

Define domain errors close to where they're used (in the same package). Use `errors.New()` for base errors and wrap them with context when returning.

```go
// Define domain errors at the package level
// Example: internal/proxy/wake_coordinator.go
var (
    // ErrMissingServerID is returned when WakeCoordinatorConfig.ServerID is empty.
    ErrMissingServerID = errors.New("server ID must not be empty")

    // ErrWakeTimeout is returned when the server does not become healthy within WakeTimeout.
    ErrWakeTimeout = errors.New("server did not become healthy within wake timeout")
)

// Wrap domain errors with context using %w (preserves error chain for errors.Is())
return fmt.Errorf("%w: %w", ErrWakeTimeout, ctx.Err())

// Check for specific domain errors using errors.Is()
if errors.Is(err, ErrWakeTimeout) {
    // Handle wake timeout specifically
}
```

**Key Principles:**
- Define domain errors in the package where they're used (not in a separate `errors.go`)
- Use descriptive error messages that explain the failure condition
- Always wrap errors with `%w` to preserve the error chain
- Use `errors.Is()` to check for specific error types (never string comparison)
- Document which errors a function can return in its godoc comments

### Logging

Always use structured logging with `log/slog`:

```go
// Good: Structured logging with context
logger.Info("Sending WoL packet",
    slog.String("server_id", serverID),
    slog.String("machine_id", machineID),
)

// Bad: Unstructured logging
logger.Info("Sending WoL packet to " + serverID)
```

### Metrics

Record metrics for important operations:

```go
// Counter for operations (with labels)
metrics.Power.RecordWakeAttempt(serverID, success)

// Gauge for state
metrics.Power.SetServerAwake(serverID, awake)

// Record request metrics
metrics.Request.RecordRequest(method, statusCode, durationSeconds)
```

## Testing Standards

### Test-Driven Development (TDD)

Always follow TDD principles:

1. **Red Phase:** Write failing test first
   - Test defines the expected behavior
   - Test fails initially (no implementation yet)

2. **Green Phase:** Write minimal implementation
   - Just enough code to make the test pass
   - Don't over-engineer at this stage

3. **Refactor Phase:** Improve code quality
   - Refactor production code while keeping tests green
   - Refactor tests for clarity and maintainability
   - Remove duplication, improve names

Never write implementation without a corresponding test. Tests drive design and ensure correctness.

### Test Coverage Requirements

- **Minimum:** 90% coverage across all packages
- **Service layer:** 95%+ (business logic is critical)
- **Handler layer:** 90%+ (all endpoints and error paths)
- **Store layer:** 85%+ (focus on CRUD correctness)

### Test Code Quality

Treat test code with the same rigor as production code:

- **SOLID Principles Apply to Tests**
  - **Single Responsibility:** Each test has one reason to fail
  - **Open/Closed:** Tests are open for extension, closed for modification
  - **Liskov Substitution:** Mock implementations are substitutable for real ones
  - **Interface Segregation:** Use small, focused interfaces in tests
  - **Dependency Inversion:** Tests depend on abstractions (interfaces), not concrete types

- **Functional Principles**
  - Keep helper functions pure (no side effects)
  - Use immutable test data where possible
  - Avoid shared mutable state between tests
  - Each test should be independent and runnable in any order

- **Maintainability**
  - Give tests clear, descriptive names (what is being tested, input, expected result)
  - Use consistent naming patterns: `Test<Function>_<Scenario>_<ExpectedBehavior>`
  - Extract common setup into well-named helper functions or setUp methods
  - Keep tests concise and focused (typically 5-20 lines)
  - Use constants/factories for test data instead of magic values

### Test Structure

Follow the **Given-When-Then** pattern:

```go
func TestWakeCoordinator_ServeHTTP_WakesServerWhenUnhealthy(t *testing.T) {
    // Given: Set up test dependencies and data
    ctx := context.Background()
    log := logger.New(logger.LevelDebug, logger.JSON, nil)
    store := store.NewInMemoryHealthStore()
    mockSender := &mockWoLSender{}
    downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })
    coordinator, err := proxy.NewWakeCoordinator(
        proxy.WakeCoordinatorConfig{
            ServerID:    "saruman",
            MachineID:   "saruman",
            WakeTimeout: 5 * time.Second,
        },
        downstream,
        mockSender,
        store,
        log,
        nil,
    )
    require.NoError(t, err)

    // When: Execute the function being tested
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    w := httptest.NewRecorder()
    coordinator.ServeHTTP(w, req)

    // Then: Verify results and side effects
    assert.Equal(t, 1, mockSender.sendCallCount)
}
```

### Mocking

- Use interface-based mocking (never mock concrete types)
- Create mock implementations in test files alongside tests
- Mocks should be simple and focused on the interface contract
- Avoid over-mocking; only mock external dependencies
- Verify both return values AND side effects (call counts, arguments)

**Example Mock Implementation:**

```go
type mockWoLSender struct {
    sendCallCount int
    sendErr       error
}

func (m *mockWoLSender) SendWoL(ctx context.Context, machineID string) error {
    m.sendCallCount++
    return m.sendErr
}
```

### Table-Driven Tests

Use table-driven tests for multiple scenarios with the same logic:

```go
func TestConfig_Validate(t *testing.T) {
    tests := []struct {
        name    string
        cfg     config.Config
        wantErr bool
        errType error
    }{
        {
            name:    "valid config with all fields",
            cfg:     buildValidConfig(),
            wantErr: false,
        },
        {
            name:    "missing route upstream",
            cfg:     buildConfigWithEmptyUpstream(),
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.cfg.Validate()
            if tt.wantErr {
                assert.Error(t, err)
                if tt.errType != nil {
                    assert.True(t, errors.Is(err, tt.errType))
                }
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Test Helpers and Builders

Extract common test setup into reusable helpers:

```go
// Helper functions for test data
func buildValidConfig() config.Config {
    return config.Config{
        Routes: []config.Route{
            {Name: "ollama", Listen: 11434, Upstream: "http://localhost:11434"},
        },
    }
}

// In tests:
func TestSomething(t *testing.T) {
    cfg := buildValidConfig()
    // ...
}
```

## Commit Practices

### Atomic Commits

Make commits that are:

- **Logically Cohesive:** Each commit represents a single logical change
- **Independently Testable:** Each commit should pass all tests on its own
- **Reversible:** You should be able to `git revert` a commit without breaking the system
- **Context-Preserving:** The commit includes exactly what's needed, no unnecessary changes

**Bad Example (Multiple concerns in one commit):**
```
commit abc123
  - Add WoL packet sending
  - Fix auth middleware bug
  - Refactor logger
  - Update documentation
```

**Good Example (Atomic commits):**
```
commit abc123
  - Add WoL packet sending functionality

commit def456
  - Fix auth middleware JWT validation bug

commit ghi789
  - Refactor logger for better testability

commit jkl012
  - Update README with new endpoint documentation
```

**Benefits:**
- Easier to review and understand changes
- Simpler to revert problematic changes without losing good work
- Better for bisecting bugs with `git bisect`
- Clearer project history

### Conventional Commits

Use Conventional Commits format for all commit messages:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat:` New feature
- `fix:` Bug fix
- `refactor:` Code refactoring (no behavior change)
- `test:` Test-related changes
- `docs:` Documentation changes
- `chore:` Build scripts, dependencies, tooling
- `perf:` Performance improvements
- `ci:` CI/CD configuration changes

**Scopes:**
- `proxy` - Proxy, route manager, wake/sleep coordination
- `health` - Health checker and health manager
- `config` - Configuration loading, parsing, validation
- `middleware` - HTTP middleware
- `infrastructure` - Logging, metrics
- `management` - Management HTTP servers
- `client` - External service clients (Gwaihir, sleep)
- `store` - In-memory stores
- `ci` - CI/CD workflows

**Examples:**

```
feat(proxy): add wake coordinator debounce to prevent wake flooding

Adds configurable debounce interval to WakeCoordinator to prevent
sending multiple WoL commands in rapid succession when many requests
arrive while a server is sleeping.

Closes #42
```

```
fix(health): handle nil auth token in server health checker

Prevent panic when ServerHealthCheck.AuthToken is unset by checking
Value() before setting the Authorization header.

Fixes #35
```

```
test(config): add table-driven tests for YAML substitution

Use table-driven tests to cover all env var substitution scenarios:
- Present variable
- Missing variable (passthrough)
- Nested braces

Achieves 100% coverage for substitute.go
```

### Pre-Commit Workflow

**Before committing, always:**

1. Run all linters and formatters:
   ```bash
   just pre-commit
   ```

2. Verify tests pass with coverage:
   ```bash
   just test
   ```

3. Check no race conditions:
   ```bash
   go test -race ./...
   ```

4. Review your changes:
   ```bash
   git diff
   git diff --cached
   ```

5. Ensure your commit is atomic:
   - Does it represent one logical change?
   - Can it stand alone?
   - Will someone understand it 6 months from now?

**Never commit if:**
- Linters are failing
- Tests don't pass
- Code coverage decreased
- You have uncommitted changes that belong in the commit
- The commit mixes multiple unrelated changes

## Development Workflow

### Running Tests

```bash
# Run all tests with coverage
just test

# Run specific package tests
go test ./internal/proxy/...
go test ./internal/health/... -v

# Run with race detector
go test -race ./...
```

### Code Quality

```bash
# Format code
just format

# Run linters
just lint

# Run all pre-commit checks
just pre-commit
```

### Building

```bash
# Build binary
just build

# Build Docker image
just docker-build latest

# Run locally
export SMAUG_CONFIG=config/smaug.example.yaml
just run
```

## Common Tasks

### Adding a New Route Field

1. Update the `Route` struct in `internal/config/types.go`
2. Add validation logic in `internal/config/validation.go`
3. Update `config/smaug.example.yaml` schema and example
4. Add tests in `internal/config/`
5. Update route handling in `internal/proxy/route_manager.go`
6. Ensure 90%+ test coverage

### Adding a New Management Endpoint

1. Define the handler function in `internal/management/health/handlers.go`
2. Register the route in `internal/management/health/server.go`
3. Add response types to `internal/management/health/types.go`
4. Create tests in `internal/management/health/`
5. Document endpoint in README.md
6. Ensure 90%+ test coverage

### Adding a New Metric

1. Add the metric field to the appropriate metrics struct in `internal/infrastructure/metrics/`
2. Register the metric in the corresponding `New*Metrics` constructor
3. Add a public `Record*` method following the existing pattern
4. Call the record method at the appropriate location in the codebase
5. Update the metrics list in README.md
6. Add tests in `internal/infrastructure/metrics/`

### Adding a New Domain Error

1. Define the error variable in the package where it is used (e.g., `internal/proxy/wake_coordinator.go`)
2. Return it (wrapped with `%w`) from the relevant functions
3. Add tests for the error handling path
4. Document in code comments

## Important Files

### Entry Point
- `cmd/smaug/main.go` - Application bootstrap and dependency wiring
- `cmd/smaug/version.go` - Version constants (updated by release process)

### Configuration
- `config/smaug.example.yaml` - Example configuration file
- `internal/config/types.go` - Configuration struct definitions
- `internal/config/loader.go` - YAML loading and env var substitution
- `internal/config/manager.go` - Config manager with hot-reload support
- `internal/config/validation.go` - Configuration validation rules

### Proxy Core
- `internal/proxy/proxy.go` - Reverse proxy handler
- `internal/proxy/route_manager.go` - Per-port listener lifecycle management
- `internal/proxy/wake_coordinator.go` - Wake-on-LAN coordination middleware
- `internal/proxy/sleep_coordinator.go` - Sleep-on-LAN coordination
- `internal/proxy/idle_tracker.go` - Idle detection and sleep triggers
- `internal/proxy/reload.go` - Config hot-reload diff logic

### Health
- `internal/health/manager.go` - Coordinates health check workers for all servers
- `internal/health/health.go` - HTTP health checker implementation
- `internal/health/server_health_checker.go` - Per-server health check with store updates
- `internal/store/health_store.go` - In-memory health status store

### Middleware
- `internal/middleware/logging.go` - Structured request logging middleware
- `internal/middleware/recovery.go` - Panic recovery middleware
- `internal/middleware/metrics.go` - Request metrics middleware

### Infrastructure
- `internal/infrastructure/logger/logger.go` - Structured logger wrapper around `log/slog`
- `internal/infrastructure/metrics/metrics.go` - Prometheus registry and metric group wiring
- `internal/infrastructure/metrics/power.go` - WoL/sleep/health metrics
- `internal/infrastructure/metrics/request.go` - HTTP request metrics
- `internal/infrastructure/metrics/gwaihir.go` - Gwaihir API call metrics
- `internal/infrastructure/metrics/config.go` - Config reload metrics

### Management Servers
- `internal/management/health/server.go` - Management HTTP server (port 2111 by default)
- `internal/management/health/handlers.go` - `/health`, `/live`, `/ready`, `/version` handlers
- `internal/management/metrics/server.go` - Prometheus metrics HTTP server (port 2112 by default)

### External Clients
- `internal/client/gwaihir/client.go` - Gwaihir WoL API client with retry
- `internal/client/sleep/client.go` - Sleep endpoint client

## Configuration

Smaug uses a unified YAML configuration file (`services.yaml` by default) that covers global settings, server power management, and route definitions.

### Configuration File Format

```yaml
# Global settings
settings:
  logging:
    level: info    # debug, info, warn, error
    format: json   # json or text

  gwaihir:
    url: "http://gwaihir-service.ai.svc.cluster.local"
    apiKey: "${GWAIHIR_API_KEY}"  # Env var substitution
    timeout: 5s

  observability:
    healthCheck:
      enabled: true   # Exposes /health, /live, /ready, /version
      port: 2111
    metrics:
      enabled: true   # Exposes /metrics (Prometheus format)
      port: 2112

# Server definitions (keyed by server name)
servers:
  saruman:
    wakeOnLan:
      enabled: true
      machineId: "saruman"         # Gwaihir machine ID
      timeout: 60s
      debounce: 5s                 # Min interval between WoL attempts

    sleepOnLan:
      enabled: true
      endpoint: "http://saruman.from-gondor.com:8000/sleep"
      authToken: "${SLEEP_ON_LAN_TOKEN}"  # Env var substitution
      idleTimeout: 5m              # Sleep after 5 min idle

    healthCheck:
      endpoint: "http://saruman.from-gondor.com:8000/status"
      authToken: "${HEALTH_CHECK_TOKEN}"  # Optional: base64-encoded user:password for Basic Auth
      interval: 2s
      timeout: 2s

# Route definitions (port-per-service)
routes:
  - name: ollama
    listen: 11434
    upstream: "http://saruman.from-gondor.com:11434"
    server: saruman

  - name: marker
    listen: 8080
    upstream: "http://saruman.from-gondor.com:8080"
    server: saruman
```

### Environment Variables

```bash
SMAUG_CONFIG=/etc/smaug/services.yaml   # Config file path (default: /etc/smaug/services.yaml)
SMAUG_LOG_LEVEL=info                    # Log level (debug|info|warn|error); default: info
SMAUG_LOG_FORMAT=json                   # Log format (json|text); default: json
```

Any YAML value in the format `${ENV_VAR_NAME}` is substituted with the corresponding environment variable at load time. This is the supported mechanism for injecting secrets such as `GWAIHIR_API_KEY` into the configuration.

## Security Considerations

### Secret Handling

- All sensitive config values (`apiKey`, `authToken`) use the `SecretString` type
- `SecretString` redacts its value in logs, JSON/text marshalling, and `fmt.Stringer` output
- Never access the raw value with `.Value()` except when making outbound API calls
- Never log API keys, auth tokens, or other secrets

### Allowlist-Based Access

- Only servers defined in the configuration can receive WoL commands
- No dynamic server registration at runtime
- Validate all inputs at the config load and service layers
- Use config validation methods before applying any configuration

### Network Security

- Runs with `hostNetwork: true` in Kubernetes (required for broadcast)
- Should be protected by NetworkPolicy
- Management ports (2111, 2112) should only be accessible from trusted services

## Observability

### Structured Logging

All log entries include structured key-value pairs. Common keys:

- `operation` - The operation being performed (e.g., `"init_idle_tracker"`)
- `server_id` - Target server identifier
- `machine_id` - Gwaihir machine identifier
- `route` - Route name
- `error` - Error value (when applicable)

### Prometheus Metrics

Key metrics registered in `internal/infrastructure/metrics/`:

- `smaug_wake_attempts_total{server, success}` - WoL wake attempts
- `smaug_server_awake{server}` - Server state gauge (1=awake, 0=sleeping)
- `smaug_health_check_failures_total{server, reason}` - Health check failures
- `smaug_sleep_triggered_total{server}` - Servers put to sleep
- `smaug_request_duration_seconds` - HTTP request latency histogram
- `smaug_requests_total` - Total HTTP requests
- `smaug_requests_by_status_total{status}` - Requests by HTTP status code
- `smaug_requests_by_method_total{method}` - Requests by HTTP method
- `smaug_config_reload_total{success}` - Config reload attempts
- `smaug_gwaihir_api_calls_total{operation, success}` - Gwaihir API calls
- `smaug_gwaihir_api_duration_seconds{operation, success}` - Gwaihir API latency

## Troubleshooting

### Tests Failing

1. Check test coverage: `just test`
2. Run specific failing test: `go test ./path/... -run TestName -v`
3. Check for race conditions: `go test -race ./...`
4. Verify mocks are properly configured

### Linter Errors

1. Run `just format` to auto-fix formatting
2. Run `just lint` to see specific issues
3. Check `golangci-lint` configuration in `.golangci.yml`
4. Fix issues one by one, maintain code quality

### Build Issues

1. Verify Go version: `go version` (need 1.26+)
2. Clean build cache: `go clean -cache`
3. Update dependencies: `go mod tidy`
4. Check for missing dependencies: `go mod verify`

## Code Review Standards

### Pre-Review Requirements

**No code should be submitted for review without:**

1. **All Tests Passing**
   - `just test` - All tests pass with 90%+ coverage
   - `go test -race ./...` - No race conditions
   - Coverage must not decrease from main branch

2. **All Linters Passing**
   - `just format` - Code formatted
   - `just lint` - All linters pass
   - No warnings or issues

3. **Complete Pre-Commit Workflow**
   - Run `just pre-commit` to verify everything
   - Commits must be atomic (one logical change)
   - Commit messages must follow Conventional Commits
   - Code must respect SOLID and functional principles

4. **Manual Review Checklist**
   - Architecture: Does code respect layer boundaries?
   - SOLID: Are principles applied correctly?
   - Tests: Are tests production-quality with good names?
   - Documentation: Are public APIs documented?
   - Error Handling: All error paths handled properly?
   - Logging: Structured logs with appropriate context?
   - Metrics: Important operations recorded?

### Code Review Focus Areas

- **Clean Architecture Compliance:** Dependency direction, layer separation
- **SOLID Principles:** SRP, OCP, LSP, ISP, DIP application
- **Functional Purity:** Side effects minimized, immutability preferred
- **Test Quality:** Clear naming, good isolation, appropriate mocking
- **Error Handling:** Proper error wrapping, specific error types
- **Observability:** Structured logging, Prometheus metrics
- **Maintainability:** Clear names, small focused functions, DRY

## Best Practices

### DO:
- Keep dependencies flowing in one direction (proxy → health/config/store → infrastructure)
- Write tests BEFORE implementation (TDD)
- Use structured logging with context
- Record metrics for important operations
- Validate inputs at the config layer
- Use domain-specific errors
- Keep functions small and focused
- Document exported functions and types
- Use Atomic Commits with Conventional Commits messages
- Run `just pre-commit` before every commit
- Ensure tests are as clean and maintainable as production code
- Verify 90%+ test coverage and no race conditions
- Only create interfaces when you have 2+ implementations

### DON'T:
- Don't create unnecessary abstractions
- Don't skip writing tests (TDD means tests first)
- Don't use unstructured logging
- Don't hardcode configuration values
- Don't ignore errors
- Don't use global state
- Don't mix concerns across layers
- Don't commit without running `just pre-commit`
- Don't submit code for review that fails linters or tests
- Don't write tests that are hard to understand or maintain
- Don't use complex mocks when simple ones suffice
- Don't commit without atomic, conventional commit messages
- Don't create interfaces before you need them

## Release Process

Smaug uses SNAPSHOT-based versioning:

1. Development on SNAPSHOT versions (e.g., `0.2.0-SNAPSHOT`)
2. Run `just pre-release` to prepare release
3. Push to trigger CI/CD
4. CD workflow creates release and bumps to next SNAPSHOT

## Related Documentation

- [README.md](README.md) - User-facing documentation
- [CONTRIBUTING.md](CONTRIBUTING.md) - Contribution guidelines
- [ADR-001: Foundation Architecture](docs/adrs/ADR-001-foundation.md) - Architecture details

## Quick Reference

```bash
# Install dependencies
just ci

# Run all checks
just pre-commit

# Run tests
just test

# Run locally
export SMAUG_CONFIG=config/smaug.example.yaml
just run

# Build
just build

# Docker
just docker-build latest
```

## Getting Help

- Review existing tests for patterns
- Check README.md for API documentation
- Review CONTRIBUTING.md for code standards
- Examine similar existing code in the layer you're working in
