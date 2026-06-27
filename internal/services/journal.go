package services

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"econome/internal/domain"
	"econome/internal/repo"
)

// Journal use-cases (functional/06, functional/04 §3.5). The journal feeds all
// actuals: quick-entry create, whole-cell inline edit, server-side sort/filter,
// the month summary, and atomic delete. Every figure is derived on read
// (derived-not-stored); only transaction rows are written. The single-row
// date↔status reconciliation (§7) lives here; the engine.Reconcile matching
// orchestration + label autocomplete land in 6d.

// JournalSortDefault / dir are the M19 default (date descending).
const (
	journalSortDate = "date"
	dirAsc          = "asc"
	dirDesc         = "desc"
)

// TxnInput is the quick-entry / create payload (already parsed to minor units +
// a clock-free Date at the HTTP boundary).
type TxnInput struct {
	OpDate        *domain.Date
	Label         string
	CategoryID    *int64
	AccountID     int64
	DestAccountID *int64
	Magnitude     int64 // unsigned amount; the sign comes from the flow
	Status        domain.TransactionStatus
	FlowType      domain.FlowType // transfer ⇒ no category; else derived from the category
	BudgetPeriod  string          // explicit, else derived from OpDate
}

// JournalFilters are the view-only right-panel filters (M18) — they never mutate.
type JournalFilters struct {
	Q                string
	CategoryID       *int64
	Statuses         []domain.TransactionStatus // empty ⇒ all
	IncludeTransfers bool
	Scope            string // ScopeAll or an account id
}

// JournalRow is one transaction joined with its display names.
type JournalRow struct {
	Txn          domain.Transaction
	CategoryName string
	AccountName  string
	DestName     string // transfers
	Flow         domain.FlowType
	IsTransfer   bool
	ExpectedDay  *int // a recurring awaited row's forecast day (for the ~DD/MM display)
}

// JournalSummary is the right-panel month summary (M18, transfers excluded).
type JournalSummary struct {
	IncomeReceived int64
	RealExpenses   int64 // cleared + pending (C7)
	Pending        int64
	PendingCount   int
	Awaited        int64
	AwaitedCount   int
	NetBalance     int64
}

// JournalData is the read-model for one (period, scope, sort, filters).
type JournalData struct {
	Period     string
	Exists     bool
	Locked     bool
	Editable   bool
	Scope      string
	Sort       string
	Dir        string
	Accounts   []domain.Account  // active current accounts (rail + selectors)
	Categories []domain.Category // leaf categories (selectors + pill labels)
	CatAccount map[int64]int64   // category → prefill account (its envelope's account)
	Rows       []JournalRow      // scope + view-filtered + sorted
	Summary    JournalSummary    // scope-filtered, view-filters ignored
}

