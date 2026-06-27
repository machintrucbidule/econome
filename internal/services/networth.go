package services

import (
	"context"
	"errors"
	"sort"

	"econome/internal/domain"
	"econome/internal/engine"
	"econome/internal/repo"
)

// Net-worth read-models + snapshot/comment mutations (functional/07, rules
// §12–§13, lifecycle §3.6/L7). Derived-not-stored: only gross snapshots and the
// per-month comment are inputs; PEA net, subtotals, the total and every delta are
// the pure engine's output, recomputed on read. Snapshots and comments are
// **always editable, independent of the budget month lock** (L7) — no
// ensureEditable guard funnels these mutations.

// NWSupport is one savings support's figures for the focus month.
type NWSupport struct {
	AccountID   int64
	Name        string
	Type        domain.AccountType
	HasSnapshot bool
	SnapshotID  int64
	Gross       int64  // entered gross (0 when no snapshot)
	Value       int64  // pea_net for securities, gross otherwise (rules §12–§13)
	Delta       int64  // value − previous snapshot's value
	GrossDelta  int64  // gross − previous gross (the PEA gross row's Δ)
	HasPrev     bool   // a prior snapshot exists for this account → Δ is defined
	Ceiling     *int64 // regulatory/chosen cap (for the near-cap subtext)
}

// NWMovement is one support's month-over-month change, ranked for the M25
// comment auto-prefill (intensity 1..3 by relative magnitude).
type NWMovement struct {
	Name      string
	Up        bool
	Intensity int // 1 (+/−), 2 (++/−−), 3 (+++/−−−)
}

// NetWorthData is the Synthèse read-model for one month.
type NetWorthData struct {
	Period        string
	Supports      []NWSupport
	Subtotal      int64 // Σ passbook gross (rules §13)
	SubtotalDelta int64
	Total         int64
	TotalDelta    int64
	TotalHasPrev  bool // the focus month is not the earliest recorded month
	NearCap       int  // near-cap threshold in basis points (livret card subtext)
	Empty         bool // no snapshot recorded for this month
	HasAnyHistory bool // at least one snapshot exists in any month
	HasSavings    bool // the user has at least one active savings account

	Comment       string
	Prefill       string       // M25 suggestion when autoprefill is on and the comment is empty
	Movements     []NWMovement // the ranked supports behind the prefill
	AutoprefillOn bool
}

