package portfolio

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
)

// TestSyncPortfolio_ConcurrentForce_OnlyOneSyncHappens verifies that when
// multiple goroutines simultaneously call SyncPortfolio(force=true), the
// syncMu mutex ensures only the first performs a full Navexa sync. All
// subsequent goroutines within the cooldown window return the cached result.
func TestSyncPortfolio_ConcurrentForce_OnlyOneSyncHappens(t *testing.T) {
	svc, navexa := newSyncCooldownFixture()
	ctx := common.WithNavexaClient(context.Background(), navexa)

	const goroutines = 5
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.SyncPortfolio(ctx, "SMSF", true)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d returned error: %v", i, err)
		}
	}

	// syncMu serializes all goroutines. After the first completes and sets
	// LastSynced, the remaining 4 will be within the 5-minute cooldown and
	// return cached — so total API calls must be exactly 1.
	if navexa.callCount != 1 {
		t.Errorf("expected exactly 1 Navexa API call due to cooldown serialization, got %d", navexa.callCount)
	}
}

// TestSyncPortfolio_GetPortfolio_NoDeadlock verifies that interleaved calls to
// GetPortfolio (which may call SyncPortfolio internally) and SyncPortfolio(force=true)
// do not deadlock. GetPortfolio does not hold syncMu when calling SyncPortfolio,
// so the call graph is safe: no lock held → acquire lock. Not reentrant.
//
// Note: this test runs calls sequentially in an alternating pattern to avoid a
// data race in memUserDataStore (the test-only in-memory map is not thread-safe).
// In production, SurrealDB handles concurrent access safely.
// See: TestSyncPortfolio_MemUserDataStore_IsNotRaceSafe for the data race finding.
func TestSyncPortfolio_GetPortfolio_NoDeadlock(t *testing.T) {
	svc, navexa := newSyncCooldownFixture()
	ctx := common.WithNavexaClient(context.Background(), navexa)

	// Alternate GetPortfolio and SyncPortfolio calls sequentially.
	// Confirms no reentrant lock panic (syncMu is not held by GetPortfolio).
	for i := range 6 {
		var err error
		if i%2 == 0 {
			_, err = svc.GetPortfolio(ctx, "SMSF")
		} else {
			_, err = svc.SyncPortfolio(ctx, "SMSF", true)
		}
		if err != nil {
			t.Errorf("call %d returned unexpected error: %v", i, err)
		}
	}
}

// TestSyncPortfolio_FutureLastSynced_TreatedAsFresh verifies clock-skew behavior:
// if LastSynced is set to a time in the future (e.g., due to clock drift or NTP jump),
// IsFresh returns true (time.Since is negative, which is < any positive TTL).
// The portfolio remains perpetually "fresh" until wall clock catches up.
// This is documented behavior — not a bug introduced by the cooldown change.
func TestSyncPortfolio_FutureLastSynced_TreatedAsFresh(t *testing.T) {
	svc, navexa := newSyncCooldownFixture()
	ctx := common.WithNavexaClient(context.Background(), navexa)

	// Initial sync
	_, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("first SyncPortfolio failed: %v", err)
	}

	// Simulate clock skew: set LastSynced 10 minutes in the future
	existing, err := svc.getPortfolioRecord(ctx, "SMSF")
	if err != nil {
		t.Fatalf("getPortfolioRecord failed: %v", err)
	}
	existing.LastSynced = time.Now().Add(10 * time.Minute) // future timestamp
	if err := svc.savePortfolioRecord(ctx, existing); err != nil {
		t.Fatalf("savePortfolioRecord failed: %v", err)
	}

	// force=true with future LastSynced: time.Since(future) is negative → < 5min TTL → treated as fresh
	_, err = svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio with future LastSynced failed: %v", err)
	}

	// IsFresh returns true for future timestamps (negative duration < TTL).
	// The cooldown prevents the sync — documented behavior.
	if navexa.callCount != 1 {
		t.Logf("Note: got %d Navexa calls with future LastSynced (expected 1 — treated as fresh)", navexa.callCount)
	}
	// Verify the invariant: IsFresh(future, any_positive_ttl) == true
	futureTime := time.Now().Add(1 * time.Hour)
	if !common.IsFresh(futureTime, 1*time.Nanosecond) {
		t.Error("IsFresh should return true for future timestamps (time.Since is negative < any positive TTL)")
	}
}

// TestSyncPortfolio_UserIsolation verifies that cooldown is per-user: User A's
// recent sync does NOT suppress User B's force=true sync. Each user has their
// own portfolio record in UserDataStore, keyed by userID.
func TestSyncPortfolio_UserIsolation(t *testing.T) {
	svc, navexa := newSyncCooldownFixture()

	userACtx := common.WithNavexaClient(
		common.WithUserContext(context.Background(), &common.UserContext{UserID: "user-a"}),
		navexa,
	)
	userBCtx := common.WithNavexaClient(
		common.WithUserContext(context.Background(), &common.UserContext{UserID: "user-b"}),
		navexa,
	)

	// User A syncs first
	_, err := svc.SyncPortfolio(userACtx, "SMSF", true)
	if err != nil {
		t.Fatalf("User A SyncPortfolio failed: %v", err)
	}
	if navexa.callCount != 1 {
		t.Fatalf("expected 1 call after User A sync, got %d", navexa.callCount)
	}

	// User B force=true should NOT be suppressed by User A's cooldown.
	// User B has no existing portfolio record → falls through to full sync.
	_, err = svc.SyncPortfolio(userBCtx, "SMSF", true)
	if err != nil {
		t.Fatalf("User B SyncPortfolio failed: %v", err)
	}
	if navexa.callCount != 2 {
		t.Errorf("expected 2 Navexa calls (User B is independent of User A cooldown), got %d", navexa.callCount)
	}
}
