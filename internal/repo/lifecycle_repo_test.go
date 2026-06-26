package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"econome/internal/domain"
	"econome/internal/repo/repotest"
)

func TestLifecycleRepos_SQLite(t *testing.T) {
	st, owner, other := newBudgetSQLite(t)
	ctx := context.Background()
	q := st.DB()

	// period + audit
	if _, err := st.Periods.Create(ctx, q, &domain.Period{UserID: owner, Period: "2026-06", State: domain.PeriodActive, CreatedAt: ts(), UpdatedAt: ts()}); err != nil {
		t.Fatalf("create period: %v", err)
	}
	if _, err := st.Periods.Create(ctx, q, &domain.Period{UserID: owner, Period: "2026-06", State: domain.PeriodActive, CreatedAt: ts(), UpdatedAt: ts()}); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("duplicate period err = %v, want ErrDuplicate", err)
	}
	lockedAt := ts()
	if err := st.Periods.UpdateState(ctx, q, owner, "2026-06", domain.PeriodLocked, &lockedAt); err != nil {
		t.Fatalf("lock period: %v", err)
	}
	if p, err := st.Periods.ByPeriod(ctx, q, owner, "2026-06"); err != nil || p.State != domain.PeriodLocked || p.LockedAt == nil {
		t.Fatalf("ByPeriod = %+v, %v", p, err)
	}
	if _, err := st.Periods.ByPeriod(ctx, q, other, "2026-06"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cross-tenant period err = %v, want ErrNotFound", err)
	}
	if _, err := st.PeriodEvents.Append(ctx, q, &domain.PeriodEvent{UserID: owner, Period: "2026-06", Action: domain.ActionLock, At: ts(), ActorUserID: owner}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if evs, err := st.PeriodEvents.ListByPeriod(ctx, q, owner, "2026-06"); err != nil || len(evs) != 1 {
		t.Fatalf("ListByPeriod events = %d, %v", len(evs), err)
	}

	// snapshot upsert + networth comment
	accID, _ := st.Accounts.Create(ctx, q, newAccount(owner, "LDDS", domain.AccountPassbook, domain.PolicyNone))
	if err := st.Snapshots.Upsert(ctx, q, &domain.Snapshot{UserID: owner, AccountID: accID, Period: "2026-06", GrossValue: 500000}); err != nil {
		t.Fatalf("snapshot insert: %v", err)
	}
	if err := st.Snapshots.Upsert(ctx, q, &domain.Snapshot{UserID: owner, AccountID: accID, Period: "2026-06", GrossValue: 550000}); err != nil {
		t.Fatalf("snapshot update: %v", err)
	}
	if s, err := st.Snapshots.ByAccountPeriod(ctx, q, owner, accID, "2026-06"); err != nil || s.GrossValue != 550000 {
		t.Fatalf("snapshot upsert did not update: %+v, %v", s, err)
	}
	if err := st.NetworthMonths.Upsert(ctx, q, owner, "2026-06", "first"); err != nil {
		t.Fatal(err)
	}
	if err := st.NetworthMonths.Upsert(ctx, q, owner, "2026-06", "edited"); err != nil {
		t.Fatal(err)
	}
	if m, err := st.NetworthMonths.Get(ctx, q, owner, "2026-06"); err != nil || m.Comment != "edited" {
		t.Fatalf("networth comment = %+v, %v", m, err)
	}

	// label learning
	for i := 0; i < 2; i++ {
		if err := st.Labels.Upsert(ctx, q, &domain.LabelMapping{UserID: owner, Label: "Carrefour", LabelKey: "carrefour"}); err != nil {
			t.Fatalf("label upsert: %v", err)
		}
	}
	if res, err := st.Labels.Search(ctx, q, owner, "carr", 10); err != nil || len(res) != 1 || res[0].UsageCount != 2 {
		t.Fatalf("label search = %+v, %v", res, err)
	}

	// ui preference toggle
	if err := st.UIPreferences.Upsert(ctx, q, &domain.UIPreference{UserID: owner, NodeType: domain.NodeCategory, NodeID: 7, Expanded: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.UIPreferences.Upsert(ctx, q, &domain.UIPreference{UserID: owner, NodeType: domain.NodeCategory, NodeID: 7, Expanded: false}); err != nil {
		t.Fatal(err)
	}
	if ps, err := st.UIPreferences.ListByUser(ctx, q, owner); err != nil || len(ps) != 1 || ps[0].Expanded {
		t.Fatalf("ui prefs = %+v, %v", ps, err)
	}

	// invitation lifecycle
	invID, err := st.Invitations.Create(ctx, q, &domain.Invitation{TokenHash: "tok", CreatedBy: owner, ExpiresAt: ts().Add(7 * 24 * time.Hour)})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	consumed := ts()
	if err := st.Invitations.Update(ctx, q, &domain.Invitation{ID: invID, ConsumedAt: &consumed}); err != nil {
		t.Fatalf("consume invitation: %v", err)
	}
	if inv, err := st.Invitations.ByTokenHash(ctx, q, "tok"); err != nil || inv.ConsumedAt == nil {
		t.Fatalf("invitation = %+v, %v", inv, err)
	}

	// totp backup codes
	id1, _ := st.TOTPBackups.Create(ctx, q, &domain.TOTPBackupCode{UserID: owner, CodeHash: "h1"})
	_, _ = st.TOTPBackups.Create(ctx, q, &domain.TOTPBackupCode{UserID: owner, CodeHash: "h2"})
	if codes, err := st.TOTPBackups.ListByUser(ctx, q, owner); err != nil || len(codes) != 2 {
		t.Fatalf("totp list = %d, %v", len(codes), err)
	}
	if err := st.TOTPBackups.MarkConsumed(ctx, q, owner, id1); err != nil {
		t.Fatalf("mark consumed: %v", err)
	}
	if err := st.TOTPBackups.MarkConsumed(ctx, q, owner, id1); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("re-consume err = %v, want ErrNotFound (already consumed)", err)
	}
	if err := st.TOTPBackups.DeleteByUser(ctx, q, owner); err != nil {
		t.Fatal(err)
	}
	if codes, _ := st.TOTPBackups.ListByUser(ctx, q, owner); len(codes) != 0 {
		t.Errorf("after delete: %d codes, want 0", len(codes))
	}
}

// Fake smoke: the in-memory store implements the same behaviour for the key ops.
func TestLifecycleFakes_Smoke(t *testing.T) {
	st := repotest.NewStore()
	ctx := context.Background()
	q := st.DB()

	if _, err := st.Periods.Create(ctx, q, &domain.Period{UserID: 1, Period: "2026-06", State: domain.PeriodActive}); err != nil {
		t.Fatal(err)
	}
	if err := st.Snapshots.Upsert(ctx, q, &domain.Snapshot{UserID: 1, AccountID: 2, Period: "2026-06", GrossValue: 100}); err != nil {
		t.Fatal(err)
	}
	if err := st.Snapshots.Upsert(ctx, q, &domain.Snapshot{UserID: 1, AccountID: 2, Period: "2026-06", GrossValue: 200}); err != nil {
		t.Fatal(err)
	}
	if s, err := st.Snapshots.ByAccountPeriod(ctx, q, 1, 2, "2026-06"); err != nil || s.GrossValue != 200 {
		t.Fatalf("fake snapshot upsert = %+v, %v", s, err)
	}
	for i := 0; i < 3; i++ {
		_ = st.Labels.Upsert(ctx, q, &domain.LabelMapping{UserID: 1, Label: "X", LabelKey: "x"})
	}
	if res, _ := st.Labels.Search(ctx, q, 1, "x", 5); len(res) != 1 || res[0].UsageCount != 3 {
		t.Errorf("fake label = %+v", res)
	}
}
