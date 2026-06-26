// Package repotest provides in-memory fakes of the repo interfaces for service
// tests (technical/09 §4). They satisfy the same repo.UserRepo/SessionRepo/
// SettingsRepo/Txer contracts as the SQLite store, verified by the parity tests
// in internal/repo. The DBTX argument is ignored (there is no real connection);
// WithTx runs the function directly without rollback (atomicity is exercised
// against real SQLite, not the fake).
package repotest

import (
	"context"
	"sync"
	"time"

	"econome/internal/domain"
	"econome/internal/repo"
)

type data struct {
	mu           sync.Mutex
	users        map[int64]domain.User
	sessions     map[int64]domain.Session
	settings     map[int64]domain.Settings
	accounts     map[int64]domain.Account
	categories   map[int64]domain.Category
	envelopes    map[int64]domain.Envelope
	allocations  map[int64]domain.Allocation
	transactions map[int64]domain.Transaction
	nextUser     int64
	nextSession  int64
	nextID       int64 // shared counter for budget entities
}

func (d *data) id() int64 {
	d.nextID++
	return d.nextID
}

// Store is an in-memory fake of *repo.Store.
type Store struct {
	d            *data
	Users        repo.UserRepo
	Sessions     repo.SessionRepo
	Settings     repo.SettingsRepo
	Accounts     repo.AccountRepo
	Categories   repo.CategoryRepo
	Envelopes    repo.EnvelopeRepo
	Allocations  repo.AllocationRepo
	Transactions repo.TransactionRepo
}

// NewStore returns an empty in-memory store.
func NewStore() *Store {
	d := &data{
		users:        map[int64]domain.User{},
		sessions:     map[int64]domain.Session{},
		settings:     map[int64]domain.Settings{},
		accounts:     map[int64]domain.Account{},
		categories:   map[int64]domain.Category{},
		envelopes:    map[int64]domain.Envelope{},
		allocations:  map[int64]domain.Allocation{},
		transactions: map[int64]domain.Transaction{},
	}
	return &Store{
		d: d, Users: fakeUsers{d}, Sessions: fakeSessions{d}, Settings: fakeSettings{d},
		Accounts: fakeAccounts{d}, Categories: fakeCategories{d}, Envelopes: fakeEnvelopes{d},
		Allocations: fakeAllocations{d}, Transactions: fakeTransactions{d},
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
