# AGENTS.md - Guidance for Agentic Coding

This file provides guidance for AI agents working on this Kafka MCP Server codebase.

## Build, Test, and Lint Commands

### Building
```bash
make build              # Build binary to bin/kafka-mcp-server
make run-dev           # Run directly from source: go run cmd/main.go
make run               # Run the built binary
```

### Testing
```bash
make test              # Run all tests (requires Docker for Kafka containers)
make test-no-kafka    # Skip Kafka integration tests: SKIP_KAFKA_TESTS=true go test ./...
go test -v ./...      # Run tests verbosely
go test -v -race ./...  # Run tests with race detector
go test -v -run TestFunctionName ./internal/kafka  # Run single test
go test -v -run TestFunctionName -count=1 ./...   # Run single test (disable caching)
```

### Code Quality
```bash
make lint             # Run all linters (golangci-lint + go mod tidy)
golangci-lint run --timeout=5m  # Run linter directly
go mod tidy           # Clean up module dependencies
```

### Docker
```bash
make run-docker       # Build and run Docker container
make docker-compose-up   # Start with Docker Compose
make docker-compose-down # Stop Docker Compose
```

---

## Code Style Guidelines

### Project Structure
- Entry point: `cmd/main.go`
- Internal packages: `internal/kafka/`, `internal/mcp/`, `internal/config/`
- Interfaces defined in separate files: `internal/kafka/interface.go`

### Imports
- Standard library imports first
- Third-party imports second
- Group imports by blank line
- Use canonical import paths: `github.com/tuannvm/kafka-mcp-server/internal/...`

```go
import (
    "context"
    "fmt"
    "log/slog"
    "os"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/tuannvm/kafka-mcp-server/internal/config"
    "github.com/tuannvm/kafka-mcp-server/internal/kafka"
    "github.com/twmb/franz-go/pkg/kgo"
)
```

### Naming Conventions
- **Packages**: lowercase, short names (e.g., `kafka`, `mcp`, `config`)
- **Interfaces**: `KafkaClient`, `ConsumerGroupInfo` - use descriptive nouns
- **Functions**: `NewClient`, `ProduceMessage`, `ListTopics` - PascalCase
- **Variables**: `kafkaClient`, `testBroker`, `maxMessages` - camelCase
- **Constants**: `MaxRetries`, `DefaultTimeout` - PascalCase
- **Types**: `TopicMetadata`, `ClusterOverviewResult` - PascalCase with descriptive nouns
- **Errors**: `errFoo` - prefix with `err` for error variables

### Types and Structs
- Use structs for data transfer objects with JSON tags
- Always implement interfaces explicitly to verify implementations:
  ```go
  var _ KafkaClient = (*Client)(nil)
  ```
- Use meaningful struct field names matching Kafka concepts
- JSON tags for all exported fields (used in MCP responses)

### Error Handling
- Return errors with context using `fmt.Errorf("...: %w", err)`
- Use `log/slog` for structured logging:
  ```go
  slog.InfoContext(ctx, "Operation description", "key", value)
  slog.ErrorContext(ctx, "Operation failed", "error", err)
  ```
- Log sensitive data carefully - avoid logging keys/passwords
- Use descriptive error messages for MCP tool results

### Context Usage
- Always use `context.Context` as first parameter for methods
- Use context for logging with `slog.InfoContext` and `slog.ErrorContext`
- Pass context through call chains

### Testing
- Use `testify/require` and `testify/assert` for assertions
- Integration tests use testcontainers-go for Kafka
- Skip tests when Kafka unavailable:
  ```go
  if testKafkaContainer == nil {
      t.Skip("Skipping test: Kafka container not available")
  }
  ```
- Use `TestMain` for container lifecycle management
- Environment variable `SKIP_KAFKA_TESTS=true` skips integration tests

### MCP Tools
- Use `mcp.NewTool()` with description and parameters
- Use `mcp.WithDescription()` for tool and parameter documentation
- Use appropriate parameter types: `mcp.WithString()`, `mcp.WithNumber()`, `mcp.WithBoolean()`, `mcp.WithArray()`
- Mark required parameters with `mcp.Required()`
- Return results using `mcp.NewToolResultText()` or `mcp.NewToolResultError()`
- JSON marshal results for structured output

### Configuration
- All config via environment variables (see `internal/config/config.go`)
- Use `config.Config` struct for configuration
- Provide sensible defaults

### Logging
- Use `log/slog` for structured logging
- Include relevant context in log messages
- Use appropriate log levels: Debug, Info, Warn, Error

---

## Architecture Notes

### Kafka Client Interface
The `KafkaClient` interface in `internal/kafka/interface.go` defines all Kafka operations. The implementation in `internal/kafka/client.go` wraps `franz-go`. This interface enables dependency injection and testing with mocks.

### MCP Server
The MCP server in `internal/mcp/` exposes:
- **Tools**: `produce_message`, `consume_messages`, `list_topics`, `describe_topic`, etc.
- **Resources**: Cluster health and diagnostics
- **Prompts**: Pre-configured workflows

### Key Dependencies
- `github.com/twmb/franz-go` - Kafka client
- `github.com/mark3labs/mcp-go` - MCP protocol
- `github.com/stretchr/testify` - Testing assertions

---

## Common Patterns

### Adding a New Tool
1. Define tool in `internal/mcp/tools.go`
2. Add parameter validation
3. Call appropriate `kafkaClient` method
4. Marshal result to JSON and return via `mcp.NewToolResultText()`

### Adding a New Kafka Operation
1. Add method to `KafkaClient` interface in `internal/kafka/interface.go`
2. Implement in `internal/kafka/client.go`
3. Use `franz-go` for Kafka operations
4. Return structured data with JSON tags
