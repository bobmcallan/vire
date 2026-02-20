package surrealdb

import (
	"strings"
	"testing"
)

// ============================================================================
// 1. Record ID collision — triple-component separator ambiguity
// ============================================================================

func TestStress_recordID_SeparatorCollision(t *testing.T) {
	// BUG: recordID uses "_" as separator between userID, subject, and key.
	// This is worse than kvID because three components are joined, creating
	// more collision possibilities.
	//
	// Example:
	//   recordID("alice", "port_folio", "key") == recordID("alice_port", "folio", "key")
	//   Both produce "alice_port_folio_key"
	//
	// This can cause cross-user data leakage: user A reading user B's records.

	collisions := []struct {
		name    string
		userA   string
		subA    string
		keyA    string
		userB   string
		subB    string
		keyB    string
		collide bool
	}{
		{
			name:  "user_subject_boundary",
			userA: "alice_bob", subA: "portfolio", keyA: "main",
			userB: "alice", subB: "bob_portfolio", keyB: "main",
			collide: true,
		},
		{
			name:  "subject_key_boundary",
			userA: "alice", subA: "port", keyA: "folio_main",
			userB: "alice", subB: "port_folio", keyB: "main",
			collide: true,
		},
		{
			name:  "all_three_shift",
			userA: "a_b", subA: "c", keyA: "d",
			userB: "a", subB: "b_c", keyB: "d",
			collide: true,
		},
		{
			name:  "triple_shift",
			userA: "a_b_c", subA: "d", keyA: "e",
			userB: "a", subB: "b", keyB: "c_d_e",
			collide: true,
		},
		{
			name:  "no_collision",
			userA: "alice", subA: "portfolio", keyA: "main",
			userB: "bob", subB: "portfolio", keyB: "main",
			collide: false,
		},
	}

	for _, tc := range collisions {
		t.Run(tc.name, func(t *testing.T) {
			idA := recordID(tc.userA, tc.subA, tc.keyA)
			idB := recordID(tc.userB, tc.subB, tc.keyB)

			if tc.collide && idA != idB {
				t.Errorf("expected collision: recordID(%q,%q,%q)=%q != recordID(%q,%q,%q)=%q",
					tc.userA, tc.subA, tc.keyA, idA, tc.userB, tc.subB, tc.keyB, idB)
			}
			if tc.collide && idA == idB {
				t.Logf("BUG CONFIRMED: recordID(%q,%q,%q) == recordID(%q,%q,%q) == %q — cross-user data collision",
					tc.userA, tc.subA, tc.keyA, tc.userB, tc.subB, tc.keyB, idA)
			}
			if !tc.collide && idA == idB {
				t.Errorf("unexpected collision: got %q for both", idA)
			}
		})
	}
}

func TestStress_recordID_CollisionCount(t *testing.T) {
	// Demonstrate the scale of the collision problem.
	// For a given composite ID "a_b_c_d", count how many distinct
	// (userID, subject, key) triples map to it.

	target := "a_b_c_d"
	type triple struct {
		user    string
		subject string
		key     string
	}
	var colliding []triple

	// Enumerate all ways to split "a_b_c_d" into 3 parts using "_" as separator
	parts := strings.Split(target, "_")
	// parts = ["a", "b", "c", "d"]
	// We need to find all ways to partition into 3 groups
	n := len(parts)
	for i := 1; i < n; i++ {
		for j := i + 1; j < n; j++ {
			user := strings.Join(parts[:i], "_")
			subject := strings.Join(parts[i:j], "_")
			key := strings.Join(parts[j:], "_")
			id := recordID(user, subject, key)
			if id == target {
				colliding = append(colliding, triple{user, subject, key})
			}
		}
	}

	if len(colliding) > 1 {
		t.Logf("BUG: %d distinct (user, subject, key) triples all map to record ID %q:", len(colliding), target)
		for _, c := range colliding {
			t.Logf("  recordID(%q, %q, %q)", c.user, c.subject, c.key)
		}
	}
}

// ============================================================================
// 2. Record ID hostile inputs
// ============================================================================

func TestStress_recordID_HostileInputs(t *testing.T) {
	hostileInputs := []struct {
		name    string
		userID  string
		subject string
		key     string
	}{
		{"all_empty", "", "", ""},
		{"empty_user", "", "portfolio", "main"},
		{"empty_subject", "alice", "", "main"},
		{"empty_key", "alice", "portfolio", ""},
		{"null_bytes", "user\x00", "sub\x00", "key\x00"},
		{"sql_in_user", "'; DROP TABLE user_data; --", "portfolio", "main"},
		{"sql_in_subject", "alice", "'; DELETE user_data; --", "main"},
		{"sql_in_key", "alice", "portfolio", "'; UPDATE user_data SET value='hacked'; --"},
		{"surrealql_record_ref", "user_data:injected", "portfolio", "main"},
		{"very_long", strings.Repeat("x", 50000), strings.Repeat("y", 50000), strings.Repeat("z", 50000)},
		{"unicode_bidi", "\u202Euser", "\u202Esubject", "\u202Ekey"},
		{"crlf_injection", "user\r\n", "subject\r\n", "key\r\n"},
	}

	for _, tc := range hostileInputs {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			id := recordID(tc.userID, tc.subject, tc.key)
			expected := tc.userID + "_" + tc.subject + "_" + tc.key
			if id != expected {
				t.Errorf("recordID modified input: got %q, want %q", id, expected)
			}
		})
	}
}

