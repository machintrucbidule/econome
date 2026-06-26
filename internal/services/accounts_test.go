package services

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"econome/internal/domain"
	"econome/internal/repo"
	"econome/migrations"
)

// newServiceStore builds a Service over a fresh SQLite store and returns both so
// tests can seed dependent rows (categories/envelopes/periods) directly.
func newServiceStore(t *testing.T) (*Service, *repo.Store) {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repo.Migrate(context.Background(), db, migrations.FS, filepath.Join(dir, "backups")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := repo.New(db)
	return New(depsFromStore(store, []byte("test-secret-0123456789abcdef0123"))), store
}

// seedUser creates an owner via Setup and returns its id.
func seedUser(t *testing.T, s *Service) int64 {
	t.Helper()
	res, err := s.Setup(context.Background(), validSetup())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return res.User.ID
}

func currentAccount(name string) AccountInput {
	return AccountInput{Name: name, Type: string(domain.AccountCurrent), MonthEndPolicy: string(domain.PolicySweep)}
}

func savingsAccount(name string) AccountInput {
	return AccountInput{Name: name, Type: string(domain.AccountPassbook), MonthEndPolicy: string(domain.PolicyNone)}
}

func isValidationOn(t *testing.T, err error, field string) {
	t.Helper()
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("want field error on %q, got %v", field, ve.Fields)
}

func TestCreateAccountValidation(t *testing.T) {
	s, _ := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()

	if _, err := s.CreateAccount(ctx, uid, currentAccount("Fortuneo")); err != nil {
		t.Fatalf("valid create: %v", err)
	}
	// Empty name.
	_, err := s.CreateAccount(ctx, uid, AccountInput{Type: string(domain.AccountCurrent), MonthEndPolicy: string(domain.PolicySweep)})
	isValidationOn(t, err, "name")
	// Cross-column: current with policy none.
	_, err = s.CreateAccount(ctx, uid, AccountInput{Name: "X", Type: string(domain.AccountCurrent), MonthEndPolicy: string(domain.PolicyNone)})
	isValidationOn(t, err, "month_end_policy")
	// Cross-column: savings with sweep.
	_, err = s.CreateAccount(ctx, uid, AccountInput{Name: "Y", Type: string(domain.AccountPassbook), MonthEndPolicy: string(domain.PolicySweep)})
	isValidationOn(t, err, "month_end_policy")
	// Bad type.
	_, err = s.CreateAccount(ctx, uid, AccountInput{Name: "Z", Type: "nope", MonthEndPolicy: string(domain.PolicyNone)})
	isValidationOn(t, err, "type")
	// Negative ceiling.
	neg := int64(-1)
	_, err = s.CreateAccount(ctx, uid, AccountInput{Name: "W", Type: string(domain.AccountPassbook), MonthEndPolicy: string(domain.PolicyNone), Ceiling: &neg})
	isValidationOn(t, err, "ceiling")
	// Duplicate name → field 422, not a raw conflict.
	_, err = s.CreateAccount(ctx, uid, currentAccount("Fortuneo"))
	isValidationOn(t, err, "name")
}

func TestUpdateAccountAndTenantScope(t *testing.T) {
	s, _ := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()

	a, err := s.CreateAccount(ctx, uid, currentAccount("Fortuneo"))
	if err != nil {
		t.Fatal(err)
	}
	upd := currentAccount("Fortuneo Pro")
	upd.MonthEndPolicy = string(domain.PolicyCarry)
	if _, err := s.UpdateAccount(ctx, uid, a.ID, upd); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.GetAccount(ctx, uid, a.ID)
	if got.Name != "Fortuneo Pro" || got.MonthEndPolicy != domain.PolicyCarry {
		t.Fatalf("update not applied: %+v", got)
	}
	// Cross-tenant: another user cannot see or edit it (404, never 403).
	if _, err := s.GetAccount(ctx, uid+999, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant get = %v, want ErrNotFound", err)
	}
	if _, err := s.UpdateAccount(ctx, uid+999, a.ID, upd); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant update = %v, want ErrNotFound", err)
	}
}

