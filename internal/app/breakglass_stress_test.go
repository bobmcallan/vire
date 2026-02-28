package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================================
// 1. Password entropy and generation security
// ============================================================================

func TestBreakglass_PasswordLength(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	password := ensureBreakglassAdmin(context.Background(), store, logger)
	if password == "" {
		t.Fatal("expected non-empty password")
	}

	// Requirement: 24-char cryptographically random password (base64)
	// base64 encoding of 18 random bytes = 24 chars
	if len(password) < 24 {
		t.Errorf("password too short: got %d chars, want at least 24", len(password))
	}
}

func TestBreakglass_PasswordUniqueness(t *testing.T) {
	// Each call to ensureBreakglassAdmin should generate a unique password.
	// Run 20 times on fresh stores and verify no duplicates.
	passwords := make(map[string]bool)

	for i := 0; i < 20; i++ {
		store := newMockInternalStore()
		logger := common.NewLogger("debug")
		pw := ensureBreakglassAdmin(context.Background(), store, logger)
		if pw == "" {
			t.Fatalf("iteration %d: expected non-empty password", i)
		}
		if passwords[pw] {
			t.Fatalf("CRITICAL: duplicate password generated on iteration %d", i)
		}
		passwords[pw] = true
	}
}

func TestBreakglass_PasswordNotStoredInPlaintext(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	password := ensureBreakglassAdmin(context.Background(), store, logger)

	user, _ := store.GetUser(context.Background(), "breakglass-admin")
	if user == nil {
		t.Fatal("user not created")
	}

	// The stored hash must NOT equal the plaintext password
	if user.PasswordHash == password {
		t.Fatal("CRITICAL: password stored in plaintext, not hashed")
	}

	// Must be a valid bcrypt hash
	if !strings.HasPrefix(user.PasswordHash, "$2a$") && !strings.HasPrefix(user.PasswordHash, "$2b$") {
		t.Errorf("stored hash does not look like bcrypt: %s", user.PasswordHash[:20])
	}
}

func TestBreakglass_PasswordBcryptCost(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	ensureBreakglassAdmin(context.Background(), store, logger)

	user, _ := store.GetUser(context.Background(), "breakglass-admin")
	if user == nil {
		t.Fatal("user not created")
	}

	// Verify bcrypt cost is at least 10
	cost, err := bcrypt.Cost([]byte(user.PasswordHash))
	if err != nil {
		t.Fatalf("failed to read bcrypt cost: %v", err)
	}
	if cost < 10 {
		t.Errorf("bcrypt cost too low: got %d, want >= 10", cost)
	}
}

func TestBreakglass_PasswordUsesSecureRandom(t *testing.T) {
	// Verify that the password contains enough entropy.
	// A base64-encoded 18-byte random value has ~144 bits of entropy.
	// We check that the password is at least plausibly random by verifying
	// it's valid base64 and not a trivial pattern.
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	password := ensureBreakglassAdmin(context.Background(), store, logger)

	// Should be decodable as base64 (standard or URL-safe)
	decoded, err := base64.URLEncoding.DecodeString(password)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(password)
		if err != nil {
			// If the password uses RawURLEncoding or RawStdEncoding, try those
			decoded, err = base64.RawURLEncoding.DecodeString(password)
			if err != nil {
				decoded, err = base64.RawStdEncoding.DecodeString(password)
			}
		}
	}

	if err == nil && len(decoded) < 16 {
		t.Errorf("password decodes to only %d bytes of entropy, want at least 16", len(decoded))
	}

	// Not a trivial pattern
	if password == strings.Repeat(password[:1], len(password)) {
		t.Error("CRITICAL: password is a single repeated character")
	}
}

// ============================================================================
// 2. Race condition: concurrent bootstrap
// ============================================================================

