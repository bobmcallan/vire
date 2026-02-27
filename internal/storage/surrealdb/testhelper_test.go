package surrealdb

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	tcommon "github.com/bobmcallan/vire/tests/common"
	surreal "github.com/surrealdb/surrealdb.go"
)

// testDB starts the shared SurrealDB container and returns a connected *surreal.DB
// using a unique database name per test to ensure isolation.
func testDB(t *testing.T) *surreal.DB {
	t.Helper()

	sc := tcommon.StartSurrealDB(t)
	ctx := context.Background()

	db, err := surreal.New(sc.Address())
	if err != nil {
		t.Fatalf("connect to SurrealDB: %v", err)
	}

	if _, err := db.SignIn(ctx, map[string]interface{}{
		"user": "root",
		"pass": "root",
	}); err != nil {
		t.Fatalf("sign in to SurrealDB: %v", err)
	}

	// Use a unique database per test for isolation.
	// Sanitize t.Name() because subtests produce names like "Test/subtest"
	// and SurrealDB rejects "/" in database names.
	sanitized := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dbName := fmt.Sprintf("t_%s_%d", sanitized, time.Now().UnixNano()%100000)
	if err := db.Use(ctx, "vire_test", dbName); err != nil {
		t.Fatalf("select namespace/database: %v", err)
	}

	// Define tables (SurrealDB v3 errors on querying non-existent tables)
	tables := []string{"user", "user_kv", "system_kv", "user_data", "market_data", "signals", "stock_index", "job_queue", "files", "mcp_feedback", "oauth_client", "oauth_code", "oauth_refresh_token", "mcp_auth_session"}
	for _, table := range tables {
		sql := fmt.Sprintf("DEFINE TABLE IF NOT EXISTS %s SCHEMALESS", table)
		if _, err := surreal.Query[any](ctx, db, sql, nil); err != nil {
			t.Fatalf("define table %s: %v", table, err)
		}
	}

	t.Cleanup(func() {
		db.Close(context.Background())
	})

	return db
}

// testLogger returns a silent logger for tests.
func testLogger() *common.Logger {
	return common.NewSilentLogger()
}
