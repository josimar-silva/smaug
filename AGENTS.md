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
├── proxy/            # Proxy and routes
├── store/            # Storage
├── health/           # Server health checker
├── config/           # Configuration parsing
├── middleware/       # HTTP middleware
├── infrastructure/   # Logging, metrics, etc.
└── management/       # Server Management

cmd/smaug/            # Application entrypoint
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

- **Interfaces:** Descriptive names (e.g., `MachineRepository`, `WoLUseCase`)
- **Implementations:** Include implementation detail (e.g., `InMemoryMachineRepository`)
- **Methods:** Clear action verbs (e.g., `SendWoLPacket`, `GetMachine`, `ListMachines`)
- **Variables:** Descriptive, avoid single letters except in very short scopes
- **Errors:** Use domain-specific errors (see Error Handling section below)

### Error Handling

**Domain Errors Pattern:**

Define domain errors close to where they're used (in the same package). Use `errors.New()` for base errors and wrap them with context when returning.

```go
// Define domain errors at the package level
// Example: internal/service/health/health.go
var (
    // ErrHealthCheckFailed is returned when a health check fails due to an unhealthy status code (4xx, 5xx).
    ErrHealthCheckFailed = errors.New("health check failed: unhealthy status code")

    // ErrHealthCheckNetworkError is returned when a health check request fails due to network issues.
    ErrHealthCheckNetworkError = errors.New("health check failed: network error")
)

// Wrap domain errors with context using %w (preserves error chain for errors.Is())
return fmt.Errorf("%w: %s returned status %d", ErrHealthCheckFailed, url, statusCode)

// Check for specific domain errors using errors.Is()
if errors.Is(err, ErrHealthCheckNetworkError) {
    // Handle network error specifically
}

if errors.Is(err, ErrHealthCheckFailed) {
    // Handle unhealthy status code
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
    slog.String("machine_id", machineID),
    slog.String("mac", machine.MAC),
    slog.String("broadcast", machine.Broadcast),
)

// Bad: Unstructured logging
logger.Info("Sending WoL packet to " + machineID)
```

### Metrics

Record metrics for important operations:

```go
// Counter for operations
metrics.WoLPacketsSentTotal.Inc()
metrics.WoLPacketsFailedTotal.Inc()

// Histogram for durations
timer := prometheus.NewTimer(metrics.RequestDuration.WithLabelValues(method, path, status))
defer timer.ObserveDuration()
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
func TestWoLUseCase_SendWoLPacket_Success(t *testing.T) {
    // Given: Set up test dependencies and data
    ctx := context.Background()
    machineID := "gandalf"
    mockRepo := &MockMachineRepository{
        GetMachineFunc: func(ctx context.Context, id string) (*domain.Machine, error) {
            if id == machineID {
                return &domain.Machine{
                    ID:        machineID,
                    MAC:       "AA:BB:CC:DD:EE:FF",
                    Broadcast: "192.168.1.255",
                }, nil
            }
            return nil, domain.ErrMachineNotFound
        },
    }
    mockSender := &MockWoLPacketSender{}
    useCase := NewWoLUseCase(mockRepo, mockSender, logger, metrics)

    // When: Execute the function being tested
    err := useCase.SendWoLPacket(ctx, machineID)

    // Then: Verify results and side effects
    assert.NoError(t, err)
    assert.Equal(t, 1, mockSender.SendCallCount())
    assert.Equal(t, "AA:BB:CC:DD:EE:FF", mockSender.LastMAC())
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
type MockMachineRepository struct {
    GetMachineCalls []*domain.GetMachineCall
    GetMachineFunc  func(context.Context, string) (*domain.Machine, error)
}

func (m *MockMachineRepository) GetMachine(ctx context.Context, id string) (*domain.Machine, error) {
    m.GetMachineCalls = append(m.GetMachineCalls, &domain.GetMachineCall{ID: id})
    if m.GetMachineFunc != nil {
        return m.GetMachineFunc(ctx, id)
    }
    return nil, nil
}

func (m *MockMachineRepository) GetMachineCallCount() int {
    return len(m.GetMachineCalls)
}
```

