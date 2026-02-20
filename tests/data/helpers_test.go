package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	surrealdb "github.com/bobmcallan/vire/internal/storage/surrealdb"
	tcommon "github.com/bobmcallan/vire/tests/common"
)

// testManager creates a StorageManager connected to the shared SurrealDB container
// with a unique database per test for isolation.
func testManager(t *testing.T) interfaces.StorageManager {
	t.Helper()

	sc := tcommon.StartSurrealDB(t)
	dataPath := t.TempDir()

	cfg := &common.Config{
		Environment: "test",
		Storage: common.StorageConfig{
			Address:   sc.Address(),
			Namespace: "vire_data_test",
			Database:  fmt.Sprintf("d_%s_%d", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano()%100000),
			Username:  "root",
			Password:  "root",
			DataPath:  dataPath,
		},
	}

	logger := common.NewSilentLogger()
	mgr, err := surrealdb.NewManager(logger, cfg)
	if err != nil {
		t.Fatalf("create storage manager: %v", err)
	}

	t.Cleanup(func() {
		mgr.Close()
	})

	return mgr
}

// testContext returns a background context.
func testContext() context.Context {
	return context.Background()
}
