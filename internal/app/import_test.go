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
	cfg.Storage.UserData = common.FileConfig{Path: filepath.Join(dir, "user"), Versions: 0}
	cfg.Storage.Data = common.FileConfig{Path: filepath.Join(dir, "data"), Versions: 0}

	mgr, err := storage.NewStorageManager(logger, cfg)
	if err != nil {
		t.Fatalf("NewStorageManager failed: %v", err)
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

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 2 {
		t.Errorf("expected 2 imported, got %d", imported)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Verify alice has the new fields
	user, err := mgr.UserStorage().GetUser(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetUser alice failed: %v", err)
	}
	if user.DisplayCurrency != "USD" {
		t.Errorf("expected display_currency=USD, got %q", user.DisplayCurrency)
	}
	if user.DefaultPortfolio != "Growth" {
		t.Errorf("expected default_portfolio=Growth, got %q", user.DefaultPortfolio)
	}
	if len(user.Portfolios) != 2 || user.Portfolios[0] != "Growth" || user.Portfolios[1] != "Income" {
		t.Errorf("expected portfolios=[Growth, Income], got %v", user.Portfolios)
	}

	// Verify bob exists
	_, err = mgr.UserStorage().GetUser(context.Background(), "bob")
	if err != nil {
		t.Errorf("expected bob to exist, got error: %v", err)
	}
}

func TestImportUsersFromFile_NonExistentFile(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	_, _, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, "/nonexistent/path/users.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestImportUsersFromFile_InvalidJSON(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	filePath := filepath.Join(t.TempDir(), "users.json")
	os.WriteFile(filePath, []byte("{{invalid json"), 0644)

	_, _, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestImportUsersFromFile_Idempotent(t *testing.T) {
	mgr := newTestUserStore(t)
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})

	// Pre-create alice
	mgr.UserStorage().SaveUser(context.Background(), &models.User{
		Username:     "alice",
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

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
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
	user, _ := mgr.UserStorage().GetUser(context.Background(), "alice")
	if user.Email != "existing@example.com" {
		t.Errorf("expected alice's email unchanged, got %q", user.Email)
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

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
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

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
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

	_, _, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
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

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
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
	user, err := mgr.UserStorage().GetUser(context.Background(), "emptypass")
	if err != nil {
		t.Fatalf("expected user to exist: %v", err)
	}
	if user.PasswordHash == "" {
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

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
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

	imported, _, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
	if err != nil {
		t.Fatalf("ImportUsersFromFile failed: %v", err)
	}
	if imported != 2 {
		t.Errorf("expected 2 imported, got %d", imported)
	}

	// "noportfolios" should have nil portfolios
	u1, _ := mgr.UserStorage().GetUser(context.Background(), "noportfolios")
	if u1.Portfolios != nil {
		t.Errorf("expected nil portfolios for absent field, got %v", u1.Portfolios)
	}

	// "emptyportfolios" — after round-trip through JSON storage with omitempty,
	// an empty slice is omitted on write and becomes nil on read-back.
	u2, _ := mgr.UserStorage().GetUser(context.Background(), "emptyportfolios")
	if len(u2.Portfolios) != 0 {
		t.Errorf("expected no portfolios for empty array, got %v", u2.Portfolios)
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

	imported, skipped, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
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
	user, _ := mgr.UserStorage().GetUser(context.Background(), "dupe")
	if user.Email != "first@x.com" {
		t.Errorf("expected first occurrence email, got %q", user.Email)
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

	_, _, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
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

	_, _, err := ImportUsersFromFile(context.Background(), mgr.UserStorage(), logger, filePath)
	if err == nil {
		t.Fatal("expected error for truncated JSON")
	}
}
