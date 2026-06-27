package services

import (
	"context"
	"errors"
	"testing"

	"econome/internal/domain"
)

// Month-lifecycle tests (increment 8, functional/04 §4 L1/L9). Real SQLite via
// newService; the clock is pinned mid-period (fixedClock) so the engine figures
// are deterministic. They prove close/unlock guard budget writes, log
// period_event, and that regenerate is additive + idempotent.

func TestLockMonthGuardsWritesAndLogs(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid, _, _, courses, _ := forecastFixture(t, s)

	// An allocation edit on the active month succeeds.
	if err := s.EditAllocation(ctx, uid, courses, "2026-06", 50000); err != nil {
		t.Fatalf("edit on active month: %v", err)
	}

	// Close the month.
	if err := s.LockMonth(ctx, uid, "2026-06", uid); err != nil {
		t.Fatalf("lock: %v", err)
	}
	p, err := s.periods.ByPeriod(ctx, s.tx.DB(), uid, "2026-06")
	if err != nil {
		t.Fatalf("read period: %v", err)
	}
	if p.State != domain.PeriodLocked || p.LockedAt == nil {
		t.Fatalf("state = %s lockedAt=%v, want locked + timestamp", p.State, p.LockedAt)
	}

	// Now a budget write is refused with ErrLocked (409).
	if err := s.EditAllocation(ctx, uid, courses, "2026-06", 60000); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("edit on locked month = %v, want ErrLocked", err)
	}

	// Re-locking is a no-op conflict.
	if err := s.LockMonth(ctx, uid, "2026-06", uid); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("re-lock = %v, want ErrConflict", err)
	}

	// Unlock re-enables edits and logs the event.
	if err := s.UnlockMonth(ctx, uid, "2026-06", uid); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := s.EditAllocation(ctx, uid, courses, "2026-06", 60000); err != nil {
		t.Fatalf("edit after unlock: %v", err)
	}

	// The audit trail records create, lock, unlock in order.
	evs, err := s.LifecycleEvents(ctx, uid, "2026-06")
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	got := []domain.PeriodAction{}
	for _, e := range evs {
		got = append(got, e.Action)
	}
	want := []domain.PeriodAction{domain.ActionCreate, domain.ActionLock, domain.ActionUnlock}
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestLockUnlockNotCreatedIs404(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid := miOwner(t, s)
	if err := s.LockMonth(ctx, uid, "2026-06", uid); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("lock not-created = %v, want ErrNotFound", err)
	}
	if err := s.UnlockMonth(ctx, uid, "2026-06", uid); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unlock not-created = %v, want ErrNotFound", err)
	}
}

func TestRegenerateMissingRecurringIsAdditiveAndIdempotent(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid, sweep, _, _, _ := forecastFixture(t, s)

	allocsBefore := countAllocs(t, s, uid, "2026-06")

	// Add a new fixed-recurring envelope AFTER the month was created.
	mkEnv(t, s, uid, EnvelopeInput{Name: "Assurance vie", FlowType: "expense", AccountID: sweep, Mode: "fixed_recurring", DefaultAmount: amt(4000), Frequency: "monthly", ExpectedDay: day(10)})

	n, err := s.RegenerateMissingRecurring(ctx, uid, "2026-06")
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if n != 1 {
		t.Fatalf("regenerate added %d, want 1", n)
	}
	if got := countAllocs(t, s, uid, "2026-06"); got != allocsBefore+1 {
		t.Fatalf("allocs after regenerate = %d, want %d", got, allocsBefore+1)
	}

	// Idempotent: a second run adds nothing.
	n2, err := s.RegenerateMissingRecurring(ctx, uid, "2026-06")
	if err != nil {
		t.Fatalf("regenerate again: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second regenerate added %d, want 0", n2)
	}
	if got := countAllocs(t, s, uid, "2026-06"); got != allocsBefore+1 {
		t.Fatalf("allocs after idempotent run = %d, want %d", got, allocsBefore+1)
	}
}

func TestRegenerateRefusedOnLockedMonth(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid, _, _, _, _ := forecastFixture(t, s)
	if err := s.LockMonth(ctx, uid, "2026-06", uid); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if _, err := s.RegenerateMissingRecurring(ctx, uid, "2026-06"); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("regenerate on locked = %v, want ErrLocked", err)
	}
}

func countAllocs(t *testing.T, s *Service, uid int64, period string) int {
	t.Helper()
	as, err := s.allocations.ListByPeriod(context.Background(), s.tx.DB(), uid, period)
	if err != nil {
		t.Fatalf("list allocs: %v", err)
	}
	return len(as)
}