// Journal assembles the read-model: the scope+filter+sort applied to the
// period's transactions, plus the (scope-only) month summary.
func (s *Service) Journal(ctx context.Context, userID int64, period, scope, srt, dir string, f JournalFilters) (*JournalData, error) {
	if _, _, ok := parsePeriodYM(period); !ok {
		v := &domain.ValidationError{}
		v.Add("period", domain.MsgPeriodInvalid)
		return nil, v
	}
	q := s.tx.DB()
	d := &JournalData{Period: period, Scope: scope, Sort: normaliseSort(srt), Dir: normaliseDir(dir, srt)}

	if p, err := s.periods.ByPeriod(ctx, q, userID, period); err == nil {
		d.Exists, d.Locked = true, p.State == domain.PeriodLocked
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	d.Editable = d.Exists && !d.Locked

	accounts, err := s.accounts.ListByUser(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	acctName := map[int64]string{}
	for _, a := range accounts {
		acctName[a.ID] = a.Name
		if a.Type == domain.AccountCurrent && a.Status == domain.ArchiveActive {
			d.Accounts = append(d.Accounts, a)
		}
	}
	categories, err := s.categories.ListByUser(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	catName := map[int64]string{}
	for _, c := range categories {
		catName[c.ID] = c.Name
		if isLeafCategory(c, categories) {
			d.Categories = append(d.Categories, c)
		}
	}
	d.CatAccount = s.categoryAccounts(ctx, q, userID)
	if !d.Exists {
		return d, nil
	}
	txns, err := s.transactions.ListByPeriod(ctx, q, userID, period)
	if err != nil {
		return nil, err
	}
	expectedDay := s.expectedDayMap(ctx, q, userID)

	for _, t := range txns {
		row := JournalRow{Txn: t, AccountName: acctName[t.AccountID], Flow: t.FlowType, IsTransfer: t.FlowType == domain.FlowTransfer}
		if t.CategoryID != nil {
			row.CategoryName = catName[*t.CategoryID]
			if t.Status == domain.StatusAwaited && t.OpDate == nil {
				if d, ok := expectedDay[envKey{*t.CategoryID, t.AccountID}]; ok {
					row.ExpectedDay = d
				}
			}
		}
		if t.DestAccountID != nil {
			row.DestName = acctName[*t.DestAccountID]
		}
		// Summary: scope-filtered only (view filters never alter it), transfers excluded.
		if inScope(t, scope) {
			accumulateSummary(&d.Summary, t)
		}
		if passesFilter(row, scope, f) {
			d.Rows = append(d.Rows, row)
		}
	}
	d.Summary.NetBalance = d.Summary.IncomeReceived - d.Summary.RealExpenses
	sortRowsBy(d.Rows, d.Sort, d.Dir)
	return d, nil
}

// CreateTransaction appends a manual transaction (quick-entry, M20). It enforces
// the locked-month guard, validates, derives the budget period from the date,
// signs the amount by flow (expense/transfer negative, income positive, I-031),
// and prefills the account from the category when none was chosen.
func (s *Service) CreateTransaction(ctx context.Context, userID int64, in TxnInput) (*domain.Transaction, error) {
	now := s.now().UTC()
	in.BudgetPeriod = budgetPeriodFor(in)
	if err := validateTxn(in); err != nil {
		return nil, err
	}
	var created *domain.Transaction
	err := s.tx.WithTx(ctx, func(q repo.DBTX) error {
		if err := s.ensureEditable(ctx, q, userID, in.BudgetPeriod); err != nil {
			return err
		}
		acctID := in.AccountID
		var catID *int64
		flow := in.FlowType
		if flow == domain.FlowTransfer {
			catID = nil
		} else {
			catID = in.CategoryID
			c, err := s.categories.Get(ctx, q, userID, *in.CategoryID)
			if err != nil {
				return err
			}
			flow = c.FlowType
			if acctID == 0 {
				if a, ok := s.accountForCategory(ctx, q, userID, *in.CategoryID); ok {
					acctID = a
				}
			}
		}
		t := &domain.Transaction{
			UserID: userID, AccountID: acctID, DestAccountID: in.DestAccountID, CategoryID: catID,
			FlowType: flow, Amount: signedAmount(flow, in.Magnitude), OpDate: in.OpDate,
			BudgetPeriod: in.BudgetPeriod, Status: in.Status, Label: strings.TrimSpace(in.Label),
			Source: domain.SourceManual, CreatedAt: now, UpdatedAt: now,
		}
		id, err := s.transactions.Create(ctx, q, t)
		if err != nil {
			return err
		}
		t.ID = id
		created = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// UpdateTransaction applies a single inline cell edit (M22). field ∈ op_date |
// budget_period | label | category_id | account_id | status | amount. It guards
// the source and (on a period change) the target period, enforces the transfer
// inline scope (M23), and keeps date↔status consistent (§4).
func (s *Service) UpdateTransaction(ctx context.Context, userID, id int64, field, value string) error {
	now := s.now().UTC()
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		t, err := s.transactions.Get(ctx, q, userID, id)
		if err != nil {
			return err
		}
		if err := s.ensureEditable(ctx, q, userID, t.BudgetPeriod); err != nil {
			return err
		}
		if t.FlowType == domain.FlowTransfer && (field == "category_id" || field == "account_id") {
			return domain.ErrConflict // transfer accounts/category are fixed (M23)
		}
		if err := s.applyTxnField(ctx, q, userID, t, field, value); err != nil {
			return err
		}
		if field == "budget_period" {
			if err := s.ensureEditable(ctx, q, userID, t.BudgetPeriod); err != nil {
				return err // the new (target) period must be editable too
			}
		}
		t.UpdatedAt = now
		return s.transactions.Update(ctx, q, t)
	})
}

// DeleteTransaction removes a transaction (open month). A manual transfer is a
// single two-legged row, so deleting it removes both balance legs (L8); the
// auto-paired multi-row case is import (6d).
func (s *Service) DeleteTransaction(ctx context.Context, userID, id int64) error {
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		t, err := s.transactions.Get(ctx, q, userID, id)
		if err != nil {
			return err
		}
		if err := s.ensureEditable(ctx, q, userID, t.BudgetPeriod); err != nil {
			return err
		}
		return s.transactions.Delete(ctx, q, userID, id)
	})
}

// applyTxnField mutates exactly one field on t, applying the date↔status and
// amount-sign rules.
func (s *Service) applyTxnField(ctx context.Context, q repo.DBTX, userID int64, t *domain.Transaction, field, value string) error {
	switch field {
	case "label":
		t.Label = strings.TrimSpace(value)
	case "status":
		st := domain.TransactionStatus(value)
		if !st.Valid() {
			return validationErr("status", domain.MsgStatusInvalid)
		}
		t.Status = st
	case "op_date":
		if strings.TrimSpace(value) == "" {
			t.OpDate = nil
			if t.Status == domain.StatusCleared || t.Status == domain.StatusPending {
				t.Status = domain.StatusAwaited // clearing the date reverts to awaited (§4)
			}
		} else {
			dt, err := parseDayMonth(value, t.BudgetPeriod)
			if err != nil {
				return validationErr("op_date", domain.MsgDateInvalid)
			}
			t.OpDate = &dt
			if t.Status == domain.StatusAwaited {
				t.Status = domain.StatusCleared // filling the date reconciles to cleared (§4)
			}
		}
	case "budget_period":
		if _, _, ok := parsePeriodYM(value); !ok {
			return validationErr("budget_period", domain.MsgPeriodInvalid)
		}
		t.BudgetPeriod = value
	case "amount":
		mag := parseMagnitude(value)
		if mag <= 0 {
			return validationErr("amount", domain.MsgAmountInvalid)
		}
		t.Amount = signedAmount(t.FlowType, mag)
	case "category_id":
		cid, err := parseID(value)
		if err != nil {
			return validationErr("category_id", domain.MsgCategoryInvalid)
		}
		c, err := s.categories.Get(ctx, q, userID, cid)
		if err != nil {
			return err
		}
		t.CategoryID = &cid
		t.FlowType = c.FlowType
		t.Amount = signedAmount(c.FlowType, magnitudeOf(t.Amount)) // re-sign on a flow change
	case "account_id":
		aid, err := parseID(value)
		if err != nil {
			return validationErr("account_id", domain.MsgAccountInvalid)
		}
		if _, err := s.accounts.Get(ctx, q, userID, aid); err != nil {
			return err
		}
		t.AccountID = aid
	default:
		return validationErr("field", domain.MsgFieldInvalid)
	}
	return nil
}

func validateTxn(in TxnInput) error {
	v := &domain.ValidationError{}
	if in.Magnitude <= 0 {
		v.Add("amount", domain.MsgAmountInvalid)
	}
	if in.FlowType == domain.FlowTransfer {
		if in.DestAccountID == nil || in.AccountID == 0 || *in.DestAccountID == in.AccountID {
			v.Add("dest_account_id", domain.MsgDestSameAccount)
		}
	} else if in.CategoryID == nil {
		v.Add("category_id", domain.MsgCategoryRequired)
	}
	if !in.Status.Valid() {
		v.Add("status", domain.MsgStatusInvalid)
	}
	if _, _, ok := parsePeriodYM(in.BudgetPeriod); !ok {
		v.Add("budget_period", domain.MsgPeriodInvalid)
	}
	return v.OrNil()
}

// accountForCategory returns the account to prefill for a category: the account
// of its (single) envelope, or the most-used one when the category pairs with
// several accounts (simplified to the first active envelope for now).
func (s *Service) accountForCategory(ctx context.Context, q repo.DBTX, userID, categoryID int64) (int64, bool) {
	envs, err := s.envelopes.ListByUser(ctx, q, userID)
	if err != nil {
		return 0, false
	}
	for _, e := range envs {
		if e.CategoryID == categoryID && e.Status == domain.ArchiveActive {
			return e.AccountID, true
		}
	}
	return 0, false
}

// --- helpers ---

type envKey struct {
	categoryID, accountID int64
}

// expectedDayMap maps (category, account) → a fixed-recurring envelope's
// expected_day, for rendering an undated awaited row's forecast date ~DD/MM.
func (s *Service) expectedDayMap(ctx context.Context, q repo.DBTX, userID int64) map[envKey]*int {
	out := map[envKey]*int{}
	envs, err := s.envelopes.ListByUser(ctx, q, userID)
	if err != nil {
		return out
	}
	for _, e := range envs {
		if e.Mode == domain.ModeFixedRecurring && e.ExpectedDay != nil {
			out[envKey{e.CategoryID, e.AccountID}] = e.ExpectedDay
		}
	}
	return out
}

// categoryAccounts maps each category to its prefill account (the first active
// envelope's account) for the quick-entry category→account prefill.
func (s *Service) categoryAccounts(ctx context.Context, q repo.DBTX, userID int64) map[int64]int64 {
	out := map[int64]int64{}
	envs, err := s.envelopes.ListByUser(ctx, q, userID)
	if err != nil {
		return out
	}
	for _, e := range envs {
		if e.Status != domain.ArchiveActive {
			continue
		}
		if _, seen := out[e.CategoryID]; !seen {
			out[e.CategoryID] = e.AccountID
		}
	}
	return out
}

func isLeafCategory(c domain.Category, all []domain.Category) bool {
	for _, o := range all {
		if o.ParentID != nil && *o.ParentID == c.ID {
			return false // has children → not a leaf (budget posts at leaves)
		}
	}
	return c.Status == domain.ArchiveActive
}

func inScope(t domain.Transaction, scope string) bool {
	if scope == ScopeAll {
		return true
	}
	return idStr(t.AccountID) == scope || (t.DestAccountID != nil && idStr(*t.DestAccountID) == scope)
}

func passesFilter(row JournalRow, scope string, f JournalFilters) bool {
	t := row.Txn
	if !inScope(t, scope) {
		return false
	}
	if row.IsTransfer && !f.IncludeTransfers {
		return false
	}
	if f.CategoryID != nil && (t.CategoryID == nil || *t.CategoryID != *f.CategoryID) {
		return false
	}
	if len(f.Statuses) > 0 && !containsStatus(f.Statuses, t.Status) {
		return false
	}
	if f.Q != "" && !strings.Contains(strings.ToLower(t.Label), strings.ToLower(f.Q)) {
		return false
	}
	return true
}

func accumulateSummary(sum *JournalSummary, t domain.Transaction) {
	if t.FlowType == domain.FlowTransfer {
		return // neutralised (rules §10)
	}
	switch t.Status {
	case domain.StatusCleared:
		if t.FlowType == domain.FlowIncome {
			sum.IncomeReceived += t.Amount
		} else {
			sum.RealExpenses += -t.Amount
		}
	case domain.StatusPending:
		if t.FlowType != domain.FlowIncome {
			sum.RealExpenses += -t.Amount
			sum.Pending += -t.Amount
			sum.PendingCount++
		}
	case domain.StatusAwaited:
		sum.Awaited += magnitudeOf(t.Amount)
		sum.AwaitedCount++
	}
}

func sortRowsBy(rows []JournalRow, srt, dir string) {
	less := func(a, b JournalRow) bool {
		switch srt {
		case "date":
			return dateLess(a.Txn, b.Txn)
		case "period":
			return a.Txn.BudgetPeriod < b.Txn.BudgetPeriod
		case "label":
			return strings.ToLower(a.Txn.Label) < strings.ToLower(b.Txn.Label)
		case "cat":
			return a.CategoryName < b.CategoryName
		case "acct":
			return a.AccountName < b.AccountName
		case "amount":
			return a.Txn.Amount < b.Txn.Amount
		case "status":
			return statusRank(a.Txn.Status) < statusRank(b.Txn.Status)
		default:
			return dateLess(a.Txn, b.Txn)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if dir == dirDesc {
			return less(rows[j], rows[i])
		}
		return less(rows[i], rows[j])
	})
	// Undated (awaited) rows sort last in descending order (M19) regardless of key.
	if dir == dirDesc && srt == "date" {
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].Txn.OpDate != nil && rows[j].Txn.OpDate == nil
		})
	}
}

