package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"econome/internal/domain"
	"econome/internal/engine"
)

// Forecast read-model (functional/05, increment 6a). Read-only: it assembles the
// engine's per-month picture for the shared month + account scope into a
// projection the view formats. Nothing is persisted; every figure is the pure
// engine's output (derived-not-stored). The three scope variants (aggregated /
// sweep / carry) render one at a time — only the selected scope is computed.

// ScopeAll is the rail's aggregated scope key.
const ScopeAll = "all"

// Scope kinds (which right-panel + timeline variant a scope renders).
const (
	scopeKindAggregated = "aggregated"
	scopeKindSweep      = "sweep"
	scopeKindCarry      = "carry"
)

// ForecastTxn is one read-only transaction line in a leaf's drill-down (D2).
type ForecastTxn struct {
	Label   string
	DateStr string // "12/06" · "~28/06" (awaited, approximate) · "—"
	Approx  bool
	Amount  int64 // signed minor units (display +/−)
	Status  domain.TransactionStatus
}

// ForecastRow is one envelope line (leaf) or a rolled-up parent category (M2).
type ForecastRow struct {
	Key         string // stable expand key ("e<id>" leaf · "c<id>" parent)
	EnvelopeID  int64
	Name        string
	AccountName string // shown as a pill in the aggregated scope
	ShowPill    bool
	Flow        domain.FlowType
	Planned     int64
	Real        int64
	Remaining   int64
	Percent     int
	State       domain.EnvelopeState // expenses only
	Income      bool                 // income row: received-vs-expected, no overrun red
	HasBar      bool                 // variable expense with planned > 0 (progress bar)
	IsParent    bool                 // rolled-up category over its children (badge "agrégé")
	Children    []ForecastRow
	Drill       []ForecastTxn // leaf only
}

// ForecastTotals is the footer total over expense envelopes in scope only.
type ForecastTotals struct {
	Planned   int64
	Real      int64
	Remaining int64
}

// ForecastFigures is the right-panel figure set (content varies by scope, M16).
type ForecastFigures struct {
	BalanceCleared  int64 // cleared_balance
	InProgress      int64 // pending (subtext "dont en attente X")
	BalanceReal     int64 // real_balance (after in-progress)
	Income          int64 // received period income (sweep/aggregated)
	IncomingXfer    int64 // funding transfer in (carry)
	ProjectedEnd    int64 // carried close (carry)
	LowPoint        int64
	LowBreaches     bool
	LowAccountName  string // aggregated: the worst single account
	HasIncome       bool
	HasIncomingXfer bool
	HasProjectedEnd bool
	HasLowPoint     bool
}

// ForecastEncart is a sweep account's savings band (residual / negative / cascade).
type ForecastEncart struct {
	Kind        string // residual | negative | cascade
	AccountName string
	TargetName  string
	Secured     int64 // prudent figure
	Projected   int64 // if-the-budget-holds figure
}

// ForecastWatch is one "à surveiller" item.
type ForecastWatch struct {
	Kind   string // overrun | awaited | remainder
	Label  string
	Amount int64
}

// TimelinePoint is one ordered cash-flow point for the treasury SVG.
type TimelinePoint struct {
	Day     int
	Balance int64
	Kind    string // start | income | debit | awaited | overrun | low
}

// ForecastTimeline is the per-account treasury cash-flow curve (M17, rules §11.2).
type ForecastTimeline struct {
	AccountName    string
	CriticalSuffix string // aggregated: worst account name
	DaysInMonth    int
	Points         []TimelinePoint
	LowValue       int64
	LowDay         int
	LowBreaches    bool
	IsCarry        bool // label the end marker "fin de mois" not "point bas"
	IncomeArrival  int64
	HasArrival     bool
}

// ForecastData is the full read-model for one (period, scope).
type ForecastData struct {
	Period    string
	Exists    bool
	Locked    bool
	Empty     bool // created but no rows in scope
	Scope     string
	ScopeKind string
	Accounts  []domain.Account // active accounts (rail)
	SweepIDs  []int64

	Rows               []ForecastRow
	HasHiddenTransfers bool // aggregated: internal transfers masked
	Total              ForecastTotals

	Figures   ForecastFigures
	Encarts   []ForecastEncart
	CarryNote bool
	Watch     []ForecastWatch
	Timeline  *ForecastTimeline
}