### Table-Driven Tests

Use table-driven tests for multiple scenarios with the same logic:

```go
func TestMachine_Validate(t *testing.T) {
    tests := []struct {
        name    string
        machine domain.Machine
        wantErr bool
        errType error
    }{
        {
            name:    "valid machine with all fields",
            machine: domain.Machine{ID: "test", MAC: "AA:BB:CC:DD:EE:FF", Broadcast: "192.168.1.255"},
            wantErr: false,
        },
        {
            name:    "invalid MAC address format",
            machine: domain.Machine{ID: "test", MAC: "invalid", Broadcast: "192.168.1.255"},
            wantErr: true,
            errType: domain.ErrInvalidMAC,
        },
        {
            name:    "empty machine ID",
            machine: domain.Machine{ID: "", MAC: "AA:BB:CC:DD:EE:FF", Broadcast: "192.168.1.255"},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.machine.Validate()
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
// testdata.go - Helper functions for test data
func NewValidMachine(id string) *domain.Machine {
    return &domain.Machine{
        ID:        id,
    }
}

func NewMachineWithMAC(id, mac string) *domain.Machine {
    machine := NewValidMachine(id)
    return machine
}

// In tests:
func TestSomething(t *testing.T) {
    machine := NewValidMachine("saruman")
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
- `domain` - Domain layer (entities, errors)
- `usecase` - Use case/business logic layer
- `delivery` - Delivery layer (HTTP handlers, routes)
- `repository` - Data access layer
- `infrastructure` - Infrastructure concerns (logging, metrics, WoL)
- `config` - Configuration files
- `ci` - CI/CD workflows

**Examples:**

```
feat(delivery): add machine wake status endpoint

Add GET /api/v1/machines/:id/status endpoint to check
if a machine is currently online.

Implements health check via ICMP ping to broadcast address.
Includes request correlation and structured logging.

Closes #42
```

```
fix(usecase): handle nil machine in WoL packet sender

Prevent panic when machine pointer is nil by adding
explicit nil check before accessing MAC address.

Fixes #35
```

```
test(domain): add comprehensive Machine validation tests

Use table-driven tests to cover all validation scenarios:
- Valid machines
- Boundary conditions

Achieves 100% coverage for Machine.Validate() method
```

```
refactor(infrastructure): extract WoL packet builder to separate function

Extract WoL magic packet construction into BuildMagicPacket function
for reusability and testability. No behavior change.
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
go test ./internal/domain
go test ./internal/usecase -v

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
export SMAUG_CONFIG=configs/services.yaml
just run
```

## Common Tasks

### Adding a New Machine Attribute

1. Update `Machine` struct in `internal/model/machine.go`
2. Add validation logic in `Validate()` method
3. Update `services.yaml` schema and example
4. Add tests in `machine_test.go`
5. Update store to parse new field
6. Ensure 85%+ test coverage

### Adding a New HTTP Endpoint

1. Define handler method in `internal/handler/handler.go`
2. Register route in main router setup
3. Add authentication middleware if needed
4. Create tests in `handler_test.go`
5. Document endpoint in README.md
6. Add Prometheus metrics if applicable
7. Ensure 90%+ test coverage

### Adding a New Service Method

1. Define method in `internal/service/` (e.g., `service/router.go`)
2. Add structured logging
3. Record metrics for important operations
4. Handle errors appropriately
5. Create comprehensive tests
6. Ensure 95%+ test coverage

### Adding a New Domain Error

1. Define error in `internal/model/errors.go`
2. Use in appropriate layers
3. Map to HTTP status code in handler layer
4. Add tests for error handling
5. Document in code comments

## Important Files

### Entry Point
- `cmd/smaug/main.go` - Application bootstrap, dependency injection setup

### Configuration
- `configs/services.yaml` - Services and routes configuration
- `internal/config/config.go` - Configuration loading and parsing

### Models & Types
- `internal/model/machine.go` - Machine entity
- `internal/model/route.go` - Route entity
- `internal/model/errors.go` - Domain-specific errors

### HTTP Layer
- `internal/handler/handler.go` - Request handlers and routes
- `internal/handler/proxy.go` - Reverse proxy handler
- `internal/middleware/middleware.go` - Logging, auth, correlation
- `internal/middleware/auth.go` - API key authentication

### Business Logic
- `internal/service/wol.go` - WoL packet sending logic
- `internal/service/router.go` - Request routing and proxying logic
- `internal/service/health.go` - Health check logic

### Data Access
- `internal/store/config.go` - Configuration loading from YAML

### Utilities
- `internal/util/logger.go` - Structured logging
- `internal/util/metrics.go` - Prometheus metrics

## Configuration

Smaug uses a unified YAML configuration file (services.yaml) that includes all settings: server, routing, wake-on-LAN, and observability.

### Configuration File Format

```yaml
server:
  port: 8080
  log:
    format: json          # json or text
    level: info           # debug, info, warn, error