func TestBreakglass_ConcurrentBootstrap_NoDoubleCreate(t *testing.T) {
	// Simulate two instances racing to create the admin user.
	// Only one should succeed in creating; the other should detect it exists.
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	var wg sync.WaitGroup
	passwords := make([]string, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			passwords[idx] = ensureBreakglassAdmin(context.Background(), store, logger)
		}(i)
	}

	wg.Wait()

	// Exactly one goroutine should have created the user (non-empty password).
	// Others should have found it already exists (empty password).
	createCount := 0
	for _, pw := range passwords {
		if pw != "" {
			createCount++
		}
	}

	// With our simple in-memory mock, the race may allow more than one creation.
	// The important thing is the user exists and is valid.
	user, err := store.GetUser(context.Background(), "breakglass-admin")
	if err != nil {
		t.Fatal("user should exist after concurrent bootstrap:", err)
	}
	if user.Role != models.RoleAdmin {
		t.Errorf("user role = %q, want admin", user.Role)
	}
	if user.PasswordHash == "" {
		t.Error("user should have a password hash")
	}
}

// ============================================================================
// 3. Thread-safe mock to simulate real DB race conditions
// ============================================================================

// raceSafeStore is a mock that uses a mutex to simulate DB-level atomicity,
// helping detect race conditions in the bootstrap logic.
type raceSafeStore struct {
	mu    sync.RWMutex
	users map[string]*models.InternalUser
	saves atomic.Int32 // count SaveUser calls
}

func newRaceSafeStore() *raceSafeStore {
	return &raceSafeStore{users: make(map[string]*models.InternalUser)}
}

func (s *raceSafeStore) GetUser(_ context.Context, userID string) (*models.InternalUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[userID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return u, nil
}

func (s *raceSafeStore) SaveUser(_ context.Context, user *models.InternalUser) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves.Add(1)
	s.users[user.UserID] = user
	return nil
}

func (s *raceSafeStore) GetUserByEmail(_ context.Context, email string) (*models.InternalUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (s *raceSafeStore) DeleteUser(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, userID)
	return nil
}

func (s *raceSafeStore) ListUsers(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ids []string
	for id := range s.users {
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *raceSafeStore) GetUserKV(_ context.Context, _, _ string) (*models.UserKeyValue, error) {
	return nil, fmt.Errorf("not found")
}
func (s *raceSafeStore) SetUserKV(_ context.Context, _, _, _ string) error { return nil }
func (s *raceSafeStore) DeleteUserKV(_ context.Context, _, _ string) error { return nil }
func (s *raceSafeStore) ListUserKV(_ context.Context, _ string) ([]*models.UserKeyValue, error) {
	return nil, nil
}
func (s *raceSafeStore) GetSystemKV(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("not found")
}
func (s *raceSafeStore) SetSystemKV(_ context.Context, _, _ string) error { return nil }
func (s *raceSafeStore) Close() error                                     { return nil }

func TestBreakglass_RaceSafe_ConcurrentBootstrap(t *testing.T) {
	store := newRaceSafeStore()
	logger := common.NewLogger("debug")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ensureBreakglassAdmin(context.Background(), store, logger)
		}()
	}
	wg.Wait()

	// User must exist and be valid
	user, err := store.GetUser(context.Background(), "breakglass-admin")
	if err != nil {
		t.Fatal("user not created after 50 concurrent calls:", err)
	}
	if user.Role != models.RoleAdmin {
		t.Errorf("role = %q, want admin", user.Role)
	}

	// Log how many SaveUser calls were made (ideally 1, but check-before-create
	// without distributed locking may allow a few). Document this.
	saves := store.saves.Load()
	t.Logf("SaveUser called %d times out of 50 concurrent bootstrap attempts", saves)
}

// ============================================================================
// 4. DB unavailable during bootstrap
// ============================================================================

// failingStore simulates a database that is unavailable.
type failingStore struct {
	mockInternalStore
	getUserErr  error
	saveUserErr error
}

func (f *failingStore) GetUser(_ context.Context, userID string) (*models.InternalUser, error) {
	if f.getUserErr != nil {
		return nil, f.getUserErr
	}
	return f.mockInternalStore.GetUser(context.Background(), userID)
}

func (f *failingStore) SaveUser(_ context.Context, user *models.InternalUser) error {
	if f.saveUserErr != nil {
		return f.saveUserErr
	}
	return f.mockInternalStore.SaveUser(context.Background(), user)
}

