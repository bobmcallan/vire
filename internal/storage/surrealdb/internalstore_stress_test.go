package surrealdb

import (
	"strings"
	"testing"
)

// ============================================================================
// 1. Record ID collision — kvID separator ambiguity
// ============================================================================

func TestStress_kvID_SeparatorCollision(t *testing.T) {
	// BUG: kvID uses "_" as separator between userID and key.
	// If userID or key contains "_", different (userID, key) pairs
	// can produce the same composite ID.
	//
	// Example:
	//   kvID("alice_bob", "setting") == kvID("alice", "bob_setting")
	//   Both produce "alice_bob_setting"
	//
	// This means one user's KV entry can overwrite another user's entry,
	// causing data corruption and a potential privilege escalation vector.

	collisions := []struct {
		name    string
		userA   string
		keyA    string
		userB   string
		keyB    string
		collide bool
	}{
		{
			name:  "underscore_in_userID",
			userA: "alice_bob", keyA: "setting",
			userB: "alice", keyB: "bob_setting",
			collide: true,
		},
		{
			name:  "underscore_in_key",
			userA: "user", keyA: "a_b",
			userB: "user_a", keyB: "b",
			collide: true,
		},
		{
			name:  "triple_underscore",
			userA: "a_b_c", keyA: "d",
			userB: "a", keyB: "b_c_d",
			collide: true,
		},
		{
			name:  "no_collision",
			userA: "alice", keyA: "theme",
			userB: "bob", keyB: "theme",
			collide: false,
		},
	}

	for _, tc := range collisions {
		t.Run(tc.name, func(t *testing.T) {
			idA := kvID(tc.userA, tc.keyA)
			idB := kvID(tc.userB, tc.keyB)

			if tc.collide && idA != idB {
				t.Errorf("expected collision: kvID(%q,%q)=%q != kvID(%q,%q)=%q",
					tc.userA, tc.keyA, idA, tc.userB, tc.keyB, idB)
			}
			if tc.collide && idA == idB {
				t.Logf("BUG CONFIRMED: kvID(%q,%q) == kvID(%q,%q) == %q — record ID collision",
					tc.userA, tc.keyA, tc.userB, tc.keyB, idA)
			}
			if !tc.collide && idA == idB {
				t.Errorf("unexpected collision: kvID(%q,%q) == kvID(%q,%q) == %q",
					tc.userA, tc.keyA, tc.userB, tc.keyB, idA)
			}
		})
	}
}

func TestStress_kvID_HostileInputs(t *testing.T) {
	// Verify kvID doesn't panic on hostile inputs
	hostileInputs := []struct {
		name   string
		userID string
		key    string
	}{
		{"empty_both", "", ""},
		{"empty_userID", "", "key"},
		{"empty_key", "user", ""},
		{"null_bytes", "user\x00evil", "key\x00drop"},
		{"sql_injection_userID", "'; DROP TABLE user_kv; --", "key"},
		{"sql_injection_key", "user", "'; DELETE FROM user_kv; --"},
		{"very_long_userID", strings.Repeat("A", 100000), "key"},
		{"very_long_key", "user", strings.Repeat("B", 100000)},
		{"unicode_userID", "\u202E\u0041\u0042\u0043", "key"}, // RTL override
		{"newlines", "user\ninjected", "key\rinjected"},
		{"tabs", "user\tinjected", "key\tinjected"},
		{"backslash", "user\\path", "key\\path"},
		{"single_underscore_userID", "_", "_"},
		{"only_underscores", "___", "___"},
	}

	for _, tc := range hostileInputs {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			id := kvID(tc.userID, tc.key)
			if id == "" && (tc.userID != "" || tc.key != "") {
				t.Errorf("kvID returned empty for non-empty inputs")
			}
			// Verify the separator is present (even for empty inputs)
			if !strings.Contains(id, "_") {
				t.Errorf("kvID(%q, %q) = %q — missing separator", tc.userID, tc.key, id)
			}
		})
	}
}

// ============================================================================
// 2. SQL injection via record IDs
// ============================================================================

