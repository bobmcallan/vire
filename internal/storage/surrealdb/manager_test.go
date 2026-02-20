package surrealdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	tcommon "github.com/bobmcallan/vire/tests/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig(t *testing.T) *common.Config {
	t.Helper()
	sc := tcommon.StartSurrealDB(t)
	dataPath := t.TempDir()

	return &common.Config{
		Environment: "test",
		Storage: common.StorageConfig{
			Address:   sc.Address(),
			Namespace: "vire_test",
			Database:  fmt.Sprintf("mgr_%s_%d", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano()%100000),
			Username:  "root",
			Password:  "root",
			DataPath:  dataPath,
		},
	}
}

func TestNewManager(t *testing.T) {
	cfg := testConfig(t)
	logger := common.NewSilentLogger()

	mgr, err := NewManager(logger, cfg)
	require.NoError(t, err)
	defer mgr.Close()

	assert.NotNil(t, mgr.InternalStore())
	assert.NotNil(t, mgr.UserDataStore())
	assert.NotNil(t, mgr.MarketDataStorage())
	assert.NotNil(t, mgr.SignalStorage())
	assert.Equal(t, cfg.Storage.DataPath, mgr.DataPath())
}

func TestWriteRaw(t *testing.T) {
	cfg := testConfig(t)
	logger := common.NewSilentLogger()

	mgr, err := NewManager(logger, cfg)
	require.NoError(t, err)
	defer mgr.Close()

	data := []byte("chart image data")
	err = mgr.WriteRaw("charts", "test-chart.png", data)
	require.NoError(t, err)

	// Verify file was written
	written, err := os.ReadFile(filepath.Join(cfg.Storage.DataPath, "charts", "test-chart.png"))
	require.NoError(t, err)
	assert.Equal(t, data, written)
}

func TestWriteRawAtomicity(t *testing.T) {
	cfg := testConfig(t)
	logger := common.NewSilentLogger()

	mgr, err := NewManager(logger, cfg)
	require.NoError(t, err)
	defer mgr.Close()

	// Write initial version
	err = mgr.WriteRaw("charts", "atomic.png", []byte("v1"))
	require.NoError(t, err)

	// Overwrite with new version
	err = mgr.WriteRaw("charts", "atomic.png", []byte("v2"))
	require.NoError(t, err)

	written, err := os.ReadFile(filepath.Join(cfg.Storage.DataPath, "charts", "atomic.png"))
	require.NoError(t, err)
	assert.Equal(t, []byte("v2"), written)

	// Verify no temp file is left behind
	_, err = os.Stat(filepath.Join(cfg.Storage.DataPath, "charts", "atomic.png.tmp"))
	assert.True(t, os.IsNotExist(err))
}

func TestPurgeDerivedData(t *testing.T) {
	cfg := testConfig(t)
	logger := common.NewSilentLogger()

	mgr, err := NewManager(logger, cfg)
	require.NoError(t, err)
	defer mgr.Close()

	ctx := context.Background()

	// Add some data to purge
	userStore := mgr.userStore
	for _, sub := range []string{"portfolio", "report", "search"} {
		require.NoError(t, userStore.Put(ctx, &models.UserRecord{
			UserID:  "purgeuser",
			Subject: sub,
			Key:     "item",
			Value:   "data",
			Version: 1,
		}))
	}

	counts, err := mgr.PurgeDerivedData(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, counts["user_records"], 3)
}

func TestPurgeReports(t *testing.T) {
	cfg := testConfig(t)
	logger := common.NewSilentLogger()

	mgr, err := NewManager(logger, cfg)
	require.NoError(t, err)
	defer mgr.Close()

	ctx := context.Background()

	// Add report records
	for _, key := range []string{"r1", "r2"} {
		require.NoError(t, mgr.userStore.Put(ctx, &models.UserRecord{
			UserID:  "reportuser",
			Subject: "report",
			Key:     key,
			Value:   "report_data",
			Version: 1,
		}))
	}

	count, err := mgr.PurgeReports(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestClose(t *testing.T) {
	cfg := testConfig(t)
	logger := common.NewSilentLogger()

	mgr, err := NewManager(logger, cfg)
	require.NoError(t, err)

	err = mgr.Close()
	assert.NoError(t, err)
}