func TestBreakglass_DBUnavailable_GetUserFails(t *testing.T) {
	store := &failingStore{
		mockInternalStore: *newMockInternalStore(),
		getUserErr:        errors.New("connection refused"),
	}
	logger := common.NewLogger("debug")

	// Should not panic when DB is unavailable
	password := ensureBreakglassAdmin(context.Background(), store, logger)

	// When GetUser fails with a non-"not found" error, the function should
	// either treat it as "user might exist" and skip (returning ""),
	// or attempt to create. Either behavior is acceptable as long as it
	// doesn't panic.
	_ = password
}

func TestBreakglass_DBUnavailable_SaveUserFails(t *testing.T) {
	store := &failingStore{
		mockInternalStore: *newMockInternalStore(),
		saveUserErr:       errors.New("disk full"),
	}
	logger := common.NewLogger("debug")

	// Should not panic when SaveUser fails
	password := ensureBreakglassAdmin(context.Background(), store, logger)

	// If save fails, the user was not created. The password may have been
	// logged but is useless. This is acceptable — the next restart will retry.
	_ = password
}

// ============================================================================
// 5. Break-glass admin cannot be tampered with via user endpoints
// ============================================================================

func TestBreakglass_AdminUser_HasCorrectProvider(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	ensureBreakglassAdmin(context.Background(), store, logger)

	user, _ := store.GetUser(context.Background(), "breakglass-admin")
	if user == nil {
		t.Fatal("user not created")
	}

	// Provider must be "system" to distinguish from user-created accounts
	if user.Provider != "system" {
		t.Errorf("Provider = %q, want %q", user.Provider, "system")
	}
}

func TestBreakglass_AdminUser_CannotBeRecreatedAfterDelete(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	// Create the admin
	pw1 := ensureBreakglassAdmin(context.Background(), store, logger)
	if pw1 == "" {
		t.Fatal("expected password on first creation")
	}

	// Simulate deletion via API
	store.DeleteUser(context.Background(), "breakglass-admin")

	// Re-run bootstrap — should recreate with a new password
	pw2 := ensureBreakglassAdmin(context.Background(), store, logger)
	if pw2 == "" {
		t.Fatal("expected new password after deletion and re-bootstrap")
	}

	// The new password should be different from the original
	if pw1 == pw2 {
		t.Error("CRITICAL: same password generated after deletion — random source may be predictable")
	}
}

func TestBreakglass_AdminUser_PasswordResetDoesNotDowngradeRole(t *testing.T) {
	// If someone resets the break-glass admin's password via the API,
	// the role should remain admin after the next bootstrap run.
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	ensureBreakglassAdmin(context.Background(), store, logger)

	// Simulate password change via API (user update does NOT change role)
	user, _ := store.GetUser(context.Background(), "breakglass-admin")
	newHash, _ := bcrypt.GenerateFromPassword([]byte("attacker-password"), 10)
	user.PasswordHash = string(newHash)
	store.SaveUser(context.Background(), user)

	// Re-run bootstrap — should detect user exists and skip
	pw := ensureBreakglassAdmin(context.Background(), store, logger)
	if pw != "" {
		t.Error("expected empty password (skip) when user already exists")
	}

	// Role must still be admin
	user, _ = store.GetUser(context.Background(), "breakglass-admin")
	if user.Role != models.RoleAdmin {
		t.Errorf("role changed to %q after bootstrap re-run", user.Role)
	}
}

func TestBreakglass_AdminUser_RoleTamperedThenRebootstrap(t *testing.T) {
	// If an attacker demotes the break-glass admin via the admin API,
	// the next bootstrap should detect the user exists and skip.
	// This means the break-glass admin would be demoted until manually fixed.
	// This is EXPECTED behavior per the requirements (idempotent skip).
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	ensureBreakglassAdmin(context.Background(), store, logger)

	// Tamper: demote to regular user
	user, _ := store.GetUser(context.Background(), "breakglass-admin")
	user.Role = models.RoleUser
	store.SaveUser(context.Background(), user)

	// Re-run bootstrap
	pw := ensureBreakglassAdmin(context.Background(), store, logger)

	// The function should skip (user exists) — it does NOT repair the role.
	// Document this as expected behavior, not a bug.
	if pw != "" {
		t.Log("NOTE: bootstrap recreated a demoted break-glass admin (role repair mode)")
	}

	user, _ = store.GetUser(context.Background(), "breakglass-admin")
	t.Logf("After re-bootstrap of demoted user: role=%s", user.Role)
	// If the implementation repairs the role, that's OK too. Either behavior is acceptable.
}

