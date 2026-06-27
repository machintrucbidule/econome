package services

import (
	"context"
	"errors"
	"testing"

	"econome/internal/domain"
)

// Net-worth service tests (increment 7, functional/07, rules §12–§13, L7). Real
// SQLite via newService. They prove the read-models match the pure engine
// (PEA net / subtotal / total / deltas), the snapshot/comment CRUD recompute
// chain, that snapshots stay editable while the budget month is LOCKED (L7), and
// the M25 comment auto-prefill ranking.

func snap(t *testing.T, s *Service, uid, accID int64, period string, gross int64) {
	t.Helper()
	if err := s.UpsertSnapshot(context.Background(), uid, accID, period, gross); err != nil {
		t.Fatalf("upsert snapshot %s: %v", period, err)
	}
}

func supportByAccount(d *NetWorthData, accID int64) (NWSupport, bool) {
	for _, sup := range d.Supports {
		if sup.AccountID == accID {
			return sup, true
		}
	}
	return NWSupport{}, false
}

func registerRow(d *RegisterData, period string) (RegisterRow, bool) {
	for _, r := range d.Rows {
		if r.Period == period {
			return r, true
		}
	}
	return RegisterRow{}, false
}

func TestNetWorth_SynthesisFiguresAndDeltas(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid := miOwner(t, s)
	la := mkAccount(t, s, uid, "Livret A", "passbook", "none")
	pea := mkAccount(t, s, uid, "PEA", "securities", "none")

	snap(t, s, uid, la, "2026-05", 1400000)
	snap(t, s, uid, la, "2026-06", 1420000)
	snap(t, s, uid, pea, "2026-05", 1200000)
	snap(t, s, uid, pea, "2026-06", 1300000)

	d, err := s.NetWorthSynthesis(ctx, uid, "2026-06")
	if err != nil {
		t.Fatalf("synthesis: %v", err)
	}
	if d.Empty || !d.HasSavings || !d.TotalHasPrev {
		t.Fatalf("flags: empty=%v hasSavings=%v totalHasPrev=%v", d.Empty, d.HasSavings, d.TotalHasPrev)
	}
	// PEA net = gross × (1 − 17.2 %) with initial deposit 0: 1 076 400 (June), 993 600 (May).
	if d.Subtotal != 1420000 {
		t.Errorf("subtotal = %d, want 1420000", d.Subtotal)
	}
	if d.Total != 2496400 {
		t.Errorf("total = %d, want 2496400 (1420000 + 1076400)", d.Total)
	}
	if d.TotalDelta != 102800 {
		t.Errorf("total delta = %d, want 102800 (20000 + 82800)", d.TotalDelta)
	}
	laSup, _ := supportByAccount(d, la)
	if laSup.Value != 1420000 || laSup.Delta != 20000 || !laSup.HasPrev {
		t.Errorf("livret support = %+v, want value 1420000 delta 20000 hasPrev", laSup)
	}
	peaSup, _ := supportByAccount(d, pea)
	if peaSup.Value != 1076400 || peaSup.Delta != 82800 || peaSup.GrossDelta != 100000 {
		t.Errorf("pea support = %+v, want net 1076400 netΔ 82800 grossΔ 100000", peaSup)
	}

	// The earliest recorded month has no prior → deltas undefined.
	first, err := s.NetWorthSynthesis(ctx, uid, "2026-05")
	if err != nil {
		t.Fatalf("synthesis may: %v", err)
	}
	if first.TotalHasPrev {
		t.Error("earliest month should have TotalHasPrev=false")
	}
	if laFirst, _ := supportByAccount(first, la); laFirst.HasPrev {
		t.Error("earliest month livret should have HasPrev=false")
	}
}

func TestNetWorth_DeleteRecomputesNextDelta(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid := miOwner(t, s)
	la := mkAccount(t, s, uid, "Livret A", "passbook", "none")
	snap(t, s, uid, la, "2026-05", 1000000)
	snap(t, s, uid, la, "2026-06", 1100000)
	snap(t, s, uid, la, "2026-07", 1150000)

	reg, err := s.Register(ctx, uid, "all", "2026-07")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if jul, _ := registerRow(reg, "2026-07"); jul.TotalDelta != 50000 {
		t.Fatalf("july delta = %d, want 50000 (vs june)", jul.TotalDelta)
	}

	// Delete the June snapshot → July now compares to May (L7).
	jun, err := s.NetWorthSynthesis(ctx, uid, "2026-06")
	if err != nil {
		t.Fatalf("synthesis june: %v", err)
	}
	junSup, _ := supportByAccount(jun, la)
	if err := s.DeleteSnapshot(ctx, uid, junSup.SnapshotID); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
	reg2, err := s.Register(ctx, uid, "all", "2026-07")
	if err != nil {
		t.Fatalf("register after delete: %v", err)
	}
	if jul, _ := registerRow(reg2, "2026-07"); jul.TotalDelta != 150000 {
		t.Errorf("july delta after delete = %d, want 150000 (vs may)", jul.TotalDelta)
	}
	if _, ok := registerRow(reg2, "2026-06"); ok {
		t.Error("june row should be gone after deleting its only snapshot")
	}
}

