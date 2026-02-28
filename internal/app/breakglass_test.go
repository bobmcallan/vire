package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// mockInternalStore implements interfaces.InternalStore for breakglass unit tests.
type mockInternalStore struct {
	users map[string]*models.InternalUser
}

func newMockInternalStore() *mockInternalStore {
	return &mockInternalStore{users: make(map[string]*models.InternalUser)}
}

func (m *mockInternalStore) GetUser(_ context.Context, userID string) (*models.InternalUser, error) {
	u, ok := m.users[userID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return u, nil
}

func (m *mockInternalStore) SaveUser(_ context.Context, user *models.InternalUser) error {
	m.users[user.UserID] = user
	return nil
}

func (m *mockInternalStore) GetUserByEmail(_ context.Context, email string) (*models.InternalUser, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockInternalStore) DeleteUser(_ context.Context, userID string) error {
	delete(m.users, userID)
	return nil
}

func (m *mockInternalStore) ListUsers(_ context.Context) ([]string, error) {
	var ids []string
	for id := range m.users {
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *mockInternalStore) GetUserKV(_ context.Context, _, _ string) (*models.UserKeyValue, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mockInternalStore) SetUserKV(_ context.Context, _, _, _ string) error { return nil }

func (m *mockInternalStore) DeleteUserKV(_ context.Context, _, _ string) error { return nil }

func (m *mockInternalStore) ListUserKV(_ context.Context, _ string) ([]*models.UserKeyValue, error) {
	return nil, nil
}

func (m *mockInternalStore) GetSystemKV(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("not found")
}

func (m *mockInternalStore) SetSystemKV(_ context.Context, _, _ string) error { return nil }

func (m *mockInternalStore) Close() error { return nil }

func TestEnsureBreakglassAdmin_CreatesWhenNotExists(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	ensureBreakglassAdmin(context.Background(), store, logger)

	user, err := store.GetUser(context.Background(), "breakglass-admin")
	if err != nil {
		t.Fatal("expected breakglass-admin user to be created, got error:", err)
	}
	if user == nil {
		t.Fatal("expected breakglass-admin user to be created, got nil")
	}
}

func TestEnsureBreakglassAdmin_SkipsWhenAlreadyExists(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	// Pre-create the user
	existing := &models.InternalUser{
		UserID:       "breakglass-admin",
		Email:        "admin@vire.local",
		Name:         "Break-Glass Admin",
		PasswordHash: "existing-hash",
		Provider:     "system",
		Role:         models.RoleAdmin,
	}
	store.users["breakglass-admin"] = existing

	ensureBreakglassAdmin(context.Background(), store, logger)

	user, _ := store.GetUser(context.Background(), "breakglass-admin")
	if user.PasswordHash != "existing-hash" {
		t.Fatal("expected existing user to remain unchanged, but password hash was modified")
	}
}

func TestEnsureBreakglassAdmin_PasswordWorksWithBcrypt(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	password := ensureBreakglassAdmin(context.Background(), store, logger)

	user, err := store.GetUser(context.Background(), "breakglass-admin")
	if err != nil {
		t.Fatal("expected breakglass-admin user to be created, got error:", err)
	}

	// The cleartext password should verify against the stored bcrypt hash
	passwordBytes := []byte(password)
	if len(passwordBytes) > 72 {
		passwordBytes = passwordBytes[:72]
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), passwordBytes); err != nil {
		t.Fatal("generated password does not match stored bcrypt hash:", err)
	}
}

func TestEnsureBreakglassAdmin_UserHasCorrectFields(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	ensureBreakglassAdmin(context.Background(), store, logger)

	user, err := store.GetUser(context.Background(), "breakglass-admin")
	if err != nil {
		t.Fatal("expected breakglass-admin user to be created, got error:", err)
	}

	if user.UserID != "breakglass-admin" {
		t.Errorf("UserID = %q, want %q", user.UserID, "breakglass-admin")
	}
	if user.Email != "admin@vire.local" {
		t.Errorf("Email = %q, want %q", user.Email, "admin@vire.local")
	}
	if user.Name != "Break-Glass Admin" {
		t.Errorf("Name = %q, want %q", user.Name, "Break-Glass Admin")
	}
	if user.Provider != "system" {
		t.Errorf("Provider = %q, want %q", user.Provider, "system")
	}
	if user.Role != models.RoleAdmin {
		t.Errorf("Role = %q, want %q", user.Role, models.RoleAdmin)
	}
	if user.PasswordHash == "" {
		t.Error("PasswordHash should not be empty")
	}
	if user.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestEnsureBreakglassAdmin_ReturnsEmptyStringWhenExists(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	// Pre-create the user
	store.users["breakglass-admin"] = &models.InternalUser{
		UserID:       "breakglass-admin",
		Email:        "admin@vire.local",
		PasswordHash: "existing-hash",
		Provider:     "system",
		Role:         models.RoleAdmin,
	}

	password := ensureBreakglassAdmin(context.Background(), store, logger)
	if password != "" {
		t.Errorf("expected empty password when user already exists, got %q", password)
	}
}
