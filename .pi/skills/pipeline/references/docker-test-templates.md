# Docker Test Templates

This document provides templates and patterns for creating Docker-based integration tests.

## Docker Compose Template

### Basic Template (Database + API)

```yaml
# tests/docker/docker-compose.yml
version: '3.8'

services:
  db:
    image: surrealdb/surrealdb:latest
    container_name: test-db
    ports:
      - "${DB_PORT:-8000}:8000"
    environment:
      - SURREALDB_USER=${DB_USER:-root}
      - SURREALDB_PASS=${DB_PASS:-root}
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8000/health"]
      interval: 2s
      timeout: 3s
      retries: 10
      start_period: 5s
    networks:
      - test-network

  api:
    build:
      context: ../..
      dockerfile: Dockerfile.test
    container_name: test-api
    ports:
      - "${API_PORT:-8080}:8080"
    depends_on:
      db:
        condition: service_healthy
    environment:
      - DATABASE_URL=ws://db:8000/rpc
      - DATABASE_USER=${DB_USER:-root}
      - DATABASE_PASS=${DB_PASS:-root}
      - LOG_LEVEL=debug
    networks:
      - test-network

networks:
  test-network:
    driver: bridge
```

### Environment File Template

```bash
# tests/docker/.env
DB_PORT=8000
DB_USER=root
DB_PASS=root
API_PORT=8080
LOG_LEVEL=debug
```

## Test File Templates

### Go Integration Test Template

```go
// tests/integration/feature_test.go
package integration

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestFeatureIntegration(t *testing.T) {
    // Setup: Create unique namespace for isolation
    ns := fmt.Sprintf("test_%s_%d", t.Name(), time.Now().UnixNano())

    ctx := context.Background()
    client, err := NewTestClient(ns)
    require.NoError(t, err, "Failed to create test client")
    defer client.Close()

    // Cleanup: Remove test data
    t.Cleanup(func() {
        client.Cleanup(ctx)
    })

    // Test: Happy path
    t.Run("happy_path", func(t *testing.T) {
        result, err := client.DoFeature(ctx, FeatureInput{
            Field: "value",
        })

        require.NoError(t, err)
        assert.NotNil(t, result)
        assert.Equal(t, "expected", result.Field)
    })

    // Test: Error case
    t.Run("invalid_input", func(t *testing.T) {
        _, err := client.DoFeature(ctx, FeatureInput{
            Field: "", // Invalid empty field
        })

        assert.Error(t, err)
        assert.Contains(t, err.Error(), "field is required")
    })

    // Test: Edge case from devil's-advocate
    t.Run("edge_case_unicode", func(t *testing.T) {
        result, err := client.DoFeature(ctx, FeatureInput{
            Field: "日本語\U0001F4A9", // Unicode + emoji
        })

        require.NoError(t, err)
        assert.Equal(t, "日本語\U0001F4A9", result.Field)
    })
}
```

### Test Helper Template

```go
// tests/common/client.go
package common

import (
    "context"
    "fmt"
    "os"
    "testing"
    "time"
)

type TestClient struct {
    ns      string
    client  *Client
    baseURL string
}

func NewTestClient(namespace string) (*TestClient, error) {
    baseURL := os.Getenv("API_URL")
    if baseURL == "" {
        baseURL = "http://localhost:8080"
    }

    client, err := NewClient(baseURL)
    if err != nil {
        return nil, err
    }

    return &TestClient{
        ns:      namespace,
        client:  client,
        baseURL: baseURL,
    }, nil
}

func (tc *TestClient) Close() {
    tc.client.Close()
}

func (tc *TestClient) Cleanup(ctx context.Context) {
    // Remove all test data in namespace
    tc.client.Query(ctx, "REMOVE NAMESPACE $ns", map[string]any{
        "ns": tc.ns,
    })
}

// Helper to generate unique identifiers
func UniqueID(prefix string) string {
    return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
```

### Docker Test Runner

```go
// tests/common/docker.go
package common

import (
    "bytes"
    "context"
    "os"
    "os/exec"
    "testing"
    "time"
)

type DockerRunner struct {
    composeFile string
    envFile     string
}

func NewDockerRunner() *DockerRunner {
    return &DockerRunner{
        composeFile: "tests/docker/docker-compose.yml",
        envFile:     "tests/docker/.env",
    }
}

func (dr *DockerRunner) Start(t *testing.T) error {
    t.Helper()

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx,
        "docker-compose",
        "-f", dr.composeFile,
        "--env-file", dr.envFile,
        "up", "-d",
    )

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start containers: %w\n%s", err, stderr.String())
    }

    // Wait for services to be healthy
    return dr.waitForHealth(t, 30*time.Second)
}

func (dr *DockerRunner) Stop(t *testing.T) error {
    t.Helper()

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx,
        "docker-compose",
        "-f", dr.composeFile,
        "down", "-v",
    )

    return cmd.Run()
}

func (dr *DockerRunner) waitForHealth(t *testing.T, timeout time.Duration) error {
    t.Helper()

    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        // Check API health endpoint
        resp, err := http.Get("http://localhost:8080/health")
        if err == nil && resp.StatusCode == 200 {
            resp.Body.Close()
            return nil
        }
        time.Sleep(1 * time.Second)
    }

    return fmt.Errorf("services not healthy after %v", timeout)
}
```