func TestNetWorth_SnapshotEditableWhenLocked(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid := miOwner(t, s)
	la := mkAccount(t, s, uid, "Livret A", "passbook", "none")

	// A LOCKED budget month must not block net-worth entry (L7).
	now := s.now().UTC()
	if _, err := s.periods.Create(ctx, s.tx.DB(), &domain.Period{
		UserID: uid, Period: "2026-06", State: domain.PeriodLocked, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create locked period: %v", err)
	}
	if err := s.UpsertSnapshot(ctx, uid, la, "2026-06", 500000); err != nil {
		t.Fatalf("upsert under lock should succeed (L7): %v", err)
	}
	d, _ := s.NetWorthSynthesis(ctx, uid, "2026-06")
	sup, _ := supportByAccount(d, la)
	if err := s.DeleteSnapshot(ctx, uid, sup.SnapshotID); err != nil {
		t.Fatalf("delete under lock should succeed (L7): %v", err)
	}
}

func TestNetWorth_CommentSharedAcrossSurfaces(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid := miOwner(t, s)
	la := mkAccount(t, s, uid, "Livret A", "passbook", "none")
	snap(t, s, uid, la, "2026-06", 1000000)

	if err := s.SaveComment(ctx, uid, "2026-06", "Versement PEA +++"); err != nil {
		t.Fatalf("save comment: %v", err)
	}
	d, _ := s.NetWorthSynthesis(ctx, uid, "2026-06")
	if d.Comment != "Versement PEA +++" {
		t.Errorf("synthesis comment = %q", d.Comment)
	}
	reg, _ := s.Register(ctx, uid, "all", "2026-06")
	if row, _ := registerRow(reg, "2026-06"); row.Comment != "Versement PEA +++" {
		t.Errorf("register comment = %q, want the same per-month comment", row.Comment)
	}
}

func TestNetWorth_Validation(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid := miOwner(t, s)
	la := mkAccount(t, s, uid, "Livret A", "passbook", "none")
	cur := mkAccount(t, s, uid, "Fortuneo", "current", "sweep")

	var ve *domain.ValidationError
	if err := s.UpsertSnapshot(ctx, uid, la, "2026-06", -1); !errors.As(err, &ve) {
		t.Errorf("negative gross: want ValidationError, got %v", err)
	}
	if err := s.UpsertSnapshot(ctx, uid, cur, "2026-06", 100); !errors.As(err, &ve) {
		t.Errorf("current account snapshot: want ValidationError, got %v", err)
	}
	if err := s.UpsertSnapshot(ctx, uid, 999999, "2026-06", 100); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing/cross-tenant account: want ErrNotFound, got %v", err)
	}
	if err := s.UpsertSnapshot(ctx, uid, la, "2026-13", 100); !errors.As(err, &ve) {
		t.Errorf("bad period: want ValidationError, got %v", err)
	}
}

func TestNetWorth_RegisterOrderingAndCurve(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid := miOwner(t, s)
	la := mkAccount(t, s, uid, "Livret A", "passbook", "none")
	snap(t, s, uid, la, "2026-05", 1000000)
	snap(t, s, uid, la, "2026-06", 1100000)
	snap(t, s, uid, la, "2026-07", 1150000)

	reg, err := s.Register(ctx, uid, "all", "2026-07")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !reg.HasHistory || len(reg.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(reg.Rows))
	}
	if reg.Rows[0].Period != "2026-07" || reg.Rows[2].Period != "2026-05" {
		t.Errorf("rows not most-recent-first: %s..%s", reg.Rows[0].Period, reg.Rows[2].Period)
	}
	if len(reg.CurvePeriods) != 3 || reg.CurvePeriods[0] != "2026-05" {
		t.Errorf("curve periods not oldest-first: %v", reg.CurvePeriods)
	}
	if len(reg.Series) < 2 || !reg.Series[0].IsTotal {
		t.Errorf("series = %d, want total first + ≥1 support", len(reg.Series))
	}
	// Range "6" still fits all three months.
	if reg6, _ := s.Register(ctx, uid, "6", "2026-07"); len(reg6.CurvePeriods) != 3 {
		t.Errorf("6-month curve = %d points, want 3", len(reg6.CurvePeriods))
	}
}

func TestNetWorth_M25Prefill(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid := miOwner(t, s)
	la := mkAccount(t, s, uid, "Livret A", "passbook", "none")
	pea := mkAccount(t, s, uid, "PEA", "securities", "none")
	pee := mkAccount(t, s, uid, "PEE", "employee_savings", "none")
	// Δ bands (I-036): [100,300)€ → +, [300,750)€ → ++, ≥750€ → +++; <100€ dropped.
	snap(t, s, uid, la, "2026-05", 1000000)
	snap(t, s, uid, la, "2026-06", 1015000) // +150 € → +
	snap(t, s, uid, pea, "2026-05", 1000000)
	snap(t, s, uid, pea, "2026-06", 1060000) // gross +600 €; net Δ = 60 000 × 0.828 = 496,80 € → ++
	snap(t, s, uid, pee, "2026-05", 1000000)
	snap(t, s, uid, pee, "2026-06", 1005000) // +50 € → below the floor, not listed

	// Off by default: no movements computed.
	if d, _ := s.NetWorthSynthesis(ctx, uid, "2026-06"); len(d.Movements) != 0 {
		t.Errorf("autoprefill off: want no movements, got %v", d.Movements)
	}

	on := true
	if _, err := s.UpdateSettings(ctx, uid, SettingsInput{CommentAutoprefill: &on}); err != nil {
		t.Fatalf("enable autoprefill: %v", err)
	}
	d, _ := s.NetWorthSynthesis(ctx, uid, "2026-06")
	if len(d.Movements) != 2 {
		t.Fatalf("movements = %d, want 2 (the +50 € PEE move is below the floor)", len(d.Movements))
	}
	if d.Movements[0].Name != "PEA" || d.Movements[0].Intensity != 2 || !d.Movements[0].Up {
		t.Errorf("first movement = %+v, want PEA ++ up (496,80 €, largest)", d.Movements[0])
	}
	if d.Movements[1].Name != "Livret A" || d.Movements[1].Intensity != 1 {
		t.Errorf("second movement = %+v, want Livret A + (150 €)", d.Movements[1])
	}
}
