package services

import (
	"context"
	"errors"
	"fmt"

	"econome/internal/domain"
	"econome/internal/engine"
	"econome/internal/repo"
)

// engineInputs assembles engine.Inputs for (userID, period) from the
// repositories — the single place persisted data crosses into the pure engine
// (T4). It loads the user's accounts/categories/envelopes, the period's
// allocations/transactions/snapshots, the calculation parameters from settings,
// and the injected clock as a clock-free domain.Date (the engine never reads a
// clock). StartBalances are derived from the immediately-preceding created
// period under each account's month_end_policy (I-018/I-026); nothing is stored.
//
// Callers that already hold synthetic (non-persisted) allocations/transactions —
// the month-init draft — overwrite in.Allocations/in.Txns after building the
// base. extraTxns nil ⇒ load the period's persisted transactions.
func (s *Service) engineInputs(ctx context.Context, q repo.DBTX, userID int64, period string) (engine.Inputs, error) {
	var in engine.Inputs
	accounts, err := s.accounts.ListByUser(ctx, q, userID)
	if err != nil {
		return in, err
	}
	categories, err := s.categories.ListByUser(ctx, q, userID)
	if err != nil {
		return in, err
	}
	envelopes, err := s.envelopes.ListByUser(ctx, q, userID)
	if err != nil {
		return in, err
	}
	allocations, err := s.allocations.ListByPeriod(ctx, q, userID, period)
	if err != nil {
		return in, err
	}
	txns, err := s.transactions.ListByPeriod(ctx, q, userID, period)
	if err != nil {
		return in, err
	}
	snapshots, err := s.snapshots.ListByPeriod(ctx, q, userID, period)
	if err != nil {
		return in, err
	}
	set, err := s.settings.Get(ctx, q, userID)
	if err != nil {
		return in, err
	}
	starts, err := s.startBalances(ctx, q, userID, period)
	if err != nil {
		return in, err
	}
	in = engine.Inputs{
		Period:        period,
		Accounts:      accounts,
		Categories:    categories,
		Envelopes:     envelopes,
		Allocations:   allocations,
		Txns:          txns,
		Snapshots:     snapshots,
		StartBalances: starts,
		Params: engine.Params{
			PEAInitialDeposit:   set.PEAInitialDeposit,
			PEASocialChargeRate: set.PEASocialChargeRate,
			NearCapThreshold:    set.NearCapThreshold,
			SecuredSavingsBasis: set.SecuredSavingsBasis,
			Today:               s.today(),
		},
	}
	return in, nil
}

// startBalances derives each account's start-of-month for period as the
// immediately-preceding *created* period's projected end (carry = the carried
// close, sweep ≈ 0 once swept, C5/§8). When no prior period row exists (the very
// first created month) every start is 0 (I-026). The recursion is bounded by the
// finite set of created period rows.
func (s *Service) startBalances(ctx context.Context, q repo.DBTX, userID int64, period string) (map[int64]int64, error) {
	prev := previousPeriod(period)
	if _, err := s.periods.ByPeriod(ctx, q, userID, prev); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return map[int64]int64{}, nil // first created month → all starts 0
		}
		return nil, err
	}
	prevIn, err := s.engineInputs(ctx, q, userID, prev)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int64, len(prevIn.Accounts))
	for _, a := range prevIn.Accounts {
		out[a.ID] = prevIn.AccountBalances(a.ID).ProjectedEnd
	}
	return out, nil
}

// today returns the service clock as a clock-free domain.Date (UTC calendar day).
func (s *Service) today() domain.Date {
	n := s.now().UTC()
	return domain.NewDate(n.Year(), int(n.Month()), n.Day())
}

// previousPeriod returns the calendar month before a "YYYY-MM" period.
func previousPeriod(p string) string {
	y, m, ok := parsePeriodYM(p)
	if !ok {
		return p
	}
	m--
	if m == 0 {
		m = 12
		y--
	}
	return fmt.Sprintf("%04d-%02d", y, m)
}

// parsePeriodYM splits "YYYY-MM" into year and month (1–12).
func parsePeriodYM(p string) (year, month int, ok bool) {
	if len(p) != 7 || p[4] != '-' {
		return 0, 0, false
	}
	var y, m int
	if _, err := fmt.Sscanf(p, "%04d-%02d", &y, &m); err != nil || m < 1 || m > 12 {
		return 0, 0, false
	}
	return y, m, true
}
