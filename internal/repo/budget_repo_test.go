package repo_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"econome/internal/domain"
	"econome/internal/repo"
	"econome/internal/repo/repotest"
	"econome/migrations"
)

func ts() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }

func newAccount(userID int64, name string, typ domain.AccountType, policy domain.MonthEndPolicy) *domain.Account {
	return &domain.Account{UserID: userID, Name: name, Type: typ, MonthEndPolicy: policy, Status: domain.ArchiveActive, CreatedAt: ts(), UpdatedAt: ts()}
}

func newBudgetSQLite(t *testing.T) (*repo.Store, int64, int64) {
	t.Helper()
	db, dir := openTestDB(t)
	if err := repo.Migrate(context.Background(), db, migrations.FS, filepath.Join(dir, "backups")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := repo.New(db)
	ctx := context.Background()
	owner, err := st.Users.Create(ctx, st.DB(), &domain.User{Email: "owner@example.org", PasswordHash: "h", Status: domain.StatusActive, Language: domain.LangFR, Currency: "EUR", CreatedAt: ts(), UpdatedAt: ts()})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	other, err := st.Users.Create(ctx, st.DB(), &domain.User{Email: "other@example.org", PasswordHash: "h", Status: domain.StatusActive, Language: domain.LangFR, Currency: "EUR", CreatedAt: ts(), UpdatedAt: ts()})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	return st, owner, other
}

// budgetContract is the behaviour both the SQLite store and the fake satisfy.
func budgetContract(t *testing.T, st storeLike, q repo.DBTX, userID, otherUser int64) {
	t.Helper()
	ctx := context.Background()

	accID, err := st.accounts().Create(ctx, q, newAccount(userID, "Courant", domain.AccountCurrent, domain.PolicySweep))
	if err != nil || accID == 0 {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.accounts().Create(ctx, q, newAccount(userID, "Courant", domain.AccountCurrent, domain.PolicySweep)); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("duplicate account name err = %v, want ErrDuplicate", err)
	}
	if got, err := st.accounts().Get(ctx, q, userID, accID); err != nil || got.Name != "Courant" {
		t.Fatalf("get account = %+v, %v", got, err)
	}
	if _, err := st.accounts().Get(ctx, q, otherUser, accID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cross-tenant account Get err = %v, want ErrNotFound", err)
	}

	catID, err := st.categories().Create(ctx, q, &domain.Category{UserID: userID, Name: "Courses", FlowType: domain.FlowExpense, Status: domain.ArchiveActive, CreatedAt: ts(), UpdatedAt: ts()})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	envID, err := st.envelopes().Create(ctx, q, &domain.Envelope{UserID: userID, CategoryID: catID, AccountID: accID, Mode: domain.ModeVariable, Status: domain.ArchiveActive, CreatedAt: ts(), UpdatedAt: ts()})
	if err != nil {
		t.Fatalf("create envelope: %v", err)
	}
	if _, err := st.envelopes().Create(ctx, q, &domain.Envelope{UserID: userID, CategoryID: catID, AccountID: accID, Mode: domain.ModeVariable, Status: domain.ArchiveActive, CreatedAt: ts(), UpdatedAt: ts()}); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("duplicate envelope (cat,acc) err = %v, want ErrDuplicate", err)
	}
	if envs, err := st.envelopes().ListByAccount(ctx, q, userID, accID); err != nil || len(envs) != 1 {
		t.Fatalf("ListByAccount = %d envs, %v", len(envs), err)
	}

	allID, err := st.allocations().Create(ctx, q, &domain.Allocation{UserID: userID, EnvelopeID: envID, Period: "2026-06", PlannedAmount: 30000, CreatedAt: ts(), UpdatedAt: ts()})
	if err != nil || allID == 0 {
		t.Fatalf("create allocation: %v", err)
	}
	if _, err := st.allocations().Create(ctx, q, &domain.Allocation{UserID: userID, EnvelopeID: envID, Period: "2026-06", PlannedAmount: 1, CreatedAt: ts(), UpdatedAt: ts()}); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("duplicate allocation (env,period) err = %v, want ErrDuplicate", err)
	}
	if a, err := st.allocations().ByEnvelopePeriod(ctx, q, userID, envID, "2026-06"); err != nil || a.PlannedAmount != 30000 {
		t.Fatalf("ByEnvelopePeriod = %+v, %v", a, err)
	}

	cat := catID
	txID, err := st.transactions().Create(ctx, q, &domain.Transaction{UserID: userID, AccountID: accID, CategoryID: &cat, FlowType: domain.FlowExpense, Amount: -12000, BudgetPeriod: "2026-06", Status: domain.StatusCleared, Source: domain.SourceManual, CreatedAt: ts(), UpdatedAt: ts()})
	if err != nil || txID == 0 {
		t.Fatalf("create transaction: %v", err)
	}
	if txs, err := st.transactions().ListByPeriod(ctx, q, userID, "2026-06"); err != nil || len(txs) != 1 {
		t.Fatalf("ListByPeriod = %d, %v", len(txs), err)
	}
	if txs, err := st.transactions().ListByAccountPeriod(ctx, q, userID, accID, "2026-06"); err != nil || len(txs) != 1 {
		t.Fatalf("ListByAccountPeriod = %d, %v", len(txs), err)
	}
	if err := st.transactions().Delete(ctx, q, userID, txID); err != nil {
		t.Fatalf("delete transaction: %v", err)
	}
	if _, err := st.transactions().Get(ctx, q, userID, txID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("after delete Get err = %v, want ErrNotFound", err)
	}
}

