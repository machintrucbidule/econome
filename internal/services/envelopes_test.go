package services

import (
	"context"
	"testing"
	"time"

	"econome/internal/domain"
)

func variableEnv(name string, accountID int64) EnvelopeInput {
	return EnvelopeInput{Name: name, FlowType: string(domain.FlowExpense), Mode: string(domain.ModeVariable), AccountID: accountID}
}

func TestCreateEnvelopeAndParent(t *testing.T) {
	s, _ := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()
	acc, _ := s.CreateAccount(ctx, uid, currentAccount("Fortuneo"))

	// Inline new parent: "Habitation" under a new "Assurance".
	in := variableEnv("Habitation", acc.ID)
	in.Mode = string(domain.ModeFixedRecurring)
	in.Frequency = string(domain.FreqMonthly)
	in.NewParentName = "Assurance"
	in.DefaultExpanded = true
	amt := int64(2840)
	in.DefaultAmount = &amt
	if _, err := s.CreateEnvelope(ctx, uid, in); err != nil {
		t.Fatalf("create habitation: %v", err)
	}
	// Second child reusing the now-existing parent.
	ov, _ := s.EnvelopesOverview(ctx, uid, true)
	if len(ov.Parents) != 1 || len(ov.Parents[0].Children) != 1 {
		t.Fatalf("want 1 parent w/ 1 child, got %+v", ov.Parents)
	}
	parentID := *ov.Parents[0].Children[0].Category.ParentID
	in2 := variableEnv("Auto", acc.ID)
	in2.Mode = string(domain.ModeFixedRecurring)
	in2.Frequency = string(domain.FreqMonthly)
	in2.ParentID = &parentID
	amt2 := int64(9620)
	in2.DefaultAmount = &amt2
	if _, err := s.CreateEnvelope(ctx, uid, in2); err != nil {
		t.Fatalf("create auto: %v", err)
	}
	ov, _ = s.EnvelopesOverview(ctx, uid, true)
	if len(ov.Parents) != 1 || len(ov.Parents[0].Children) != 2 {
		t.Fatalf("want 1 parent w/ 2 children, got %+v", ov.Parents)
	}
	// Read-only parent sum = Σ children default amounts (exact integer sum).
	if got := ov.Parents[0].SumDefault; got != 2840+9620 {
		t.Fatalf("parent sum = %d, want %d", got, 2840+9620)
	}
	if !ov.Parents[0].Category.DefaultExpanded {
		t.Error("parent default_expanded not seeded")
	}
}

func TestEnvelopeSharedCategoryAndUniqueness(t *testing.T) {
	s, _ := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, uid, currentAccount("Fortuneo"))
	b, _ := s.CreateAccount(ctx, uid, currentAccount("Boursorama"))

	if _, err := s.CreateEnvelope(ctx, uid, variableEnv("Divers", a.ID)); err != nil {
		t.Fatal(err)
	}
	// Same category name on a second account ⇒ one shared category, two envelopes.
	if _, err := s.CreateEnvelope(ctx, uid, variableEnv("Divers", b.ID)); err != nil {
		t.Fatalf("divers on second account: %v", err)
	}
	ov, _ := s.EnvelopesOverview(ctx, uid, true)
	if len(ov.TopLevel) != 2 {
		t.Fatalf("want 2 top-level envelopes, got %d", len(ov.TopLevel))
	}
	if ov.TopLevel[0].Category.ID != ov.TopLevel[1].Category.ID {
		t.Error("the two Divers envelopes should share one category")
	}
	// Same (category × account) twice ⇒ duplicate 422.
	if _, err := s.CreateEnvelope(ctx, uid, variableEnv("Divers", a.ID)); err == nil {
		t.Fatal("duplicate (category × account) should fail")
	} else {
		isValidationOn(t, err, "account_id")
	}
}

