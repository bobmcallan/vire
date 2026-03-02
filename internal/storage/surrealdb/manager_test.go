package surrealdb

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/storage/blob"
	tcommon "github.com/bobmcallan/vire/tests/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig(t *testing.T) (*common.Config, string) {
	t.Helper()
	sc := tcommon.StartSurrealDB(t)
	dataPath := t.TempDir()
	blobPath := t.TempDir()

	cfg := &common.Config{
		Environment: "test",
		Storage: common.StorageConfig{
			Address:   sc.Address(),
			Namespace: "vire_test",
			Database:  fmt.Sprintf("mgr_%s_%d", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano()%100000),
			Username:  "root",
			Password:  "root",
			DataPath:  dataPath,
			Blob: common.BlobConfig{
				Type: "file",
				Path: blobPath,
			},
		},
	}
	return cfg, blobPath
}

func testManagerWithBlob(t *testing.T) (*Manager, string) {
	t.Helper()
	cfg, blobPath := testConfig(t)
	logger := common.NewSilentLogger()

	fileStore, err := blob.NewFileSystemStore(blobPath, logger)
	require.NoError(t, err)

	mgr, err := NewManager(logger, cfg, fileStore)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })
	return mgr, blobPath
}

func TestNewManager(t *testing.T) {
	cfg, blobPath := testConfig(t)
	logger := common.NewSilentLogger()

	fileStore, err := blob.NewFileSystemStore(blobPath, logger)
	require.NoError(t, err)

	mgr, err := NewManager(logger, cfg, fileStore)
	require.NoError(t, err)
	defer mgr.Close()

	assert.NotNil(t, mgr.InternalStore())
	assert.NotNil(t, mgr.UserDataStore())
	assert.NotNil(t, mgr.MarketDataStorage())
	assert.NotNil(t, mgr.SignalStorage())
	assert.Equal(t, cfg.Storage.DataPath, mgr.DataPath())
}

func TestWriteRaw(t *testing.T) {
	mgr, blobPath := testManagerWithBlob(t)

	data := []byte("chart image data")
	err := mgr.WriteRaw("charts", "test-chart.png", data)
	require.NoError(t, err)

	// Verify file was written to the blob store path
	// WriteRaw calls fileStore.SaveFile(ctx, "chart", "charts/test-chart.png", ...)
	// FileSystemStore saves to {blobPath}/chart/charts/test-chart.png
	fileStore := mgr.fileStore
	got, _, err := fileStore.GetFile(context.Background(), "chart", "charts/test-chart.png")
	require.NoError(t, err)
	assert.True(t, bytes.Equal(data, got))
	_ = blobPath
}

func TestWriteRawAtomicity(t *testing.T) {
	mgr, _ := testManagerWithBlob(t)
	ctx := context.Background()

	// Write initial version
	err := mgr.WriteRaw("charts", "atomic.png", []byte("v1"))
	require.NoError(t, err)

	// Overwrite with new version
	err = mgr.WriteRaw("charts", "atomic.png", []byte("v2"))
	require.NoError(t, err)

	fileStore := mgr.fileStore
	got, _, err := fileStore.GetFile(ctx, "chart", "charts/atomic.png")
	require.NoError(t, err)
	assert.Equal(t, []byte("v2"), got)
}

func TestPurgeDerivedData(t *testing.T) {
	mgr, _ := testManagerWithBlob(t)

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
	mgr, _ := testManagerWithBlob(t)

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
	mgr, _ := testManagerWithBlob(t)

	err := mgr.Close()
	assert.NoError(t, err)
}