authentication:
  api_key: "secret-key"  # Optional: leave empty for public endpoints

routes:
  - path: /service-name
    target: "http://192.168.1.100:8080"
    machine_id: "gandalf"  # Wake this machine on request (optional)

machines:
  - id: gandalf                 # Required: unique identifier
    name: "Gandalf Server"      # Required: human-readable name

observability:
  health_check:
    enabled: true             # Enable /health, /live, /ready endpoints
  metrics:
    enabled: true             # Enable /metrics endpoint
```

### Environment Variable Overrides

Environment variables override configuration file values:

```bash
SMAUG_CONFIG=/etc/smaug/services.yaml   # Config file path (default: /etc/smaug/services.yaml)
SMAUG_PORT=8080                         # Overrides server.port
SMAUG_LOG_FORMAT=json                   # Overrides server.log.format (json|text)
SMAUG_LOG_LEVEL=info                    # Overrides server.log.level (debug|info|warn|error)
SMAUG_API_KEY=secret-key                # Overrides authentication.api_key
```

## Security Considerations

### API Key Authentication

- Set `SMAUG_API_KEY` to enable authentication
- All WoL/machine endpoints require `X-API-Key` header
- Health/metrics endpoints are always public
- Never log API keys

### Allowlist-Based Access

- Only machines in `services.yaml` can receive WoL packets
- No dynamic machine registration
- Validate all inputs
- Use domain validation methods

### Network Security

- Runs with `hostNetwork: true` in Kubernetes (required for broadcast)
- Should be protected by NetworkPolicy
- Only accessible from trusted services (e.g., Smaug itself)

## Observability

### Structured Logging

All logs include:
- `request_id`: Correlation ID for request tracing
- `machine_id`: Target machine identifier
- `operation`: Operation being performed
- Appropriate log level (INFO, WARN, ERROR)

### Prometheus Metrics

Key metrics:
- `smaug_wol_packets_sent_total` - Successful WoL packets
- `smaug_wol_packets_failed_total` - Failed WoL packets
- `smaug_machine_not_found_total` - Machine not found errors
- `smaug_request_duration_seconds` - Request latency histogram
- `smaug_configured_machines_total` - Number of configured machines

## Troubleshooting

### Tests Failing

1. Check test coverage: `just test`
2. Run specific failing test: `go test ./path -run TestName -v`
3. Check for race conditions: `go test -race ./...`
4. Verify mocks are properly configured

### Linter Errors

1. Run `just format` to auto-fix formatting
2. Run `just lint` to see specific issues
3. Check `golangci-lint` configuration in `.golangci.yml`
4. Fix issues one by one, maintain code quality

### Build Issues

1. Verify Go version: `go version` (need 1.23+)
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
- Keep dependencies flowing in one direction (handler → service → store)
- Write tests BEFORE implementation (TDD)
- Use structured logging with context
- Record metrics for important operations
- Validate inputs at model/service layer
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
- [2026-02-08-service-architecture.md](docs/adrs/2026-02-08-service-architecture.md) - Architecture details

## Quick Reference

```bash
# Install dependencies
just ci

# Run all checks
just pre-commit

# Run tests
just test

# Run locally
export SMAUG_CONFIG=configs/services.yaml
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