## Test Patterns

### Pattern 1: Lifecycle Test (CRUD)

```go
func TestFeatureLifecycle(t *testing.T) {
    client := NewTestClient(t)
    defer client.Close()

    ctx := context.Background()

    // Create
    created, err := client.Create(ctx, Feature{
        Name: "test",
    })
    require.NoError(t, err)

    // Read
    read, err := client.Get(ctx, created.ID)
    require.NoError(t, err)
    assert.Equal(t, created.Name, read.Name)

    // Update
    updated, err := client.Update(ctx, created.ID, Feature{
        Name: "updated",
    })
    require.NoError(t, err)
    assert.Equal(t, "updated", updated.Name)

    // Delete
    err = client.Delete(ctx, created.ID)
    require.NoError(t, err)

    // Verify deleted
    _, err = client.Get(ctx, created.ID)
    assert.Error(t, err)
}
```

### Pattern 2: Table-Driven Tests

```go
func TestFeatureValidation(t *testing.T) {
    client := NewTestClient(t)
    defer client.Close()

    tests := []struct {
        name    string
        input   Feature
        wantErr bool
        errMsg  string
    }{
        {
            name:    "empty_name",
            input:   Feature{Name: ""},
            wantErr: true,
            errMsg:  "name is required",
        },
        {
            name:    "valid",
            input:   Feature{Name: "test"},
            wantErr: false,
        },
        {
            name:    "max_length",
            input:   Feature{Name: strings.Repeat("x", 256)},
            wantErr: true,
            errMsg:  "name too long",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := client.Create(context.Background(), tt.input)

            if tt.wantErr {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Pattern 3: Concurrent Access Test

```go
func TestFeatureConcurrency(t *testing.T) {
    client := NewTestClient(t)
    defer client.Close()

    // Create shared resource
    resource, err := client.Create(context.Background(), Feature{
        Name: "shared",
        Count: 0,
    })
    require.NoError(t, err)

    // Run concurrent updates
    var wg sync.WaitGroup
    errors := make(chan error, 10)

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            _, err := client.Increment(context.Background(), resource.ID)
            if err != nil {
                errors <- err
            }
        }()
    }

    wg.Wait()
    close(errors)

    // Check for errors
    for err := range errors {
        t.Errorf("concurrent update failed: %v", err)
    }

    // Verify final state
    final, err := client.Get(context.Background(), resource.ID)
    require.NoError(t, err)
    assert.Equal(t, 10, final.Count)
}
```

## Test Results Output

```go
// tests/common/results.go
package common

import (
    "encoding/json"
    "os"
    "path/filepath"
    "time"
)

type TestResults struct {
    Timestamp   time.Time `json:"timestamp"`
    Feature     string    `json:"feature"`
    Status      string    `json:"status"`
    Duration    string    `json:"duration"`
    Tests       TestStats `json:"tests"`
    Failures    []Failure `json:"failures,omitempty"`
}

type TestStats struct {
    Total   int `json:"total"`
    Passed  int `json:"passed"`
    Failed  int `json:"failed"`
    Skipped int `json:"skipped"`
}

type Failure struct {
    Test    string `json:"test"`
    Error   string `json:"error"`
    File    string `json:"file"`
    Line    int    `json:"line"`
}

func SaveResults(feature string, stats TestStats, failures []Failure) error {
    results := TestResults{
        Timestamp: time.Now(),
        Feature:   feature,
        Status:    "passed",
        Duration:  "TODO",
        Tests:     stats,
        Failures:  failures,
    }

    if stats.Failed > 0 {
        results.Status = "failed"
    }

    data, err := json.MarshalIndent(results, "", "  ")
    if err != nil {
        return err
    }

    filename := filepath.Join(
        "tests", "logs",
        time.Now().Format("20060102-150405")+"-test.json",
    )

    if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
        return err
    }

    return os.WriteFile(filename, data, 0644)
}
```

## Best Practices

1. **Isolation**: Each test gets its own namespace/database
2. **Cleanup**: Always clean up resources in t.Cleanup()
3. **Parallel-safe**: Use unique identifiers (timestamps, UUIDs)
4. **Health checks**: Wait for services to be ready before testing
5. **Timeouts**: Set reasonable timeouts for all operations
6. **Results**: Output results to JSON for CI/CD integration
7. **Fixtures**: Use test fixtures for complex test data
8. **Mocking**: Mock external services that aren't being tested
