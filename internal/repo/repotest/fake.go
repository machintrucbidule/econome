// Package repotest provides in-memory fakes of the repo interfaces for service
// tests (technical/09 §4). They satisfy the same repo.UserRepo/SessionRepo/
// SettingsRepo/Txer contracts as the SQLite store, verified by the parity tests
// in internal/repo. The DBTX argument is ignored (there is no real connection);
// WithTx runs the function directly without rollback (atomicity is exercised
// against real SQLite, not the fake).
package repotest

import (
	"context"
	"sort"
	"sync"
	"time"

	"econome/internal/domain"
	"econome/internal/repo"
)

type data struct {
	mu             sync.Mutex
	users          map[int64]domain.User
	sessions       map[int64]domain.Session
	settings       map[int64]domain.Settings
	accounts       map[int64]domain.Account
	categories     map[int64]domain.Category
	envelopes      map[int64]domain.Envelope
	allocations    map[int64]domain.Allocation
	transactions   map[int64]domain.Transaction
	periods        map[int64]domain.Period
	periodEvents   map[int64]domain.PeriodEvent
	snapshots      map[int64]domain.Snapshot
	networthMonths map[int64]domain.NetworthMonth
	labels         map[int64]domain.LabelMapping
	uiPrefs        map[int64]domain.UIPreference
	invitations    map[int64]domain.Invitation
	totpBackups    map[int64]domain.TOTPBackupCode
	nextUser       int64
	nextSession    int64
	nextID         int64 // shared counter for budget/lifecycle entities
}

func (d *data) id() int64 {
	d.nextID++
	return d.nextID
}

// Store is an in-memory fake of *repo.Store.
type Store struct {
	d              *data
	Users          repo.UserRepo
	Sessions       repo.SessionRepo
	Settings       repo.SettingsRepo
	Accounts       repo.AccountRepo
	Categories     repo.CategoryRepo
	Envelopes      repo.EnvelopeRepo
	Allocations    repo.AllocationRepo
	Transactions   repo.TransactionRepo
	Periods        repo.PeriodRepo
	PeriodEvents   repo.PeriodEventRepo
	Snapshots      repo.SnapshotRepo
	NetworthMonths repo.NetworthMonthRepo
	Labels         repo.LabelMappingRepo
	UIPreferences  repo.UIPreferenceRepo
	Invitations    repo.InvitationRepo
	TOTPBackups    repo.TOTPBackupRepo
}

// NewStore returns an empty in-memory store.
func NewStore() *Store {
	d := &data{
		users:          map[int64]domain.User{},
		sessions:       map[int64]domain.Session{},
		settings:       map[int64]domain.Settings{},
		accounts:       map[int64]domain.Account{},
		categories:     map[int64]domain.Category{},
		envelopes:      map[int64]domain.Envelope{},
		allocations:    map[int64]domain.Allocation{},
		transactions:   map[int64]domain.Transaction{},
		periods:        map[int64]domain.Period{},
		periodEvents:   map[int64]domain.PeriodEvent{},
		snapshots:      map[int64]domain.Snapshot{},
		networthMonths: map[int64]domain.NetworthMonth{},
		labels:         map[int64]domain.LabelMapping{},
		uiPrefs:        map[int64]domain.UIPreference{},
		invitations:    map[int64]domain.Invitation{},
		totpBackups:    map[int64]domain.TOTPBackupCode{},
	}
	return &Store{
		d: d, Users: fakeUsers{d}, Sessions: fakeSessions{d}, Settings: fakeSettings{d},
		Accounts: fakeAccounts{d}, Categories: fakeCategories{d}, Envelopes: fakeEnvelopes{d},
		Allocations: fakeAllocations{d}, Transactions: fakeTransactions{d},
		Periods: fakePeriods{d}, PeriodEvents: fakePeriodEvents{d}, Snapshots: fakeSnapshots{d},
		NetworthMonths: fakeNetworthMonths{d}, Labels: fakeLabels{d}, UIPreferences: fakeUIPrefs{d},
		Invitations: fakeInvitations{d}, TOTPBackups: fakeTOTPBackups{d},
	}
}

// DB satisfies repo.Txer; the fakes ignore the returned value.
func (s *Store) DB() repo.DBTX { return nil }

// WithTx runs fn directly (no real transaction; no rollback on error).
func (s *Store) WithTx(_ context.Context, fn func(q repo.DBTX) error) error {
	return fn(nil)
}

type fakeUsers struct{ d *data }

func (f fakeUsers) CountUsers(_ context.Context, _ repo.DBTX) (int, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	return len(f.d.users), nil
}

func (f fakeUsers) GetByEmail(_ context.Context, _ repo.DBTX, email string) (*domain.User, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, u := range f.d.users {
		if u.Email == email {
			uu := u
			return &uu, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f fakeUsers) GetByID(_ context.Context, _ repo.DBTX, id int64) (*domain.User, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if u, ok := f.d.users[id]; ok {
		uu := u
		return &uu, nil
	}
	return nil, domain.ErrNotFound
}

func (f fakeUsers) Create(_ context.Context, _ repo.DBTX, u *domain.User) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, ex := range f.d.users {
		if ex.Email == u.Email {
			return 0, domain.ErrDuplicate
		}
	}
	f.d.nextUser++
	u.ID = f.d.nextUser
	f.d.users[u.ID] = *u
	return u.ID, nil
}

func (f fakeUsers) UpdateLoginState(_ context.Context, _ repo.DBTX, u *domain.User) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if _, ok := f.d.users[u.ID]; !ok {
		return domain.ErrNotFound
	}
	f.d.users[u.ID] = *u
	return nil
}