// Forecast assembles the read-model for (userID, period, scope). scope is
// ScopeAll or a current-account id string. A not-created period yields
// Exists=false (the landing/empty state); the engine is otherwise the single
// source of every figure.
func (s *Service) Forecast(ctx context.Context, userID int64, period, scope string) (*ForecastData, error) {
	if _, _, ok := parsePeriodYM(period); !ok {
		v := &domain.ValidationError{}
		v.Add("period", domain.MsgPeriodInvalid)
		return nil, v
	}
	q := s.tx.DB()

	d := &ForecastData{Period: period, Scope: scope}

	prow, err := s.periods.ByPeriod(ctx, q, userID, period)
	switch {
	case err == nil:
		d.Exists = true
		d.Locked = prow.State == domain.PeriodLocked
	case errors.Is(err, domain.ErrNotFound):
		d.Exists = false
	default:
		return nil, err
	}

	in, err := s.engineInputs(ctx, q, userID, period)
	if err != nil {
		return nil, err
	}
	for _, a := range in.Accounts {
		if a.Status == domain.ArchiveActive {
			d.Accounts = append(d.Accounts, a)
		}
	}
	d.SweepIDs = in.SweepAccounts()

	if !d.Exists {
		return d, nil // landing/not-created state; nothing more to compute
	}

	// Which current accounts are in scope.
	inScope := s.scopeAccounts(d.Accounts, scope)
	d.ScopeKind = s.scopeKind(d.Accounts, scope)

	d.Rows = s.forecastRows(in, inScope, d.ScopeKind == scopeKindAggregated)
	d.Total = expenseTotal(d.Rows)
	d.HasHiddenTransfers = d.ScopeKind == scopeKindAggregated && hasInternalTransfers(in, inScope)
	d.Empty = len(d.Rows) == 0

	d.Figures = s.figures(in, inScope, d.ScopeKind)
	d.Encarts = s.encarts(in, inScope, d.ScopeKind)
	d.CarryNote = d.ScopeKind == scopeKindCarry
	d.Watch = watchItems(d.Rows)
	d.Timeline = s.timeline(in, inScope, d.ScopeKind)

	return d, nil
}

// scopeAccounts returns the current accounts a scope covers (aggregated = all
// active current accounts; otherwise the single matching account).
func (s *Service) scopeAccounts(accounts []domain.Account, scope string) []domain.Account {
	var out []domain.Account
	for _, a := range accounts {
		if a.Type != domain.AccountCurrent {
			continue
		}
		if scope == ScopeAll || idStr(a.ID) == scope {
			out = append(out, a)
		}
	}
	return out
}

// scopeKind classifies the selected scope for the right panel + timeline.
func (s *Service) scopeKind(accounts []domain.Account, scope string) string {
	if scope == ScopeAll {
		return scopeKindAggregated
	}
	for _, a := range accounts {
		if a.Type == domain.AccountCurrent && idStr(a.ID) == scope {
			if a.MonthEndPolicy == domain.PolicySweep {
				return scopeKindSweep
			}
			return scopeKindCarry
		}
	}
	return scopeKindAggregated
}