func dateLess(a, b domain.Transaction) bool {
	if a.OpDate == nil || b.OpDate == nil {
		return a.OpDate != nil && b.OpDate == nil // nil dates sort low (last in desc)
	}
	return a.OpDate.Before(*b.OpDate)
}

func statusRank(s domain.TransactionStatus) int {
	switch s {
	case domain.StatusCleared:
		return 0
	case domain.StatusPending:
		return 1
	case domain.StatusAwaited:
		return 2
	default:
		return 2
	}
}

func magnitudeOf(signed int64) int64 {
	if signed < 0 {
		return -signed
	}
	return signed
}

func containsStatus(set []domain.TransactionStatus, s domain.TransactionStatus) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}

// budgetPeriodFor returns the explicit period, else the period of the op_date,
// else the current month (date/period decoupling, foundation §3).
func budgetPeriodFor(in TxnInput) string {
	if in.BudgetPeriod != "" {
		return in.BudgetPeriod
	}
	if in.OpDate != nil {
		return in.OpDate.Period()
	}
	return ""
}

func normaliseSort(s string) string {
	switch s {
	case "date", "period", "label", "cat", "acct", "amount", "status":
		return s
	default:
		return journalSortDate
	}
}

func normaliseDir(dir, srt string) string {
	if dir == dirAsc || dir == dirDesc {
		return dir
	}
	if srt == "" || srt == "date" || srt == "amount" || srt == "period" {
		return dirDesc
	}
	return dirAsc
}

