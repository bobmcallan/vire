package common

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	surrealOnce      sync.Once
	surrealContainer *SurrealDBContainer
	surrealError     error
)

// SurrealDBContainer wraps a testcontainers SurrealDB instance.
type SurrealDBContainer struct {
	container testcontainers.Container
	host      string
	port      string
}

// StartSurrealDB starts a shared SurrealDB container for the test run.
// Uses sync.Once so only one container is created per process.
func StartSurrealDB(t *testing.T) *SurrealDBContainer {
	t.Helper()

	surrealOnce.Do(func() {
		ctx := context.Background()

		req := testcontainers.ContainerRequest{
			Image:        "surrealdb/surrealdb:v3.0.0",
			ExposedPorts: []string{"8000/tcp"},
			Cmd:          []string{"start", "--user", "root", "--pass", "root"},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort("8000/tcp"),
				wait.ForLog("Started web server"),
			).WithDeadline(60 * time.Second),
		}

		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		if err != nil {
			surrealError = fmt.Errorf("start SurrealDB container: %w", err)
			return
		}

		host, err := container.Host(ctx)
		if err != nil {
			container.Terminate(ctx)
			surrealError = fmt.Errorf("get SurrealDB host: %w", err)
			return
		}

		mappedPort, err := container.MappedPort(ctx, "8000/tcp")
		if err != nil {
			container.Terminate(ctx)
			surrealError = fmt.Errorf("get SurrealDB port: %w", err)
			return
		}

		surrealContainer = &SurrealDBContainer{
			container: container,
			host:      host,
			port:      mappedPort.Port(),
		}
	})

	if surrealError != nil {
		t.Fatalf("SurrealDB container failed: %v", surrealError)
	}

	return surrealContainer
}

// Address returns the WebSocket RPC address for SurrealDB.
func (c *SurrealDBContainer) Address() string {
	return fmt.Sprintf("ws://%s:%s/rpc", c.host, c.port)
}

// Cleanup terminates the container. Call from TestMain if needed.
func (c *SurrealDBContainer) Cleanup() {
	if c != nil && c.container != nil {
		c.container.Terminate(context.Background())
	}
}