// forecastRows builds the envelope table for the scope. Per single account it is
// the category hierarchy (parents roll up their children, M2); aggregated is a
// flat list with account pills (functional/05 §4c). Transfer and residual
// envelopes are excluded from rows (rules §10).
func (s *Service) forecastRows(in engine.Inputs, scopeAccts []domain.Account, aggregated bool) []ForecastRow {
	inScope := map[int64]bool{}
	for _, a := range scopeAccts {
		inScope[a.ID] = true
	}
	acctName := map[int64]string{}
	for _, a := range in.Accounts {
		acctName[a.ID] = a.Name
	}
	catByID := map[int64]domain.Category{}
	for _, c := range in.Categories {
		catByID[c.ID] = c
	}

	// Collect leaf envelopes (one per (category, account)) in scope.
	type leaf struct {
		env domain.Envelope
		cat domain.Category
		v   engine.EnvelopeView
	}
	var leaves []leaf
	for _, e := range in.Envelopes {
		if e.Status != domain.ArchiveActive || !inScope[e.AccountID] {
			continue
		}
		cat, ok := catByID[e.CategoryID]
		if !ok {
			continue
		}
		if cat.FlowType == domain.FlowTransfer || e.Mode == domain.ModeResidual {
			continue // transfers/residual are not budget rows
		}
		leaves = append(leaves, leaf{env: e, cat: cat, v: in.EnvelopeView(e.ID)})
	}

	mkLeaf := func(l leaf) ForecastRow {
		row := ForecastRow{
			Key:         "e" + idStr(l.env.ID),
			EnvelopeID:  l.env.ID,
			Name:        l.cat.Name,
			AccountName: acctName[l.env.AccountID],
			Flow:        l.v.Flow,
			Planned:     l.v.Planned,
			Real:        l.v.Real,
			Remaining:   l.v.Remaining,
			Percent:     l.v.Percent,
			State:       l.v.State,
			Income:      l.v.Flow == domain.FlowIncome,
			HasBar:      l.env.Mode == domain.ModeVariable && l.v.Planned > 0 && l.v.Flow == domain.FlowExpense,
		}
		row.Drill = drillTxns(in, l.env)
		return row
	}

	if aggregated {
		var rows []ForecastRow
		for _, l := range leaves {
			r := mkLeaf(l)
			r.ShowPill = true
			r.Drill = nil // aggregated rows are flat, no drill-down
			rows = append(rows, r)
		}
		sortRows(rows)
		return rows
	}

	// Single-account scope: build the category hierarchy.
	var topLevel []ForecastRow
	groups := map[int64][]leaf{} // parent category id -> child leaves
	var parentOrder []int64
	for _, l := range leaves {
		if l.cat.ParentID == nil {
			topLevel = append(topLevel, mkLeaf(l))
			continue
		}
		pid := *l.cat.ParentID
		if _, seen := groups[pid]; !seen {
			parentOrder = append(parentOrder, pid)
		}
		groups[pid] = append(groups[pid], l)
	}

	var rows []ForecastRow
	rows = append(rows, topLevel...)
	for _, pid := range parentOrder {
		pc := catByID[pid]
		var children []ForecastRow
		for _, l := range groups[pid] {
			children = append(children, mkLeaf(l))
		}
		sortRows(children)
		rows = append(rows, rollupParent(pc, children))
	}
	sortRows(rows)
	return rows
}

// rollupParent builds a parent category row from its children: exact integer
// sums and the most-severe child state (overrun > partial > expected > paid >
// none), badge marked "agrégé" (functional/05 §2 / M2).
func rollupParent(pc domain.Category, children []ForecastRow) ForecastRow {
	p := ForecastRow{
		Key:      "c" + idStr(pc.ID),
		Name:     pc.Name,
		Flow:     domain.FlowExpense,
		IsParent: true,
		Children: children,
	}
	for _, c := range children {
		p.Planned += c.Planned
		p.Real += c.Real
		p.State = moreSevere(p.State, c.State)
	}
	p.Remaining = p.Planned - p.Real
	if p.Planned > 0 {
		p.Percent = int(engine.RoundHalfEvenDiv(100*p.Real, p.Planned))
	}
	return p
}

var stateSeverity = map[domain.EnvelopeState]int{
	domain.StateNone:     0,
	domain.StatePaid:     1,
	domain.StateExpected: 2,
	domain.StatePartial:  3,
	domain.StateOverrun:  4,
}

func moreSevere(a, b domain.EnvelopeState) domain.EnvelopeState {
	if stateSeverity[b] > stateSeverity[a] {
		return b
	}
	return a
}