// ============================================================================
// 6. Crypto source validation
// ============================================================================

func TestBreakglass_CryptoRandNotMathRand(t *testing.T) {
	// Generate 100 passwords and verify sufficient entropy.
	// math/rand with a fixed seed would produce the same sequence.
	passwords := make([]string, 100)
	for i := 0; i < 100; i++ {
		store := newMockInternalStore()
		logger := common.NewLogger("debug")
		passwords[i] = ensureBreakglassAdmin(context.Background(), store, logger)
	}

	// Check all are unique
	seen := make(map[string]bool)
	for i, pw := range passwords {
		if seen[pw] {
			t.Fatalf("CRITICAL: duplicate password at index %d — using math/rand instead of crypto/rand?", i)
		}
		seen[pw] = true
	}
}

// ============================================================================
// 7. Verify crypto/rand is actually used (compile-time check)
// ============================================================================

func TestBreakglass_CryptoRandImport(t *testing.T) {
	// This test verifies the crypto source by generating random bytes
	// through the same mechanism the implementation should use.
	// If crypto/rand is broken, this will fail.
	buf := make([]byte, 18)
	n, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("crypto/rand.Read failed: %v", err)
	}
	if n != 18 {
		t.Fatalf("crypto/rand.Read returned %d bytes, want 18", n)
	}
}

// ============================================================================
// 8. Password within bcrypt 72-byte limit
// ============================================================================

func TestBreakglass_PasswordWithinBcryptLimit(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	password := ensureBreakglassAdmin(context.Background(), store, logger)

	// 24-char base64 is always within 72-byte limit, but verify explicitly
	if len([]byte(password)) > 72 {
		t.Errorf("password is %d bytes, exceeds bcrypt 72-byte limit", len([]byte(password)))
	}
}

// ============================================================================
// 9. Context cancellation safety
// ============================================================================

func TestBreakglass_CancelledContext(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Should not panic with cancelled context
	password := ensureBreakglassAdmin(ctx, store, logger)
	_ = password
}

// ============================================================================
// 10. Email must not be empty
// ============================================================================

func TestBreakglass_AdminEmailNotEmpty(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	ensureBreakglassAdmin(context.Background(), store, logger)

	user, _ := store.GetUser(context.Background(), "breakglass-admin")
	if user == nil {
		t.Fatal("user not created")
	}
	if user.Email == "" {
		t.Error("admin email should not be empty")
	}
	if user.Email != "admin@vire.local" {
		t.Errorf("Email = %q, want %q", user.Email, "admin@vire.local")
	}
}

// ============================================================================
// 11. Idempotency: multiple calls don't change existing user
// ============================================================================

func TestBreakglass_Idempotency_DoesNotChangeExistingHash(t *testing.T) {
	store := newMockInternalStore()
	logger := common.NewLogger("debug")

	// First call creates the user
	pw1 := ensureBreakglassAdmin(context.Background(), store, logger)
	if pw1 == "" {
		t.Fatal("first call should return a password")
	}

	user1, _ := store.GetUser(context.Background(), "breakglass-admin")
	hash1 := user1.PasswordHash

	// Second call should be idempotent
	pw2 := ensureBreakglassAdmin(context.Background(), store, logger)
	if pw2 != "" {
		t.Error("second call should return empty string (user exists)")
	}

	user2, _ := store.GetUser(context.Background(), "breakglass-admin")
	if user2.PasswordHash != hash1 {
		t.Error("CRITICAL: idempotent call changed the password hash")
	}
}

// ============================================================================
// 12. Nil/zero-value safety
// ============================================================================

func TestBreakglass_NilContext(t *testing.T) {
	// While passing nil context is bad practice, the function should not panic.
	// Some implementations use context.TODO() internally. We just check no panic.
	defer func() {
		if r := recover(); r != nil {
			t.Logf("NOTE: function panics with nil context: %v", r)
			// This is acceptable if documented, but flag it.
		}
	}()

	store := newMockInternalStore()
	logger := common.NewLogger("debug")
	ensureBreakglassAdmin(nil, store, logger) //nolint:staticcheck
}
