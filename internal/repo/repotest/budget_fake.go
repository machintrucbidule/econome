package repotest

import (
	"context"

	"econome/internal/domain"
	"econome/internal/repo"
)

// In-memory budget repositories. They ignore the DBTX argument and do not
// enforce foreign keys (FK behaviour is exercised against real SQLite); they do
// enforce the UNIQUE constraints the contract tests rely on, and every read/write
// is user_id-scoped.

type fakeAccounts struct{ d *data }

func (f fakeAccounts) Create(_ context.Context, _ repo.DBTX, a *domain.Account) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, ex := range f.d.accounts {
		if ex.UserID == a.UserID && ex.Name == a.Name {
			return 0, domain.ErrDuplicate
		}
	}
	a.ID = f.d.id()
	f.d.accounts[a.ID] = *a
	return a.ID, nil
}

func (f fakeAccounts) Get(_ context.Context, _ repo.DBTX, userID, id int64) (*domain.Account, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if a, ok := f.d.accounts[id]; ok && a.UserID == userID {
		aa := a
		return &aa, nil
	}
	return nil, domain.ErrNotFound
}

func (f fakeAccounts) Update(_ context.Context, _ repo.DBTX, a *domain.Account) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	ex, ok := f.d.accounts[a.ID]
	if !ok || ex.UserID != a.UserID {
		return domain.ErrNotFound
	}
	for _, o := range f.d.accounts {
		if o.ID != a.ID && o.UserID == a.UserID && o.Name == a.Name {
			return domain.ErrDuplicate
		}
	}
	f.d.accounts[a.ID] = *a
	return nil
}

func (f fakeAccounts) Delete(_ context.Context, _ repo.DBTX, userID, id int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if a, ok := f.d.accounts[id]; !ok || a.UserID != userID {
		return domain.ErrNotFound
	}
	delete(f.d.accounts, id)
	return nil
}

func (f fakeAccounts) ListByUser(_ context.Context, _ repo.DBTX, userID int64) ([]domain.Account, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.Account
	for _, a := range f.d.accounts {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return sortByID(out, func(a domain.Account) int64 { return a.ID }), nil
}

type fakeCategories struct{ d *data }

func (f fakeCategories) Create(_ context.Context, _ repo.DBTX, c *domain.Category) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	c.ID = f.d.id()
	f.d.categories[c.ID] = *c
	return c.ID, nil
}

func (f fakeCategories) Get(_ context.Context, _ repo.DBTX, userID, id int64) (*domain.Category, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if c, ok := f.d.categories[id]; ok && c.UserID == userID {
		cc := c
		return &cc, nil
	}
	return nil, domain.ErrNotFound
}

func (f fakeCategories) Update(_ context.Context, _ repo.DBTX, c *domain.Category) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if ex, ok := f.d.categories[c.ID]; !ok || ex.UserID != c.UserID {
		return domain.ErrNotFound
	}
	f.d.categories[c.ID] = *c
	return nil
}

func (f fakeCategories) Delete(_ context.Context, _ repo.DBTX, userID, id int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if c, ok := f.d.categories[id]; !ok || c.UserID != userID {
		return domain.ErrNotFound
	}
	delete(f.d.categories, id)
	return nil
}

func (f fakeCategories) ListByUser(_ context.Context, _ repo.DBTX, userID int64) ([]domain.Category, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.Category
	for _, c := range f.d.categories {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return sortByID(out, func(c domain.Category) int64 { return c.ID }), nil
}

type fakeEnvelopes struct{ d *data }

func (f fakeEnvelopes) Create(_ context.Context, _ repo.DBTX, e *domain.Envelope) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, ex := range f.d.envelopes {
		if ex.UserID == e.UserID && ex.CategoryID == e.CategoryID && ex.AccountID == e.AccountID {
			return 0, domain.ErrDuplicate
		}
	}
	e.ID = f.d.id()
	f.d.envelopes[e.ID] = *e
	return e.ID, nil
}

func (f fakeEnvelopes) Get(_ context.Context, _ repo.DBTX, userID, id int64) (*domain.Envelope, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if e, ok := f.d.envelopes[id]; ok && e.UserID == userID {
		ee := e
		return &ee, nil
	}
	return nil, domain.ErrNotFound
}

func (f fakeEnvelopes) Update(_ context.Context, _ repo.DBTX, e *domain.Envelope) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if ex, ok := f.d.envelopes[e.ID]; !ok || ex.UserID != e.UserID {
		return domain.ErrNotFound
	}
	f.d.envelopes[e.ID] = *e
	return nil
}

func (f fakeEnvelopes) Delete(_ context.Context, _ repo.DBTX, userID, id int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if e, ok := f.d.envelopes[id]; !ok || e.UserID != userID {
		return domain.ErrNotFound
	}
	delete(f.d.envelopes, id)
	return nil
}