func TestStress_recordID_AllEmpty_ProducesSameID(t *testing.T) {
	// recordID("", "", "") == "__"
	// Any call with all-empty inputs maps to the same record.
	// This means if two different callers both pass empty strings,
	// they silently share/overwrite the same record.

	id := recordID("", "", "")
	if id != "__" {
		t.Errorf("expected '__', got %q", id)
	}
	t.Log("FINDING: recordID('','','') produces '__' — all empty-input calls collide")
}

// ============================================================================
// 3. Put nil pointer dereference
// ============================================================================

func TestStress_Put_NilRecordDocumented(t *testing.T) {
	// UserStore.Put (userstore.go:41) accesses record.UserID, record.Subject,
	// record.Key on line 42:
	//   id := recordID(record.UserID, record.Subject, record.Key)
	//
	// If record is nil, this panics.
	//
	// FINDING: Put has no nil guard. A nil *models.UserRecord causes panic.

	t.Log("BUG: UserStore.Put will panic on nil record — no nil check before accessing record.UserID at userstore.go:42")
}

// ============================================================================
// 4. Query — SQL string concatenation for ORDER BY
// ============================================================================

func TestStress_Query_OrderByInjection(t *testing.T) {
	// UserStore.Query (userstore.go:88-99) uses string comparison on opts.OrderBy:
	//   if opts.OrderBy == "datetime_asc" {
	//       sql += " ORDER BY datetime ASC"
	//   } else {
	//       sql += " ORDER BY datetime DESC"
	//   }
	//
	// This is SAFE from injection because OrderBy is compared against a fixed
	// string, not interpolated into the query. Any value other than
	// "datetime_asc" falls through to DESC. The value is never placed into SQL.
	//
	// Similarly, opts.Limit is formatted with %d (integer), which is safe.

	t.Log("VERIFIED: Query OrderBy is safe — value is compared, not interpolated into SQL")
}

func TestStress_Query_LimitEdgeCases(t *testing.T) {
	// UserStore.Query (userstore.go:97-99) uses Sprintf with %d for Limit:
	//   sql += fmt.Sprintf(" LIMIT %d", opts.Limit)
	//
	// Since Limit is an int, negative values produce "LIMIT -1" which
	// SurrealDB may interpret differently. Zero is skipped (no LIMIT clause).
	//
	// FINDING: Negative Limit values produce valid but unexpected SurrealQL.
	// SurrealDB may error or return all results. No input validation.

	t.Log("FINDING: Query with negative Limit produces 'LIMIT -N' in SurrealQL — behavior depends on SurrealDB's handling of negative LIMIT")
}

// ============================================================================
// 5. DeleteBySubject — no user scoping
// ============================================================================

func TestStress_DeleteBySubject_NoUserScope(t *testing.T) {
	// DeleteBySubject (userstore.go:121-137) uses:
	//   "DELETE user_data WHERE subject = $subject"
	//
	// FINDING: This deletes ALL records with the given subject across ALL users.
	// This is by design for purge operations, but it means any call to
	// DeleteBySubject("portfolio") wipes every user's portfolio data.
	//
	// If this is ever exposed through an API endpoint without proper
	// authorization, it would be a critical data destruction vulnerability.
	//
	// Current usage: Only called from Manager.PurgeDerivedData and
	// Manager.PurgeReports, which are admin operations. But the method
	// is public on UserStore, so any code with a UserStore reference can
	// call it.

	t.Log("FINDING: DeleteBySubject deletes across ALL users — safe as admin-only operation but dangerous if exposed through user-facing API")
}

// ============================================================================
// 6. UserStore.Close() is a no-op
// ============================================================================

func TestStress_UserStoreClose_NoOp(t *testing.T) {
	store := &UserStore{}
	err := store.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
	t.Log("NOTE: UserStore.Close() is a no-op — DB connection managed by Manager")
}

// ============================================================================
// 7. Recommended fix for separator collision
// ============================================================================

func TestStress_RecommendedFix_SeparatorCollision(t *testing.T) {
	// The fix for the separator collision is to use a separator that cannot
	// appear in the component values, or to use a length-prefixed format.
	//
	// Option A: Use a non-printable separator (e.g., \x00)
	//   func kvID(userID, key string) string { return userID + "\x00" + key }
	//   Pro: Simple. Con: Null bytes may cause issues in some DB drivers.
	//
	// Option B: Use SurrealDB's array-based record IDs
	//   surrealmodels.NewRecordID("user_kv", []interface{}{userID, key})
	//   Pro: No collision possible. Con: Requires SDK support.
	//
	// Option C: Use colon separator with escaping
	//   func kvID(userID, key string) string {
	//       return url.PathEscape(userID) + ":" + url.PathEscape(key)
	//   }
	//   Pro: Readable, no collision. Con: Longer IDs.
	//
	// Option D: Use hash-based IDs
	//   func kvID(userID, key string) string {
	//       h := sha256.Sum256([]byte(userID + "\x00" + key))
	//       return hex.EncodeToString(h[:])
	//   }
	//   Pro: Fixed length, no collision. Con: Not human-readable for debugging.

	t.Log("RECOMMENDATION: Replace '_' separator with array-based SurrealDB record IDs or URL-escaped components to prevent collision")
}
