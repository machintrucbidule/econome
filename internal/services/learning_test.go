package services

import (
	"context"
	"errors"
	"testing"

	"econome/internal/domain"
)

// Learned-label (M21) + expand-persistence (M4) tests (increment 6d).

func parentCatID(t *testing.T, s *Service, uid int64, name string) int64 {
	t.Helper()
	cats, err := s.categories.ListByUser(context.Background(), s.tx.DB(), uid)
	if err != nil {
		t.Fatalf("cats: %v", err)
	}
	for _, c := range cats {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("category %q not found", name)
	return 0
}

func TestLearning_RecordAndTopLabels(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, _, _, _, courses := journalFixture(t, s)

	for i := 0; i < 3; i++ {
		mkTxn(t, s, uid, TxnInput{Label: "Boulangerie", CategoryID: &courses, AccountID: sweep, Magnitude: 500, Status: domain.StatusCleared, OpDate: date(2026, 6, 3)})
	}
	mkTxn(t, s, uid, TxnInput{Label: "Café", CategoryID: &courses, AccountID: sweep, Magnitude: 350, Status: domain.StatusCleared, OpDate: date(2026, 6, 4)})

	top, err := s.TopLabels(ctx, uid, 10)
	if err != nil {
		t.Fatalf("TopLabels: %v", err)
	}
	if len(top) != 2 || top[0].Label != "Boulangerie" || top[0].UsageCount != 3 {
		t.Errorf("top labels = %+v, want Boulangerie(3) first", top)
	}
	if top[0].AccountID == nil || *top[0].AccountID != sweep || top[0].CategoryID == nil || *top[0].CategoryID != courses {
		t.Errorf("label not associated with its category/account: %+v", top[0])
	}
}

func TestLearning_ExpandPersistsToForecast(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, _, _, _ := forecastFixture(t, s) // Assurance parent w/ children
	assurance := parentCatID(t, s, uid, "Assurance")

	// Collapsed by default (no pref, no default_expanded).
	d, _ := s.Forecast(ctx, uid, "2026-06", idStr(sweep))
	row, ok := rowByName(d.Rows, "Assurance")
	if !ok || row.Open {
		t.Fatalf("Assurance should be collapsed by default: %+v", row)
	}

	// Persist expanded → the forecast renders it open + children visible.
	if err := s.SetExpand(ctx, uid, domain.NodeCategory, assurance, true); err != nil {
		t.Fatalf("SetExpand: %v", err)
	}
	prefs, _ := s.ExpandPrefs(ctx, uid)
	if !prefs[NodeKey{Type: domain.NodeCategory, ID: assurance}] {
		t.Error("ExpandPrefs missing the set node")
	}
	d, _ = s.Forecast(ctx, uid, "2026-06", idStr(sweep))
	row, _ = rowByName(d.Rows, "Assurance")
	if !row.Open {
		t.Error("Assurance should render expanded after SetExpand")
	}

	// Invalid node → 422.
	var ve *domain.ValidationError
	if err := s.SetExpand(ctx, uid, domain.NodeType("bogus"), assurance, true); !errors.As(err, &ve) {
		t.Errorf("invalid node type err = %v, want ValidationError", err)
	}
}