func validationErr(field, msg string) error {
	v := &domain.ValidationError{}
	v.Add(field, msg)
	return v
}

// parseDayMonth parses a "DD/MM" inline-edit date into a clock-free Date using
// the year of the budget period (the calendar widget is locale/year-agnostic;
// the year is authoritative server-side). Locale-free (numeric).
func parseDayMonth(value, period string) (domain.Date, error) {
	value = strings.TrimSpace(value)
	y, _, ok := parsePeriodYM(period)
	if !ok || len(value) != 5 || value[2] != '/' {
		return domain.Date{}, errBadField
	}
	d, err1 := parseIntStrict(value[:2])
	m, err2 := parseIntStrict(value[3:])
	if err1 != nil || err2 != nil || m < 1 || m > 12 || d < 1 || d > 31 {
		return domain.Date{}, errBadField
	}
	return domain.NewDate(y, m, d), nil
}

// parseMagnitude parses a canonical integer minor-unit string (the handler has
// already converted the locale-formatted amount via i18n.ParseMoney). Locale-free.
func parseMagnitude(value string) int64 {
	n, err := parseInt64(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parseID(value string) (int64, error) {
	n, err := parseInt64(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return 0, errBadField
	}
	return n, nil
}

var errBadField = errors.New("services: bad field value")

func parseInt64(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

func parseIntStrict(s string) (int, error) {
	n, err := strconv.Atoi(s)
	return n, err
}
