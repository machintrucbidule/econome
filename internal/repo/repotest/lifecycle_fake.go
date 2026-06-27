package repotest

import (
	"context"
	"sort"
	"strings"
	"time"

	"econome/internal/domain"
	"econome/internal/repo"
)

type fakePeriods struct{ d *data }

func (f fakePeriods) Create(_ context.Context, _ repo.DBTX, p *domain.Period) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, ex := range f.d.periods {
		if ex.UserID == p.UserID && ex.Period == p.Period {
			return 0, domain.ErrDuplicate
		}
	}
	p.ID = f.d.id()
	f.d.periods[p.ID] = *p
	return p.ID, nil
}

func (f fakePeriods) ByPeriod(_ context.Context, _ repo.DBTX, userID int64, period string) (*domain.Period, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, p := range f.d.periods {
		if p.UserID == userID && p.Period == period {
			pp := p
			return &pp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f fakePeriods) UpdateState(_ context.Context, _ repo.DBTX, userID int64, period string, state domain.PeriodState, lockedAt *time.Time) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for id, p := range f.d.periods {
		if p.UserID == userID && p.Period == period {
			p.State = state
			p.LockedAt = lockedAt
			f.d.periods[id] = p
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakePeriodEvents struct{ d *data }

func (f fakePeriodEvents) Append(_ context.Context, _ repo.DBTX, e *domain.PeriodEvent) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	e.ID = f.d.id()
	f.d.periodEvents[e.ID] = *e
	return e.ID, nil
}

func (f fakePeriodEvents) ListByPeriod(_ context.Context, _ repo.DBTX, userID int64, period string) ([]domain.PeriodEvent, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.PeriodEvent
	for _, e := range f.d.periodEvents {
		if e.UserID == userID && e.Period == period {
			out = append(out, e)
		}
	}
	return sortByID(out, func(e domain.PeriodEvent) int64 { return e.ID }), nil
}

type fakeSnapshots struct{ d *data }

func (f fakeSnapshots) Upsert(_ context.Context, _ repo.DBTX, s *domain.Snapshot) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for id, ex := range f.d.snapshots {
		if ex.AccountID == s.AccountID && ex.Period == s.Period {
			ex.GrossValue = s.GrossValue
			f.d.snapshots[id] = ex
			return nil
		}
	}
	s.ID = f.d.id()
	f.d.snapshots[s.ID] = *s
	return nil
}

func (f fakeSnapshots) ByAccountPeriod(_ context.Context, _ repo.DBTX, userID, accountID int64, period string) (*domain.Snapshot, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, s := range f.d.snapshots {
		if s.UserID == userID && s.AccountID == accountID && s.Period == period {
			ss := s
			return &ss, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f fakeSnapshots) ListByPeriod(_ context.Context, _ repo.DBTX, userID int64, period string) ([]domain.Snapshot, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.Snapshot
	for _, s := range f.d.snapshots {
		if s.UserID == userID && s.Period == period {
			out = append(out, s)
		}
	}
	return sortByID(out, func(s domain.Snapshot) int64 { return s.ID }), nil
}

func (f fakeSnapshots) ListByUser(_ context.Context, _ repo.DBTX, userID int64) ([]domain.Snapshot, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.Snapshot
	for _, s := range f.d.snapshots {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Period != out[j].Period {
			return out[i].Period < out[j].Period
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (f fakeSnapshots) Delete(_ context.Context, _ repo.DBTX, userID, id int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if s, ok := f.d.snapshots[id]; !ok || s.UserID != userID {
		return domain.ErrNotFound
	}
	delete(f.d.snapshots, id)
	return nil
}

type fakeNetworthMonths struct{ d *data }

func (f fakeNetworthMonths) Get(_ context.Context, _ repo.DBTX, userID int64, period string) (*domain.NetworthMonth, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, m := range f.d.networthMonths {
		if m.UserID == userID && m.Period == period {
			mm := m
			return &mm, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f fakeNetworthMonths) ListByUser(_ context.Context, _ repo.DBTX, userID int64) ([]domain.NetworthMonth, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.NetworthMonth
	for _, m := range f.d.networthMonths {
		if m.UserID == userID {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Period < out[j].Period })
	return out, nil
}

func (f fakeNetworthMonths) Upsert(_ context.Context, _ repo.DBTX, userID int64, period, comment string) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for id, m := range f.d.networthMonths {
		if m.UserID == userID && m.Period == period {
			m.Comment = comment
			f.d.networthMonths[id] = m
			return nil
		}
	}
	id := f.d.id()
	f.d.networthMonths[id] = domain.NetworthMonth{ID: id, UserID: userID, Period: period, Comment: comment}
	return nil
}

type fakeLabels struct{ d *data }

func (f fakeLabels) Search(_ context.Context, _ repo.DBTX, userID int64, prefix string, limit int) ([]domain.LabelMapping, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.LabelMapping
	for _, m := range f.d.labels {
		if m.UserID == userID && strings.HasPrefix(m.LabelKey, prefix) {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UsageCount > out[j].UsageCount })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f fakeLabels) Upsert(_ context.Context, _ repo.DBTX, m *domain.LabelMapping) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for id, ex := range f.d.labels {
		if ex.UserID == m.UserID && ex.LabelKey == m.LabelKey {
			ex.Label = m.Label
			ex.CategoryID = m.CategoryID
			ex.AccountID = m.AccountID
			ex.UsageCount++
			f.d.labels[id] = ex
			return nil
		}
	}
	id := f.d.id()
	mm := *m
	mm.ID = id
	mm.UsageCount = 1
	f.d.labels[id] = mm
	return nil
}

type fakeUIPrefs struct{ d *data }

func (f fakeUIPrefs) Upsert(_ context.Context, _ repo.DBTX, p *domain.UIPreference) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for id, ex := range f.d.uiPrefs {
		if ex.UserID == p.UserID && ex.NodeType == p.NodeType && ex.NodeID == p.NodeID {
			ex.Expanded = p.Expanded
			f.d.uiPrefs[id] = ex
			return nil
		}
	}
	id := f.d.id()
	pp := *p
	pp.ID = id
	f.d.uiPrefs[id] = pp
	return nil
}

func (f fakeUIPrefs) ListByUser(_ context.Context, _ repo.DBTX, userID int64) ([]domain.UIPreference, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.UIPreference
	for _, p := range f.d.uiPrefs {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return sortByID(out, func(p domain.UIPreference) int64 { return p.ID }), nil
}

type fakeInvitations struct{ d *data }

func (f fakeInvitations) Create(_ context.Context, _ repo.DBTX, inv *domain.Invitation) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, ex := range f.d.invitations {
		if ex.TokenHash == inv.TokenHash {
			return 0, domain.ErrDuplicate
		}
	}
	inv.ID = f.d.id()
	f.d.invitations[inv.ID] = *inv
	return inv.ID, nil
}

func (f fakeInvitations) ByTokenHash(_ context.Context, _ repo.DBTX, tokenHash string) (*domain.Invitation, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, inv := range f.d.invitations {
		if inv.TokenHash == tokenHash {
			ii := inv
			return &ii, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f fakeInvitations) Update(_ context.Context, _ repo.DBTX, inv *domain.Invitation) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if _, ok := f.d.invitations[inv.ID]; !ok {
		return domain.ErrNotFound
	}
	f.d.invitations[inv.ID] = *inv
	return nil
}

func (f fakeInvitations) ListByCreator(_ context.Context, _ repo.DBTX, createdBy int64) ([]domain.Invitation, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.Invitation
	for _, inv := range f.d.invitations {
		if inv.CreatedBy == createdBy {
			out = append(out, inv)
		}
	}
	return sortByID(out, func(i domain.Invitation) int64 { return i.ID }), nil
}

type fakeTOTPBackups struct{ d *data }

func (f fakeTOTPBackups) Create(_ context.Context, _ repo.DBTX, c *domain.TOTPBackupCode) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	c.ID = f.d.id()
	f.d.totpBackups[c.ID] = *c
	return c.ID, nil
}

func (f fakeTOTPBackups) ListByUser(_ context.Context, _ repo.DBTX, userID int64) ([]domain.TOTPBackupCode, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.TOTPBackupCode
	for _, c := range f.d.totpBackups {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return sortByID(out, func(c domain.TOTPBackupCode) int64 { return c.ID }), nil
}

func (f fakeTOTPBackups) MarkConsumed(_ context.Context, _ repo.DBTX, userID, id int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	c, ok := f.d.totpBackups[id]
	if !ok || c.UserID != userID || c.ConsumedAt != nil {
		return domain.ErrNotFound
	}
	now := time.Now().UTC()
	c.ConsumedAt = &now
	f.d.totpBackups[id] = c
	return nil
}

func (f fakeTOTPBackups) DeleteByUser(_ context.Context, _ repo.DBTX, userID int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for id, c := range f.d.totpBackups {
		if c.UserID == userID {
			delete(f.d.totpBackups, id)
		}
	}
	return nil
}