// NetWorthSynthesis assembles the Synthèse for (userID, period): the per-support
// table, the livrets subtotal, the PEA net/total and every delta, plus the
// month comment (and its M25 auto-prefill when enabled). Deltas need the full
// snapshot history, so it loads every snapshot, not just the period's.
func (s *Service) NetWorthSynthesis(ctx context.Context, userID int64, period string) (*NetWorthData, error) {
	if _, _, ok := parsePeriodYM(period); !ok {
		return nil, periodInvalid()
	}
	q := s.tx.DB()
	in, set, snaps, err := s.networthInputs(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	in.Period = period
	nw := in.NetWorth()

	d := &NetWorthData{Period: period, AutoprefillOn: set.CommentAutoprefill, HasAnyHistory: len(snaps) > 0, NearCap: set.NearCapThreshold}
	d.TotalHasPrev = earlierSnapshotExists(snaps, period, 0, false)

	supByAcc := map[int64]engine.NetWorthSupport{}
	for _, sup := range nw.Supports {
		supByAcc[sup.AccountID] = sup
	}

	savings := savingsAccounts(in.Accounts)
	d.HasSavings = len(savings) > 0
	anySnapshot := false
	for _, a := range savings {
		row := NWSupport{AccountID: a.ID, Name: a.Name, Type: a.Type, Ceiling: a.Ceiling}
		if cur, ok := currentSnapshot(snaps, a.ID, period); ok {
			anySnapshot = true
			row.HasSnapshot = true
			row.SnapshotID = cur.ID
			row.Gross = cur.GrossValue
		}
		if sup, ok := supByAcc[a.ID]; ok {
			row.Value = sup.Value
			row.Delta = sup.Delta
		}
		row.HasPrev = earlierSnapshotExists(snaps, period, a.ID, true)
		if prev, ok := previousGross(snaps, a.ID, period); ok {
			row.GrossDelta = row.Gross - prev
		} else {
			row.GrossDelta = row.Gross
		}
		d.Supports = append(d.Supports, row)
		if a.Type == domain.AccountPassbook && row.HasSnapshot {
			d.SubtotalDelta += row.Delta
		}
	}
	d.Empty = !anySnapshot
	d.Subtotal = nw.LivretsSubtotal
	d.Total = nw.Total
	d.TotalDelta = nw.TotalDelta

	m, err := s.networthMos.Get(ctx, q, userID, period)
	switch {
	case err == nil:
		d.Comment = m.Comment
	case errors.Is(err, domain.ErrNotFound):
		// no comment yet
	default:
		return nil, err
	}
	if set.CommentAutoprefill && d.Comment == "" {
		d.Movements = movements(d.Supports)
	}
	return d, nil
}

// --- Registre ---

// RegisterRow is one month line in the history table.
type RegisterRow struct {
	Period     string
	Livrets    int64
	PEANet     int64
	HasPEA     bool
	Total      int64
	TotalDelta int64
	HasPrev    bool
	Comment    string
}

// RegisterSeries is one curve line (a support or the emphasised total).
type RegisterSeries struct {
	AccountID int64 // 0 = the total series
	Name      string
	Type      domain.AccountType
	IsTotal   bool
	Points    []int64 // one value per period in RegisterData.CurvePeriods (0 = absent)
}

// RegisterData is the Registre read-model (full history table + a range-clipped
// evolution curve, M24/D3).
type RegisterData struct {
	Period       string // the focus month (drives the highlighted row)
	Range        string // all | 12 | 6
	Rows         []RegisterRow
	HasHistory   bool
	CurvePeriods []string
	Series       []RegisterSeries
	TotalLatest  int64
}

// Register assembles the Registre: one engine pass per recorded month for the
// full history table, plus a range-clipped multi-series evolution curve. The
// table always shows the full history (M24); only the curve honours the range.
func (s *Service) Register(ctx context.Context, userID int64, rangeKey, period string) (*RegisterData, error) {
	q := s.tx.DB()
	in, _, snaps, err := s.networthInputs(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	comments, err := s.networthMos.ListByUser(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	commentByPeriod := map[string]string{}
	for _, c := range comments {
		commentByPeriod[c.Period] = c.Comment
	}

	periodsAsc := distinctPeriods(snaps)
	d := &RegisterData{Period: period, Range: normaliseRange(rangeKey), HasHistory: len(periodsAsc) > 0}

	// History table — every recorded month, most recent first.
	for i := len(periodsAsc) - 1; i >= 0; i-- {
		p := periodsAsc[i]
		in.Period = p
		nw := in.NetWorth()
		row := RegisterRow{
			Period: p, Livrets: nw.LivretsSubtotal, Total: nw.Total,
			TotalDelta: nw.TotalDelta, HasPrev: i > 0, Comment: commentByPeriod[p],
		}
		if pea, ok := peaSupport(nw); ok {
			row.PEANet, row.HasPEA = pea.Value, true
		}
		d.Rows = append(d.Rows, row)
	}

	// Evolution curve — the range-clipped window, oldest first.
	window := clipRange(periodsAsc, d.Range)
	d.CurvePeriods = window
	if len(window) > 0 {
		total := RegisterSeries{IsTotal: true}
		for _, p := range window {
			in.Period = p
			total.Points = append(total.Points, in.NetWorth().Total)
		}
		d.Series = append(d.Series, total)
		d.TotalLatest = total.Points[len(total.Points)-1]
		for _, a := range savingsAccounts(in.Accounts) {
			if !accountInWindow(snaps, a.ID, window) {
				continue
			}
			ser := RegisterSeries{AccountID: a.ID, Name: a.Name, Type: a.Type}
			for _, p := range window {
				ser.Points = append(ser.Points, supportValueAt(in, a, snaps, p))
			}
			d.Series = append(d.Series, ser)
		}
	}
	return d, nil
}

// --- mutations (L7: no lock guard) ---

// UpsertSnapshot enters or corrects one (account, month) gross value. Always
// allowed regardless of the budget month lock (L7). Validates gross ≥ 0 (422)
// and that the account is the user's savings account (404 cross-tenant).
func (s *Service) UpsertSnapshot(ctx context.Context, userID, accountID int64, period string, gross int64) error {
	if _, _, ok := parsePeriodYM(period); !ok {
		return periodInvalid()
	}
	if gross < 0 {
		v := &domain.ValidationError{}
		v.Add("gross_value", domain.MsgAmountNegative)
		return v
	}
	now := s.now().UTC()
	return s.tx.WithTx(ctx, func(qx repo.DBTX) error {
		acc, err := s.accounts.Get(ctx, qx, userID, accountID)
		if err != nil {
			return err // ErrNotFound ⇒ 404 (also the cross-tenant outcome)
		}
		if !acc.IsSavings() {
			v := &domain.ValidationError{}
			v.Add("account_id", domain.MsgAccountInvalid)
			return v
		}
		return s.snapshots.Upsert(ctx, qx, &domain.Snapshot{
			UserID: userID, AccountID: accountID, Period: period,
			GrossValue: gross, CreatedAt: now, UpdatedAt: now,
		})
	})
}

// DeleteSnapshot removes one snapshot (L7). The following month's delta
// recomputes on the next read (it now compares to the previous remaining
// snapshot). No lock guard.
func (s *Service) DeleteSnapshot(ctx context.Context, userID, id int64) error {
	return s.tx.WithTx(ctx, func(qx repo.DBTX) error {
		return s.snapshots.Delete(ctx, qx, userID, id)
	})
}

// SaveComment upserts the single per-month comment shared by the Synthèse box
// and the Registre cell (M24/B.2). Net-worth comments are never locked (L7).
func (s *Service) SaveComment(ctx context.Context, userID int64, period, comment string) error {
	if _, _, ok := parsePeriodYM(period); !ok {
		return periodInvalid()
	}
	return s.tx.WithTx(ctx, func(qx repo.DBTX) error {
		return s.networthMos.Upsert(ctx, qx, userID, period, comment)
	})
}

// SavingsAccounts lists the user's active savings accounts (for the shared rail's
// Épargne section, O-21). Read-only, no figures.
func (s *Service) SavingsAccounts(ctx context.Context, userID int64) ([]domain.Account, error) {
	accounts, err := s.accounts.ListByUser(ctx, s.tx.DB(), userID)
	if err != nil {
		return nil, err
	}
	return savingsAccounts(accounts), nil
}

// RailAccounts returns the active current accounts (ordered by name) and savings
// accounts (savings-first ordering) for the Patrimoine rail.
func (s *Service) RailAccounts(ctx context.Context, userID int64) (current, savings []domain.Account, err error) {
	accounts, err := s.accounts.ListByUser(ctx, s.tx.DB(), userID)
	if err != nil {
		return nil, nil, err
	}
	for _, a := range accounts {
		if a.Status == domain.ArchiveActive && a.Type == domain.AccountCurrent {
			current = append(current, a)
		}
	}
	sort.SliceStable(current, func(i, j int) bool {
		if current[i].Name != current[j].Name {
			return current[i].Name < current[j].Name
		}
		return current[i].ID < current[j].ID
	})
	return current, savingsAccounts(accounts), nil
}

// --- internals ---

// networthInputs loads the accounts, settings and the FULL snapshot history into
// a base engine.Inputs (Period unset — callers vary it per month). Net worth
// needs no allocations/transactions/start-balances.
func (s *Service) networthInputs(ctx context.Context, q repo.DBTX, userID int64) (engine.Inputs, *domain.Settings, []domain.Snapshot, error) {
	accounts, err := s.accounts.ListByUser(ctx, q, userID)
	if err != nil {
		return engine.Inputs{}, nil, nil, err
	}
	set, err := s.settings.Get(ctx, q, userID)
	if err != nil {
		return engine.Inputs{}, nil, nil, err
	}
	snaps, err := s.snapshots.ListByUser(ctx, q, userID)
	if err != nil {
		return engine.Inputs{}, nil, nil, err
	}
	in := engine.Inputs{
		Accounts:  accounts,
		Snapshots: snaps,
		Params: engine.Params{
			PEAInitialDeposit:   set.PEAInitialDeposit,
			PEASocialChargeRate: set.PEASocialChargeRate,
			NearCapThreshold:    set.NearCapThreshold,
			SecuredSavingsBasis: set.SecuredSavingsBasis,
			Today:               s.today(),
		},
	}
	return in, set, snaps, nil
}

// savingsAccounts returns the active savings accounts ordered savings-first by
// type (passbook → securities → employee_savings) then name, matching the
// snapshot-table / rail ordering.
func savingsAccounts(accounts []domain.Account) []domain.Account {
	var out []domain.Account
	for _, a := range accounts {
		if a.IsSavings() && a.Status == domain.ArchiveActive {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if savingsTypeRank(out[i].Type) != savingsTypeRank(out[j].Type) {
			return savingsTypeRank(out[i].Type) < savingsTypeRank(out[j].Type)
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func savingsTypeRank(t domain.AccountType) int {
	switch t {
	case domain.AccountPassbook:
		return 0
	case domain.AccountSecurities:
		return 1
	case domain.AccountEmployeeSavings:
		return 2
	case domain.AccountCurrent:
		return 3
	default:
		return 4
	}
}

func currentSnapshot(snaps []domain.Snapshot, accID int64, period string) (domain.Snapshot, bool) {
	for _, s := range snaps {
		if s.AccountID == accID && s.Period == period {
			return s, true
		}
	}
	return domain.Snapshot{}, false
}

// earlierSnapshotExists reports whether any snapshot precedes period — globally
// (scoped=false) or for one account (scoped=true).
func earlierSnapshotExists(snaps []domain.Snapshot, period string, accID int64, scoped bool) bool {
	for _, s := range snaps {
		if s.Period >= period {
			continue
		}
		if !scoped || s.AccountID == accID {
			return true
		}
	}
	return false
}

func previousGross(snaps []domain.Snapshot, accID int64, period string) (int64, bool) {
	best, found := "", false
	var gross int64
	for _, s := range snaps {
		if s.AccountID != accID || s.Period >= period {
			continue
		}
		if !found || s.Period > best {
			best, gross, found = s.Period, s.GrossValue, true
		}
	}
	return gross, found
}

func distinctPeriods(snaps []domain.Snapshot) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range snaps {
		if !seen[s.Period] {
			seen[s.Period] = true
			out = append(out, s.Period)
		}
	}
	sort.Strings(out)
	return out
}

func peaSupport(nw engine.NetWorth) (engine.NetWorthSupport, bool) {
	for _, sup := range nw.Supports {
		if sup.Type == domain.AccountSecurities {
			return sup, true
		}
	}
	return engine.NetWorthSupport{}, false
}

// supportValueAt computes one support's contribution for an arbitrary month via
// the engine (pea_net for securities, gross otherwise; 0 when absent).
func supportValueAt(in engine.Inputs, a domain.Account, snaps []domain.Snapshot, period string) int64 {
	cur, ok := currentSnapshot(snaps, a.ID, period)
	if !ok {
		return 0
	}
	if a.Type == domain.AccountSecurities {
		return engine.PEANet(cur.GrossValue, in.Params.PEAInitialDeposit, in.Params.PEASocialChargeRate)
	}
	return cur.GrossValue
}

func accountInWindow(snaps []domain.Snapshot, accID int64, window []string) bool {
	inWin := map[string]bool{}
	for _, p := range window {
		inWin[p] = true
	}
	for _, s := range snaps {
		if s.AccountID == accID && inWin[s.Period] {
			return true
		}
	}
	return false
}

func normaliseRange(r string) string {
	switch r {
	case "12", "6":
		return r
	default:
		return "all"
	}
}

func clipRange(periodsAsc []string, rangeKey string) []string {
	n := len(periodsAsc)
	switch rangeKey {
	case "12":
		if n > 12 {
			return periodsAsc[n-12:]
		}
	case "6":
		if n > 6 {
			return periodsAsc[n-6:]
		}
	}
	return periodsAsc
}

// M25 auto-prefill bands (I-036, user-chosen at D4): a movement is listed when
// |Δ| ≥ 100 € and graded by absolute magnitude — [100,300) € → +, [300,750) € →
// ++, ≥ 750 € → +++ (same for losses). Minor units.
const (
	movFloor = 10000 // 100 € — below this a movement is not listed
	movBand2 = 30000 // 300 €
	movBand3 = 75000 // 750 €
)

// movements ranks the supports that moved this month for the M25 comment
// auto-prefill (I-036): each support's month-over-month Δ, filtered to ≥ 100 €,
// graded by the absolute bands above, largest first.
func movements(supports []NWSupport) []NWMovement {
	type mv struct {
		name string
		mag  int64
		up   bool
	}
	var moved []mv
	for _, sup := range supports {
		if !sup.HasPrev || sup.Delta == 0 {
			continue
		}
		mag := sup.Delta
		up := mag > 0
		if mag < 0 {
			mag = -mag
		}
		if mag < movFloor {
			continue // below 100 € — too small to surface
		}
		moved = append(moved, mv{name: sup.Name, mag: mag, up: up})
	}
	if len(moved) == 0 {
		return nil
	}
	sort.SliceStable(moved, func(i, j int) bool { return moved[i].mag > moved[j].mag })
	out := make([]NWMovement, 0, len(moved))
	for _, m := range moved {
		intensity := 1
		switch {
		case m.mag >= movBand3:
			intensity = 3
		case m.mag >= movBand2:
			intensity = 2
		}
		out = append(out, NWMovement{Name: m.name, Up: m.up, Intensity: intensity})
	}
	return out
}

func periodInvalid() error {
	v := &domain.ValidationError{}
	v.Add("period", domain.MsgPeriodInvalid)
	return v
}