func (f fakeUsers) UpdatePasswordHash(_ context.Context, _ repo.DBTX, id int64, hash string) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	u, ok := f.d.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.PasswordHash = hash
	u.UpdatedAt = time.Now().UTC()
	f.d.users[id] = u
	return nil
}

func (f fakeUsers) CountActiveAdmins(_ context.Context, _ repo.DBTX) (int, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	n := 0
	for _, u := range f.d.users {
		if u.IsAdmin && u.Status == domain.StatusActive {
			n++
		}
	}
	return n, nil
}

func (f fakeUsers) ListAll(_ context.Context, _ repo.DBTX) ([]domain.User, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	ids := make([]int64, 0, len(f.d.users))
	for id := range f.d.users {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]domain.User, 0, len(ids))
	for _, id := range ids {
		out = append(out, f.d.users[id])
	}
	return out, nil
}

func (f fakeUsers) SetPassword(_ context.Context, _ repo.DBTX, id int64, hash string, mustChange bool) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	u, ok := f.d.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.PasswordHash = hash
	u.MustChangePassword = mustChange
	u.UpdatedAt = time.Now().UTC()
	f.d.users[id] = u
	return nil
}

func (f fakeUsers) UpdateEmail(_ context.Context, _ repo.DBTX, id int64, email string) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, ex := range f.d.users {
		if ex.Email == email && ex.ID != id {
			return domain.ErrDuplicate
		}
	}
	u, ok := f.d.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.Email = email
	u.UpdatedAt = time.Now().UTC()
	f.d.users[id] = u
	return nil
}

func (f fakeUsers) UpdateTOTP(_ context.Context, _ repo.DBTX, id int64, enabled bool, secret *string) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	u, ok := f.d.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.TOTPEnabled = enabled
	u.TOTPSecret = secret
	u.UpdatedAt = time.Now().UTC()
	f.d.users[id] = u
	return nil
}

func (f fakeUsers) UpdateStatus(_ context.Context, _ repo.DBTX, id int64, status domain.Status) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	u, ok := f.d.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.Status = status
	u.UpdatedAt = time.Now().UTC()
	f.d.users[id] = u
	return nil
}

func (f fakeUsers) SetAdmin(_ context.Context, _ repo.DBTX, id int64, isAdmin bool) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	u, ok := f.d.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.IsAdmin = isAdmin
	u.UpdatedAt = time.Now().UTC()
	f.d.users[id] = u
	return nil
}

type fakeSessions struct{ d *data }

func (f fakeSessions) Create(_ context.Context, _ repo.DBTX, s *domain.Session) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, ex := range f.d.sessions {
		if ex.TokenHash == s.TokenHash {
			return 0, domain.ErrDuplicate
		}
	}
	f.d.nextSession++
	s.ID = f.d.nextSession
	f.d.sessions[s.ID] = *s
	return s.ID, nil
}

func (f fakeSessions) GetByTokenHash(_ context.Context, _ repo.DBTX, tokenHash string) (*domain.Session, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, s := range f.d.sessions {
		if s.TokenHash == tokenHash {
			ss := s
			return &ss, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f fakeSessions) ListByUser(_ context.Context, _ repo.DBTX, userID int64) ([]domain.Session, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.Session
	for _, s := range f.d.sessions {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (f fakeSessions) DeleteByUserScoped(_ context.Context, _ repo.DBTX, userID, id int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	s, ok := f.d.sessions[id]
	if !ok || s.UserID != userID {
		return domain.ErrNotFound
	}
	delete(f.d.sessions, id)
	return nil
}

func (f fakeSessions) DeleteByUserExcept(_ context.Context, _ repo.DBTX, userID, keepID int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for id, s := range f.d.sessions {
		if s.UserID == userID && id != keepID {
			delete(f.d.sessions, id)
		}
	}
	return nil
}

func (f fakeSessions) Touch(_ context.Context, _ repo.DBTX, id int64, lastSeen, expires time.Time) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	s, ok := f.d.sessions[id]
	if !ok {
		return domain.ErrNotFound
	}
	s.LastSeenAt = lastSeen
	s.ExpiresAt = expires
	f.d.sessions[id] = s
	return nil
}

func (f fakeSessions) Delete(_ context.Context, _ repo.DBTX, id int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	delete(f.d.sessions, id)
	return nil
}

func (f fakeSessions) DeleteByUser(_ context.Context, _ repo.DBTX, userID int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for id, s := range f.d.sessions {
		if s.UserID == userID {
			delete(f.d.sessions, id)
		}
	}
	return nil
}

type fakeSettings struct{ d *data }

func (f fakeSettings) Create(_ context.Context, _ repo.DBTX, s *domain.Settings) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if _, ok := f.d.settings[s.UserID]; ok {
		return domain.ErrDuplicate
	}
	f.d.settings[s.UserID] = *s
	return nil
}

func (f fakeSettings) Get(_ context.Context, _ repo.DBTX, userID int64) (*domain.Settings, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if s, ok := f.d.settings[userID]; ok {
		ss := s
		return &ss, nil
	}
	return nil, domain.ErrNotFound
}

func (f fakeSettings) Update(_ context.Context, _ repo.DBTX, s *domain.Settings) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if _, ok := f.d.settings[s.UserID]; !ok {
		return domain.ErrNotFound
	}
	f.d.settings[s.UserID] = *s
	return nil
}