func TestEnvelopeValidation(t *testing.T) {
	s, _ := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, uid, savingsAccount("Livret A"))

	// fixed_recurring requires a frequency.
	in := variableEnv("Loyer", a.ID)
	in.Mode = string(domain.ModeFixedRecurring)
	if _, err := s.CreateEnvelope(ctx, uid, in); err == nil {
		t.Fatal("fixed without frequency should fail")
	} else {
		isValidationOn(t, err, "frequency")
	}
	// non-monthly requires a due month.
	in.Frequency = string(domain.FreqAnnual)
	if _, err := s.CreateEnvelope(ctx, uid, in); err == nil {
		t.Fatal("annual without due month should fail")
	} else {
		isValidationOn(t, err, "due_months")
	}
	// residual takes no amount.
	res := EnvelopeInput{Name: "Épargne", FlowType: string(domain.FlowExpense), Mode: string(domain.ModeResidual), AccountID: a.ID}
	amt := int64(100)
	res.DefaultAmount = &amt
	if _, err := s.CreateEnvelope(ctx, uid, res); err == nil {
		t.Fatal("residual with amount should fail")
	} else {
		isValidationOn(t, err, "default_amount")
	}
}

func TestResidualNotDeletable(t *testing.T) {
	s, _ := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, uid, savingsAccount("Livret A"))

	res := EnvelopeInput{Name: "Épargne", FlowType: string(domain.FlowExpense), Mode: string(domain.ModeResidual), AccountID: a.ID}
	e, err := s.CreateEnvelope(ctx, uid, res)
	if err != nil {
		t.Fatalf("create residual: %v", err)
	}
	if _, err := s.DeleteEnvelope(ctx, uid, e.ID); err == nil {
		t.Fatal("residual delete should fail")
	} else {
		isValidationOn(t, err, "mode")
	}
	if err := s.ArchiveEnvelope(ctx, uid, e.ID); err == nil {
		t.Fatal("residual archive should fail")
	}
}

func TestEnvelopeFlowTypeConflict(t *testing.T) {
	s, _ := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, uid, currentAccount("Fortuneo"))

	// Create an expense parent via the first child.
	first := variableEnv("Habitation", a.ID)
	first.NewParentName = "Assurance"
	if _, err := s.CreateEnvelope(ctx, uid, first); err != nil {
		t.Fatal(err)
	}
	// A second child under the same parent name but with a different flow ⇒ 422.
	second := EnvelopeInput{Name: "Prime", FlowType: string(domain.FlowIncome), Mode: string(domain.ModeVariable), AccountID: a.ID, NewParentName: "Assurance"}
	if _, err := s.CreateEnvelope(ctx, uid, second); err == nil {
		t.Fatal("flow-type conflict with parent should fail")
	} else {
		isValidationOn(t, err, "flow_type")
	}
}

func TestDeleteEnvelopeArchivesWhenDependents(t *testing.T) {
	s, store := newServiceStore(t)
	uid := seedUser(t, s)
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, uid, currentAccount("Fortuneo"))

	// No dependents → hard delete.
	e1, _ := s.CreateEnvelope(ctx, uid, variableEnv("Courses", a.ID))
	archived, err := s.DeleteEnvelope(ctx, uid, e1.ID)
	if err != nil || archived {
		t.Fatalf("delete no-deps = archived %v err %v", archived, err)
	}

	// With an allocation → archived.
	e2, _ := s.CreateEnvelope(ctx, uid, variableEnv("Loisirs", a.ID))
	now := time.Now().UTC()
	if _, err := store.Allocations.Create(ctx, store.DB(), &domain.Allocation{UserID: uid, EnvelopeID: e2.ID, Period: "2026-06", PlannedAmount: 5000, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	archived, err = s.DeleteEnvelope(ctx, uid, e2.ID)
	if err != nil || !archived {
		t.Fatalf("delete with-deps = archived %v err %v, want archived", archived, err)
	}
	got, _, err := s.GetEnvelope(ctx, uid, e2.ID)
	if err != nil || got.Status != domain.ArchiveArchived {
		t.Fatalf("envelope not archived: %+v err %v", got, err)
	}

	// Cross-tenant.
	if _, _, err := s.GetEnvelope(ctx, uid+999, e2.ID); err == nil {
		t.Fatal("cross-tenant get should be ErrNotFound")
	}
}