func TestStress_kvID_SQLInjectionPayloads(t *testing.T) {
	// The kvID result is passed to SurrealDB's type::record() function
	// via a $id parameter. SurrealDB's Go SDK should parameterize this,
	// but the composite ID string itself could contain SurrealQL-significant
	// characters that might escape the record ID context.
	//
	// The key question: does NewRecordID("user_kv", someString) treat
	// someString as a literal record ID, or can special chars break out?

	injectionPayloads := []string{
		"'; DROP TABLE user_kv; --",
		"user_kv:injected",
		"`) REMOVE TABLE user_kv; --",
		"test\"; DELETE user_kv; --",
		"test' OR '1'='1",
		"test); DELETE user_kv WHERE true; --",
		"../../../etc/passwd",
		"user_kv:⟨injected⟩",
		"test`; REMOVE DATABASE test; `",
	}

	for _, payload := range injectionPayloads {
		t.Run(payload[:min(len(payload), 30)], func(t *testing.T) {
			// kvID should produce the concatenation without any sanitization
			// (it's the caller's job to ensure the SDK handles this safely)
			id := kvID(payload, "key")
			expected := payload + "_key"
			if id != expected {
				t.Errorf("kvID modified the input: got %q, want %q", id, expected)
			}

			// FINDING: kvID does NO sanitization. It relies entirely on the
			// SurrealDB Go SDK's NewRecordID and $id parameter binding to
			// prevent injection. This is acceptable IF the SDK properly
			// escapes the ID value. If it doesn't, these payloads would
			// cause SQL injection.
			t.Logf("FINDING: kvID passes unsanitized input to record ID: %q", id)
		})
	}
}

// ============================================================================
// 3. Nil pointer dereference risk
// ============================================================================

func TestStress_SaveUser_NilUserDocumented(t *testing.T) {
	// SaveUser (internalstore.go:37) dereferences user.UserID on line 39:
	//   vars := map[string]any{"id": user.UserID, "user": user}
	//
	// If user is nil, this will panic with a nil pointer dereference.
	// There is no nil guard.
	//
	// FINDING: SaveUser will panic on nil *models.InternalUser input.
	// This is a runtime crash risk if any caller passes nil.
	// The function should check for nil and return an error.

	t.Log("BUG: SaveUser has no nil guard — calling SaveUser(ctx, nil) will panic with nil pointer dereference on user.UserID access at internalstore.go:39")
}

func TestStress_SetUserKV_EmptyInputs(t *testing.T) {
	// SetUserKV creates a UserKeyValue with whatever inputs are given.
	// Empty userID + empty key produces kvID("", "") == "_"
	// which is a valid but bizarre record ID: user_kv:⟨_⟩
	//
	// FINDING: No input validation on SetUserKV. Empty strings produce
	// a record with ID "_" which could collide with other empty-input calls.

	id := kvID("", "")
	if id != "_" {
		t.Errorf("expected kvID('','') == '_', got %q", id)
	}
	t.Log("FINDING: kvID('','') produces '_' — all empty-input calls map to the same record, causing silent data overwrites")
}

// ============================================================================
// 4. ListUserKV — WHERE clause relies on stored user_id field
// ============================================================================

func TestStress_ListUserKV_FieldNameMismatch(t *testing.T) {
	// ListUserKV (internalstore.go:126) queries:
	//   "SELECT * FROM user_kv WHERE user_id = $user_id"
	//
	// This relies on the stored document having a "user_id" JSON field.
	// The UserKeyValue struct has `json:"user_id"` tag — but SurrealDB
	// stores the Go struct as CONTENT, so the field names come from the
	// JSON tags. This should work correctly.
	//
	// However, if the CONTENT stored by UPSERT includes the full struct
	// (including the SurrealDB record ID), the WHERE clause needs to match
	// the exact field name. If SurrealDB renames the field or adds metadata,
	// this could silently return no results.
	//
	// This is NOT a bug — just a fragility note. If the JSON tag on
	// UserKeyValue.UserID ever changes, ListUserKV will silently break.

	t.Log("NOTE: ListUserKV WHERE clause depends on json tag 'user_id' matching exactly — changing the tag breaks listing without compile errors")
}

// ============================================================================
// 5. GetSystemKV — error suppression
// ============================================================================

func TestStress_GetSystemKV_ErrorSuppression(t *testing.T) {
	// GetSystemKV (internalstore.go:152) has:
	//   if err != nil || kv == nil {
	//       return "", errors.New("system KV not found")
	//   }
	//
	// FINDING: This suppresses the actual error from SurrealDB. If the
	// database is down or returns a permission error, the caller gets
	// "system KV not found" instead of the real error. This makes
	// debugging connection issues harder.
	//
	// Recommendation: Return a different error for err != nil vs kv == nil:
	//   if err != nil { return "", fmt.Errorf("failed to get system KV: %w", err) }
	//   if kv == nil { return "", errors.New("system KV not found") }

	t.Log("FINDING: GetSystemKV suppresses actual DB errors — connection failures appear as 'system KV not found' at internalstore.go:152")
}

// ============================================================================
// 6. InternalStore.Close() is a no-op
// ============================================================================

func TestStress_InternalStoreClose_NoOp(t *testing.T) {
	// InternalStore.Close() (line 180) returns nil without doing anything.
	// The actual DB connection is closed by Manager.Close().
	// This is fine architecturally, but means calling InternalStore.Close()
	// gives a false sense of cleanup.
	//
	// If InternalStore is ever used independently of Manager, the DB
	// connection would leak.

	store := &InternalStore{}
	err := store.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
	t.Log("NOTE: InternalStore.Close() is a no-op — DB connection managed by Manager")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