func TestMonthEndPolicyForwardOnlyGuard(t *testing.T) {
	s, store := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()

	a, _ := s.CreateAccount(ctx, uid, currentAccount("Fortuneo"))

	// No periods exist ⇒ a policy change is allowed.
	in := currentAccount("Fortuneo")
	in.MonthEndPolicy = string(domain.PolicyCarry)
	in.EffectivePeriod = "2026-07"
	if _, err := s.UpdateAccount(ctx, uid, a.ID, in); err != nil {
		t.Fatalf("policy change with no periods: %v", err)
	}

	// Lock a period, then a policy change effective on that locked month is refused.
	now := time.Now().UTC()
	if _, err := store.Periods.Create(ctx, store.DB(), &domain.Period{UserID: uid, Period: "2026-05", State: domain.PeriodLocked, LockedAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	in.MonthEndPolicy = string(domain.PolicySweep)
	in.EffectivePeriod = "2026-05"
	if _, err := s.UpdateAccount(ctx, uid, a.ID, in); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("policy change into locked month = %v, want ErrLocked", err)
	}
}

func TestDeleteAccountArchivesWhenDependents(t *testing.T) {
	s, store := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()

	// Account with no dependents → hard delete.
	a, _ := s.CreateAccount(ctx, uid, currentAccount("Temp"))
	archived, err := s.DeleteAccount(ctx, uid, a.ID)
	if err != nil || archived {
		t.Fatalf("delete no-deps = archived %v err %v, want hard delete", archived, err)
	}
	if _, err := s.GetAccount(ctx, uid, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("account still present after hard delete")
	}

	// Account WITH a dependent envelope → archived, history kept.
	b, _ := s.CreateAccount(ctx, uid, currentAccount("Keep"))
	now := time.Now().UTC()
	catID, err := store.Categories.Create(ctx, store.DB(), &domain.Category{UserID: uid, Name: "Loyer", FlowType: domain.FlowExpense, Status: domain.ArchiveActive, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Envelopes.Create(ctx, store.DB(), &domain.Envelope{UserID: uid, CategoryID: catID, AccountID: b.ID, Mode: domain.ModeVariable, Status: domain.ArchiveActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	archived, err = s.DeleteAccount(ctx, uid, b.ID)
	if err != nil || !archived {
		t.Fatalf("delete with-deps = archived %v err %v, want archived", archived, err)
	}
	got, err := s.GetAccount(ctx, uid, b.ID)
	if err != nil || got.Status != domain.ArchiveArchived {
		t.Fatalf("account not archived: %+v err %v", got, err)
	}
}

func TestReorderCascade(t *testing.T) {
	s, _ := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()

	la, _ := s.CreateAccount(ctx, uid, savingsAccount("Livret A"))
	ldds, _ := s.CreateAccount(ctx, uid, savingsAccount("LDDS"))
	cur, _ := s.CreateAccount(ctx, uid, currentAccount("Fortuneo"))

	if err := s.ReorderCascade(ctx, uid, []int64{ldds.ID, la.ID}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	gotLDDS, _ := s.GetAccount(ctx, uid, ldds.ID)
	gotLA, _ := s.GetAccount(ctx, uid, la.ID)
	if gotLDDS.FillPriority == nil || *gotLDDS.FillPriority != 1 {
		t.Fatalf("LDDS priority = %v, want 1", gotLDDS.FillPriority)
	}
	if gotLA.FillPriority == nil || *gotLA.FillPriority != 2 {
		t.Fatalf("Livret A priority = %v, want 2", gotLA.FillPriority)
	}
	// Removing one (reorder without it) clears its priority.
	if err := s.ReorderCascade(ctx, uid, []int64{la.ID}); err != nil {
		t.Fatalf("reorder shrink: %v", err)
	}
	gotLDDS, _ = s.GetAccount(ctx, uid, ldds.ID)
	if gotLDDS.FillPriority != nil {
		t.Fatalf("LDDS still in cascade after removal: %v", gotLDDS.FillPriority)
	}
	gotLA, _ = s.GetAccount(ctx, uid, la.ID)
	if gotLA.FillPriority == nil || *gotLA.FillPriority != 1 {
		t.Fatalf("Livret A priority after shrink = %v, want 1", gotLA.FillPriority)
	}
	// A current account cannot enter the cascade.
	if err := s.ReorderCascade(ctx, uid, []int64{cur.ID}); err == nil {
		t.Fatal("reorder with a current account should fail")
	} else {
		isValidationOn(t, err, "cascade")
	}
}

func TestUpdateSettings(t *testing.T) {
	s, _ := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()

	cur, _ := s.CreateAccount(ctx, uid, currentAccount("Fortuneo"))

	// Valid update across cards.
	dep := int64(900000)
	rate := 1720
	basis := string(domain.BasisFixedOnly)
	lang := string(domain.LangEN)
	defAcc := cur.ID
	if _, err := s.UpdateSettings(ctx, uid, SettingsInput{
		DefaultAccountID: &defAcc, PEAInitialDeposit: &dep, PEASocialChargeRate: &rate,
		SecuredSavingsBasis: &basis, Language: &lang,
	}); err != nil {
		t.Fatalf("valid settings update: %v", err)
	}
	got, _ := s.Settings(ctx, uid)
	if got.PEAInitialDeposit != 900000 || got.SecuredSavingsBasis != domain.BasisFixedOnly || got.Language != domain.LangEN {
		t.Fatalf("settings not applied: %+v", got)
	}
	if got.DefaultAccountID == nil || *got.DefaultAccountID != cur.ID {
		t.Fatalf("default account not set: %v", got.DefaultAccountID)
	}

	// Negative deposit → 422, no partial write.
	negDep := int64(-1)
	if _, err := s.UpdateSettings(ctx, uid, SettingsInput{PEAInitialDeposit: &negDep}); err == nil {
		t.Fatal("negative deposit should fail")
	} else {
		isValidationOn(t, err, "pea_initial_deposit")
	}
	// Unknown default account → 422.
	bad := int64(999999)
	if _, err := s.UpdateSettings(ctx, uid, SettingsInput{DefaultAccountID: &bad}); err == nil {
		t.Fatal("unknown default account should fail")
	} else {
		isValidationOn(t, err, "default_account_id")
	}
	// Bad basis → 422.
	badBasis := "nope"
	if _, err := s.UpdateSettings(ctx, uid, SettingsInput{SecuredSavingsBasis: &badBasis}); err == nil {
		t.Fatal("bad basis should fail")
	} else {
		isValidationOn(t, err, "secured_savings_basis")
	}
}
