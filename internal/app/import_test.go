package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/storage"
)

func newTestUserStore(t *testing.T) interfaces.StorageManager {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	cfg := common.NewDefaultConfig()
	cfg.Storage.Internal = common.AreaConfig{Path: filepath.Join(dir, "internal")}
	cfg.Storage.User = common.AreaConfig{Path: filepath.Join(dir, "user")}
	cfg.Storage.Market = common.AreaConfig{Path: filepath.Join(dir, "market")}

	mgr, err := storage.NewManager(logger, cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })
	return mgr
}

func TestImportUsersFromFile_Success(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	usersJSON := `{
		"users": [
			{
				"username": "alice",
				"email": "alice@example.com",
				"password": "pass1",
				"role": "admin",
				"display_currency": "USD",
				"default_portfolio": "Growth",
				"portfolios": ["Growth", "Income"]
			},
			{
				"username": "bob",
				"email": "bob@example.com",
				"password": "pass2",
				"role": "user"
			}
		]
	}`

	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(usersJSON), 0644)

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 2 {
		t.Errorf("expected 2 imported, got %d", imported)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Verify alice has the new fields as UserKV
	_, err = mgr.InternalStore().GetUser(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetUser alice failed: %v", err)
	}
	dcKV, _ := mgr.InternalStore().GetUserKV(context.Background(), "alice", "display_currency")
	if dcKV.Value != "USD" {
		t.Errorf("expected display_currency=USD, got %q", dcKV.Value)
	}
	pfKV, _ := mgr.InternalStore().GetUserKV(context.Background(), "alice", "portfolios")
	if pfKV.Value != "Growth,Income" {
		t.Errorf("expected portfolios=Growth,Income, got %q", pfKV.Value)
	}

	// Verify bob exists
	_, err = mgr.InternalStore().GetUser(context.Background(), "bob")
	if err != nil {
		t.Errorf("expected bob to exist, got error: %v", err)
	}
}

func TestImportUsersFromFile_NonExistentFile(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	_, _, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, "/nonexistent/path/users.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestImportUsersFromFile_InvalidJSON(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte("{{invalid json"), 0644)

	_, _, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestImportUsersFromFile_Idempotent(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	// Pre-create alice
	mgr.InternalStore().SaveUser(context.Background(), &models.InternalUser{
		UserID:       "alice",
		Email:        "existing@example.com",
		PasswordHash: "hash",
		Role:         "admin",
	})

	usersJSON := `{
		"users": [
			{"username": "alice", "email": "new@example.com", "password": "pass1", "role": "user"},
			{"username": "bob", "email": "bob@example.com", "password": "pass2", "role": "user"}
		]
	}`
	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(usersJSON), 0644)

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 1 {
		t.Errorf("expected 1 imported, got %d", imported)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", skipped)
	}

	// Verify alice was NOT overwritten
	aliceUser, _ := mgr.InternalStore().GetUser(context.Background(), "alice")
	if aliceUser.Email != "existing@example.com" {
		t.Errorf("expected alice's email unchanged, got %q", aliceUser.Email)
	}
}

func TestImportUsersFromFile_SkipsEmptyUsername(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	usersJSON := `{
		"users": [
			{"username": "", "email": "no-name@example.com", "password": "pass1", "role": "user"},
			{"username": "valid", "email": "valid@example.com", "password": "pass2", "role": "user"}
		]
	}`
	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(usersJSON), 0644)

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 1 {
		t.Errorf("expected 1 imported, got %d", imported)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", skipped)
	}
}

// --- Stress tests: hostile inputs and edge cases ---

func TestImportUsersFromFile_EmptyUsersArray(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	usersJSON := `{"users": []}`
	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(usersJSON), 0644)

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 0 {
		t.Errorf("expected 0 imported, got %d", imported)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
}

func TestImportUsersFromFile_EmptyFile(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(""), 0644)

	_, _, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestImportUsersFromFile_EmptyPassword(t *testing.T) {
	// File import accepts empty passwords — bcrypt will hash ""
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	usersJSON := `{
		"users": [
			{"username": "emptypass", "email": "e@x.com", "password": "", "role": "user"}
		]
	}`
	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(usersJSON), 0644)

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 1 {
		t.Errorf("expected 1 imported, got %d", imported)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Verify user has a valid bcrypt hash (not empty)
	emptyPassUser, err := mgr.InternalStore().GetUser(context.Background(), "emptypass")
	if err != nil {
		t.Fatalf("expected user to exist: %v", err)
	}
	if emptyPassUser.PasswordHash == "" {
		t.Error("expected non-empty password hash even for empty password")
	}
}

func TestImportUsersFromFile_MissingUsersKey(t *testing.T) {
	// JSON with no "users" key — should import 0 with no error
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	usersJSON := `{"something_else": "value"}`
	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(usersJSON), 0644)

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 0 {
		t.Errorf("expected 0 imported, got %d", imported)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
}

func TestImportUsersFromFile_NilPortfolios(t *testing.T) {
	// User with no portfolios field vs empty array — both should work
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	usersJSON := `{
		"users": [
			{"username": "noportfolios", "email": "n@x.com", "password": "pass", "role": "user"},
			{"username": "emptyportfolios", "email": "e@x.com", "password": "pass", "role": "user", "portfolios": []}
		]
	}`
	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(usersJSON), 0644)

	imported, _, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 2 {
		t.Errorf("expected 2 imported, got %d", imported)
	}

	// "noportfolios" should have no portfolios KV entry
	_, err = mgr.InternalStore().GetUserKV(context.Background(), "noportfolios", "portfolios")
	if err == nil {
		t.Error("expected no portfolios KV for absent field")
	}

	// "emptyportfolios" — with empty array, no KV entry should be created
	_, err = mgr.InternalStore().GetUserKV(context.Background(), "emptyportfolios", "portfolios")
	if err == nil {
		t.Error("expected no portfolios KV for empty array")
	}
}

func TestImportUsersFromFile_DuplicatesInSameFile(t *testing.T) {
	// If the same username appears twice in one file, only the first should be imported
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	usersJSON := `{
		"users": [
			{"username": "dupe", "email": "first@x.com", "password": "pass1", "role": "admin"},
			{"username": "dupe", "email": "second@x.com", "password": "pass2", "role": "user"}
		]
	}`
	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(usersJSON), 0644)

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 1 {
		t.Errorf("expected 1 imported (first occurrence), got %d", imported)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped (duplicate), got %d", skipped)
	}

	// Verify the FIRST user's data was kept
	dupeUser, _ := mgr.InternalStore().GetUser(context.Background(), "dupe")
	if dupeUser.Email != "first@x.com" {
		t.Errorf("expected first occurrence email, got %q", dupeUser.Email)
	}
}

func TestImportUsersFromFile_PermissionDenied(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(`{"users":[]}`), 0644)
	// Make unreadable
	os.Chmod(filePath, 0000)
	t.Cleanup(func() { os.Chmod(filePath, 0644) })

	_, _, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
}

func TestImportUsersFromFile_TruncatedJSON(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	// Valid JSON start, truncated mid-object
	usersJSON := `{"users": [{"username": "alice", "email": "a@x.com"`
	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte(usersJSON), 0644)

	_, _, err := ImportUsersFromFile(context.Background(), mgr.InternalStore(), logger, filePath)
	if err == nil {
		t.Fatal("expected error for truncated JSON")
	}
}