// storeLike unifies *repo.Store and *repotest.Store for the contract.
type storeLike interface {
	accounts() repo.AccountRepo
	categories() repo.CategoryRepo
	envelopes() repo.EnvelopeRepo
	allocations() repo.AllocationRepo
	transactions() repo.TransactionRepo
}

type sqliteStoreAdapter struct{ s *repo.Store }

func (a sqliteStoreAdapter) accounts() repo.AccountRepo         { return a.s.Accounts }
func (a sqliteStoreAdapter) categories() repo.CategoryRepo      { return a.s.Categories }
func (a sqliteStoreAdapter) envelopes() repo.EnvelopeRepo       { return a.s.Envelopes }
func (a sqliteStoreAdapter) allocations() repo.AllocationRepo   { return a.s.Allocations }
func (a sqliteStoreAdapter) transactions() repo.TransactionRepo { return a.s.Transactions }

type fakeStoreAdapter struct{ s *repotest.Store }

func (a fakeStoreAdapter) accounts() repo.AccountRepo         { return a.s.Accounts }
func (a fakeStoreAdapter) categories() repo.CategoryRepo      { return a.s.Categories }
func (a fakeStoreAdapter) envelopes() repo.EnvelopeRepo       { return a.s.Envelopes }
func (a fakeStoreAdapter) allocations() repo.AllocationRepo   { return a.s.Allocations }
func (a fakeStoreAdapter) transactions() repo.TransactionRepo { return a.s.Transactions }

func TestBudgetContract_SQLite(t *testing.T) {
	st, owner, other := newBudgetSQLite(t)
	budgetContract(t, sqliteStoreAdapter{st}, st.DB(), owner, other)
}

func TestBudgetContract_Fake(t *testing.T) {
	st := repotest.NewStore()
	budgetContract(t, fakeStoreAdapter{st}, st.DB(), 1, 2)
}

// SQLite-specific: CHECK and FK constraints fire.
func TestBudgetConstraints_SQLite(t *testing.T) {
	st, owner, _ := newBudgetSQLite(t)
	ctx := context.Background()

	// CHECK(amount <> 0).
	cat := int64(0)
	accID, _ := st.Accounts.Create(ctx, st.DB(), newAccount(owner, "C", domain.AccountCurrent, domain.PolicySweep))
	if _, err := st.Transactions.Create(ctx, st.DB(), &domain.Transaction{UserID: owner, AccountID: accID, FlowType: domain.FlowExpense, Amount: 0, BudgetPeriod: "2026-06", Status: domain.StatusCleared, Source: domain.SourceManual, CreatedAt: ts(), UpdatedAt: ts()}); err == nil {
		t.Error("amount=0 should violate CHECK(amount <> 0)")
	}
	_ = cat

	// CHECK enum: invalid account type.
	if _, err := st.Accounts.Create(ctx, st.DB(), &domain.Account{UserID: owner, Name: "Bad", Type: "nope", MonthEndPolicy: domain.PolicySweep, Status: domain.ArchiveActive, CreatedAt: ts(), UpdatedAt: ts()}); err == nil {
		t.Error("invalid account type should violate CHECK")
	}

	// FK RESTRICT: deleting an account that has an envelope is rejected.
	catID, _ := st.Categories.Create(ctx, st.DB(), &domain.Category{UserID: owner, Name: "Cat", FlowType: domain.FlowExpense, Status: domain.ArchiveActive, CreatedAt: ts(), UpdatedAt: ts()})
	if _, err := st.Envelopes.Create(ctx, st.DB(), &domain.Envelope{UserID: owner, CategoryID: catID, AccountID: accID, Mode: domain.ModeVariable, Status: domain.ArchiveActive, CreatedAt: ts(), UpdatedAt: ts()}); err != nil {
		t.Fatalf("create envelope: %v", err)
	}
	if err := st.Accounts.Delete(ctx, st.DB(), owner, accID); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("delete account with dependents err = %v, want ErrConflict", err)
	}
}