func (f fakeEnvelopes) ListByUser(_ context.Context, _ repo.DBTX, userID int64) ([]domain.Envelope, error) {
	return f.filter(func(e domain.Envelope) bool { return e.UserID == userID })
}

func (f fakeEnvelopes) ListByAccount(_ context.Context, _ repo.DBTX, userID, accountID int64) ([]domain.Envelope, error) {
	return f.filter(func(e domain.Envelope) bool { return e.UserID == userID && e.AccountID == accountID })
}

func (f fakeEnvelopes) filter(keep func(domain.Envelope) bool) ([]domain.Envelope, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.Envelope
	for _, e := range f.d.envelopes {
		if keep(e) {
			out = append(out, e)
		}
	}
	return sortByID(out, func(e domain.Envelope) int64 { return e.ID }), nil
}

type fakeAllocations struct{ d *data }

func (f fakeAllocations) Create(_ context.Context, _ repo.DBTX, a *domain.Allocation) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, ex := range f.d.allocations {
		if ex.EnvelopeID == a.EnvelopeID && ex.Period == a.Period {
			return 0, domain.ErrDuplicate
		}
	}
	a.ID = f.d.id()
	f.d.allocations[a.ID] = *a
	return a.ID, nil
}

func (f fakeAllocations) Update(_ context.Context, _ repo.DBTX, a *domain.Allocation) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if ex, ok := f.d.allocations[a.ID]; !ok || ex.UserID != a.UserID {
		return domain.ErrNotFound
	}
	f.d.allocations[a.ID] = *a
	return nil
}

func (f fakeAllocations) Delete(_ context.Context, _ repo.DBTX, userID, id int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if a, ok := f.d.allocations[id]; !ok || a.UserID != userID {
		return domain.ErrNotFound
	}
	delete(f.d.allocations, id)
	return nil
}

func (f fakeAllocations) ByEnvelopePeriod(_ context.Context, _ repo.DBTX, userID, envelopeID int64, period string) (*domain.Allocation, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	for _, a := range f.d.allocations {
		if a.UserID == userID && a.EnvelopeID == envelopeID && a.Period == period {
			aa := a
			return &aa, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f fakeAllocations) ListByPeriod(_ context.Context, _ repo.DBTX, userID int64, period string) ([]domain.Allocation, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.Allocation
	for _, a := range f.d.allocations {
		if a.UserID == userID && a.Period == period {
			out = append(out, a)
		}
	}
	return sortByID(out, func(a domain.Allocation) int64 { return a.ID }), nil
}

type fakeTransactions struct{ d *data }

func (f fakeTransactions) Create(_ context.Context, _ repo.DBTX, t *domain.Transaction) (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	t.ID = f.d.id()
	f.d.transactions[t.ID] = *t
	return t.ID, nil
}

func (f fakeTransactions) Get(_ context.Context, _ repo.DBTX, userID, id int64) (*domain.Transaction, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if t, ok := f.d.transactions[id]; ok && t.UserID == userID {
		tt := t
		return &tt, nil
	}
	return nil, domain.ErrNotFound
}

func (f fakeTransactions) Update(_ context.Context, _ repo.DBTX, t *domain.Transaction) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if ex, ok := f.d.transactions[t.ID]; !ok || ex.UserID != t.UserID {
		return domain.ErrNotFound
	}
	f.d.transactions[t.ID] = *t
	return nil
}

func (f fakeTransactions) Delete(_ context.Context, _ repo.DBTX, userID, id int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if t, ok := f.d.transactions[id]; !ok || t.UserID != userID {
		return domain.ErrNotFound
	}
	delete(f.d.transactions, id)
	return nil
}

func (f fakeTransactions) ListByPeriod(_ context.Context, _ repo.DBTX, userID int64, period string) ([]domain.Transaction, error) {
	return f.filter(func(t domain.Transaction) bool { return t.UserID == userID && t.BudgetPeriod == period })
}

func (f fakeTransactions) ListByAccountPeriod(_ context.Context, _ repo.DBTX, userID, accountID int64, period string) ([]domain.Transaction, error) {
	return f.filter(func(t domain.Transaction) bool {
		return t.UserID == userID && t.AccountID == accountID && t.BudgetPeriod == period
	})
}

func (f fakeTransactions) filter(keep func(domain.Transaction) bool) ([]domain.Transaction, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	var out []domain.Transaction
	for _, t := range f.d.transactions {
		if keep(t) {
			out = append(out, t)
		}
	}
	return sortByID(out, func(t domain.Transaction) int64 { return t.ID }), nil
}

// sortByID returns the slice ordered by ascending id (map iteration is random).
func sortByID[T any](xs []T, id func(T) int64) []T {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && id(xs[j]) < id(xs[j-1]); j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
	return xs
}