// drillTxns lists the leaf envelope's transactions for the period (read-only D2).
func drillTxns(in engine.Inputs, e domain.Envelope) []ForecastTxn {
	var out []ForecastTxn
	for _, t := range in.Txns {
		if t.BudgetPeriod != in.Period || t.CategoryID == nil || *t.CategoryID != e.CategoryID || t.AccountID != e.AccountID {
			continue
		}
		out = append(out, ForecastTxn{
			Label:   t.Label,
			DateStr: txnDateStr(t, e),
			Approx:  t.Status == domain.StatusAwaited,
			Amount:  t.Amount,
			Status:  t.Status,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// txnDateStr renders a transaction's drill-down date: "DD/MM" when dated;
// "~DD/MM" for an awaited fixed-recurring (its expected day); "—" otherwise.
func txnDateStr(t domain.Transaction, e domain.Envelope) string {
	if t.OpDate != nil {
		return ddmm(t.OpDate.Day, t.OpDate.Month)
	}
	if t.Status == domain.StatusAwaited && e.Mode == domain.ModeFixedRecurring && e.ExpectedDay != nil {
		if _, m, ok := parsePeriodYM(t.BudgetPeriod); ok {
			return "~" + ddmm(*e.ExpectedDay, m)
		}
	}
	return "—"
}

// expenseTotal sums planned/real/remaining over expense rows in scope (footer).
func expenseTotal(rows []ForecastRow) ForecastTotals {
	var tot ForecastTotals
	for _, r := range rows {
		if r.Flow != domain.FlowExpense {
			continue
		}
		tot.Planned += r.Planned
		tot.Real += r.Real
		tot.Remaining += r.Remaining
	}
	return tot
}

// figures builds the right-panel figure set for the scope (M16).
func (s *Service) figures(in engine.Inputs, scopeAccts []domain.Account, kind string) ForecastFigures {
	var f ForecastFigures
	for _, a := range scopeAccts {
		bal := in.AccountBalances(a.ID)
		f.BalanceCleared += bal.ClearedBalance
		f.BalanceReal += bal.RealBalance
		f.InProgress += bal.InProgress
	}
	switch kind {
	case scopeKindCarry:
		if len(scopeAccts) == 1 {
			a := scopeAccts[0]
			f.ProjectedEnd = in.AccountBalances(a.ID).ProjectedEnd
			f.HasProjectedEnd = true
			f.IncomingXfer = incomingTransfer(in, a.ID)
			f.HasIncomingXfer = true
		}
	case scopeKindSweep:
		a := scopeAccts[0]
		f.Income = receivedIncome(in, a.ID)
		f.HasIncome = true
		lp := in.LowPoint(a.ID)
		f.LowPoint, f.LowBreaches, f.HasLowPoint = lp.Value, lp.BreachesZero, true
	default: // aggregated
		for _, a := range scopeAccts {
			f.Income += receivedIncome(in, a.ID)
		}
		f.HasIncome = true
		ids := currentIDs(scopeAccts)
		if len(ids) > 0 {
			lp, acc := in.AggregateLowPoint(ids)
			f.LowPoint, f.LowBreaches, f.HasLowPoint = lp.Value, lp.BreachesZero, true
			if a, ok := in.AccountByID(acc); ok {
				f.LowAccountName = a.Name
			}
		}
	}
	return f
}

// encarts builds the savings band(s): one per in-scope sweep account (rules §14).
func (s *Service) encarts(in engine.Inputs, scopeAccts []domain.Account, kind string) []ForecastEncart {
	if kind == scopeKindCarry {
		return nil
	}
	acctName := map[int64]string{}
	for _, a := range in.Accounts {
		acctName[a.ID] = a.Name
	}
	var out []ForecastEncart
	for _, a := range scopeAccts {
		if a.MonthEndPolicy != domain.PolicySweep {
			continue
		}
		sv := in.Savings(a.ID)
		e := ForecastEncart{AccountName: a.Name, Secured: sv.Secured, Projected: sv.Projected}
		switch {
		case sv.CascadeFull:
			e.Kind = "cascade"
		case sv.ResidualNegative:
			e.Kind = "negative"
		default:
			e.Kind = "residual"
			if sv.CascadeTargetID != nil {
				e.TargetName = acctName[*sv.CascadeTargetID]
			}
		}
		out = append(out, e)
	}
	return out
}

// watchItems surfaces overruns, awaited-soon items and notable remainders from
// the in-scope leaf rows (functional/05 §4 "à surveiller"), capped to a few.
func watchItems(rows []ForecastRow) []ForecastWatch {
	var leaves []ForecastRow
	var walk func([]ForecastRow)
	walk = func(rs []ForecastRow) {
		for _, r := range rs {
			if r.IsParent {
				walk(r.Children)
				continue
			}
			if r.Flow == domain.FlowExpense {
				leaves = append(leaves, r)
			}
		}
	}
	walk(rows)

	var watch []ForecastWatch
	for _, r := range leaves {
		if r.State == domain.StateOverrun && r.Remaining < 0 {
			watch = append(watch, ForecastWatch{Kind: "overrun", Label: r.Name, Amount: -r.Remaining})
		}
	}
	for _, r := range leaves {
		if r.State == domain.StateExpected {
			watch = append(watch, ForecastWatch{Kind: "awaited", Label: r.Name, Amount: r.Planned})
		}
	}
	// Notable remainder: the single largest positive remaining among partials.
	var best *ForecastRow
	for i := range leaves {
		if leaves[i].State == domain.StatePartial && leaves[i].Remaining > 0 {
			if best == nil || leaves[i].Remaining > best.Remaining {
				best = &leaves[i]
			}
		}
	}
	if best != nil {
		watch = append(watch, ForecastWatch{Kind: "remainder", Label: best.Name, Amount: best.Remaining})
	}
	if len(watch) > 5 {
		watch = watch[:5]
	}
	return watch
}

// timeline builds the treasury cash-flow curve for the scope's account (sweep/
// carry → that account; aggregated → the critical/worst account).
func (s *Service) timeline(in engine.Inputs, scopeAccts []domain.Account, kind string) *ForecastTimeline {
	var accID int64
	var critical string
	isCarry := kind == scopeKindCarry
	switch kind {
	case scopeKindAggregated:
		ids := currentIDs(scopeAccts)
		if len(ids) == 0 {
			return nil
		}
		_, worst := in.AggregateLowPoint(ids)
		accID = worst
		if a, ok := in.AccountByID(accID); ok {
			critical = a.Name
			isCarry = a.MonthEndPolicy == domain.PolicyCarry
		}
	default:
		if len(scopeAccts) == 0 {
			return nil
		}
		accID = scopeAccts[0].ID
	}

	lp := in.LowPoint(accID)
	_, dim, _ := monthBounds(in.Period)
	tl := &ForecastTimeline{
		DaysInMonth:    dim,
		LowValue:       lp.Value,
		LowDay:         clampDay(lp.AtDate.Day, dim),
		LowBreaches:    lp.BreachesZero,
		IsCarry:        isCarry,
		CriticalSuffix: critical,
	}
	if a, ok := in.AccountByID(accID); ok {
		tl.AccountName = a.Name
	}
	for i, p := range lp.OrderedPoints {
		kind := "debit"
		switch {
		case i == 0:
			kind = "start"
		case p.Balance < 0:
			kind = "overrun"
		case i > 0 && p.Balance > lp.OrderedPoints[i-1].Balance:
			kind = "income"
		}
		if i == 0 {
			tl.IncomeArrival = receivedIncome(in, accID)
			tl.HasArrival = tl.IncomeArrival > 0
		}
		tl.Points = append(tl.Points, TimelinePoint{Day: clampDay(p.Date.Day, dim), Balance: p.Balance, Kind: kind})
	}
	return tl
}

// --- small derivations ---

// receivedIncome is the cleared+pending income recognised on an account this
// period (the "Revenus du mois" figure). It reuses the engine's per-envelope
// view so the figure matches the table.
func receivedIncome(in engine.Inputs, accID int64) int64 {
	var total int64
	for _, e := range in.Envelopes {
		if e.AccountID != accID || in.EnvelopeFlow(e) != domain.FlowIncome {
			continue
		}
		total += in.EnvelopeView(e.ID).Real
	}
	return total
}

// incomingTransfer is the total transfer inflow funding an account (all statuses
// — the projected incoming virement, functional/05 §4b).
func incomingTransfer(in engine.Inputs, accID int64) int64 {
	var total int64
	for _, t := range in.Txns {
		if t.BudgetPeriod != in.Period || t.FlowType != domain.FlowTransfer {
			continue
		}
		if t.DestAccountID != nil && *t.DestAccountID == accID {
			total += -t.Amount // inflow (amount is source-signed)
		}
	}
	return total
}

// hasInternalTransfers reports whether any internal transfer touches the scope
// (drives the aggregated masked-internal-transfers footer).
func hasInternalTransfers(in engine.Inputs, scopeAccts []domain.Account) bool {
	inScope := map[int64]bool{}
	for _, a := range scopeAccts {
		inScope[a.ID] = true
	}
	for _, t := range in.Txns {
		if t.BudgetPeriod != in.Period || t.FlowType != domain.FlowTransfer {
			continue
		}
		if inScope[t.AccountID] || (t.DestAccountID != nil && inScope[*t.DestAccountID]) {
			return true
		}
	}
	return false
}

func currentIDs(accts []domain.Account) []int64 {
	ids := make([]int64, 0, len(accts))
	for _, a := range accts {
		ids = append(ids, a.ID)
	}
	return ids
}

// sortRows orders rows by name then envelope id for a stable, readable table.
func sortRows(rows []ForecastRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].EnvelopeID < rows[j].EnvelopeID
	})
}

// monthBounds returns the year, days-in-month and ok for a "YYYY-MM" period.
func monthBounds(period string) (year, dim int, ok bool) {
	y, m, valid := parsePeriodYM(period)
	if !valid {
		return 0, 0, false
	}
	return y, daysIn(y, m), true
}

func daysIn(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	default:
		return 30
	}
}

func clampDay(day, dim int) int {
	if day < 1 {
		return 1
	}
	if day > dim {
		return dim
	}
	return day
}

func ddmm(day, month int) string {
	return fmt.Sprintf("%02d/%02d", day, month)
}

func idStr(id int64) string {
	return strconv.FormatInt(id, 10)
}
