package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"econome/internal/domain"
	"econome/internal/i18n"
	"econome/internal/server/middleware"
	"econome/internal/services"
	"econome/internal/view"
)

// Journal handlers (functional/06, technical/04 §3.3). Quick-entry create,
// whole-cell inline edit, server-side sort/filter, atomic delete. Money is
// parsed at the boundary; the service is locale-free.

// JournalGet renders the full journal for the shared month/scope.
func (h *Handlers) JournalGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	d, err := h.svc.Journal(r.Context(), c.User.ID, h.period(r), h.scope(r), r.FormValue("sort"), r.FormValue("dir"), h.journalFilters(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.render(w, http.StatusOK, "journal", h.journalView(r, d))
}

// JournalRows re-renders just the table body for a sort/filter change (htmx),
// plus the OOB month summary.
func (h *Handlers) JournalRows(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	d, err := h.svc.Journal(r.Context(), c.User.ID, h.period(r), h.scope(r), r.FormValue("sort"), r.FormValue("dir"), h.journalFilters(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v := h.journalView(r, d)
	v.OOB = true
	h.writeFragments(w, fragment{"journal-rows", v}, fragment{"month-summary", v})
}

// TransactionCreate appends a quick-entry transaction and returns the new row
// (prepended) + the OOB summary.
func (h *Handlers) TransactionCreate(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	_ = r.ParseForm()
	in, err := h.txnInput(r)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	if _, err := h.svc.CreateTransaction(r.Context(), c.User.ID, in); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	d, err := h.svc.Journal(r.Context(), c.User.ID, h.period(r), h.scope(r), r.FormValue("sort"), r.FormValue("dir"), h.journalFilters(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v := h.journalView(r, d)
	v.OOB = true
	h.writeFragments(w, fragment{"journal-rows", v}, fragment{"month-summary", v})
}

// TransactionPatch applies one inline cell edit and returns the row + OOB summary.
func (h *Handlers) TransactionPatch(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	field, value, err := h.txnField(r)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	if err := h.svc.UpdateTransaction(r.Context(), c.User.ID, id, field, value); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.journalRowResponse(w, r, c.User.ID, id)
}

// TransactionDelete removes a transaction; the row is swapped out client-side
// and the summary updates OOB.
func (h *Handlers) TransactionDelete(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.svc.DeleteTransaction(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	d, err := h.svc.Journal(r.Context(), c.User.ID, h.period(r), h.scope(r), r.FormValue("sort"), r.FormValue("dir"), h.journalFilters(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v := h.journalView(r, d)
	v.OOB = true
	h.writeFragments(w, fragment{"month-summary", v})
}

// journalRowResponse re-reads the journal and emits the one edited row + OOB summary.
func (h *Handlers) journalRowResponse(w http.ResponseWriter, r *http.Request, userID, id int64) {
	d, err := h.svc.Journal(r.Context(), userID, h.period(r), h.scope(r), r.FormValue("sort"), r.FormValue("dir"), h.journalFilters(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v := h.journalView(r, d)
	for _, row := range v.Rows {
		if row.ID == id {
			vo := v
			vo.OOB = true
			h.writeFragments(w, fragment{"journal-row", row}, fragment{"month-summary", vo})
			return
		}
	}
	// The edited row fell out of the current filter — just refresh the summary.
	vo := v
	vo.OOB = true
	h.writeFragments(w, fragment{"month-summary", vo})
}

// --- request parsing ---

// journalFilters reads the view filters. Without the `filtered=1` sentinel (a
// bare GET /journal) the defaults apply: all statuses, internal transfers shown.
func (h *Handlers) journalFilters(r *http.Request) services.JournalFilters {
	_ = r.ParseForm()
	qy := r.Form // query + body (so a POST's hx-included filters are honoured)
	if qy.Get("filtered") != "1" {
		return services.JournalFilters{IncludeTransfers: true, Scope: h.scope(r)}
	}
	f := services.JournalFilters{
		Q:                strings.TrimSpace(qy.Get("fq")),
		IncludeTransfers: qy.Get("ftransfers") == "1",
		Scope:            h.scope(r),
	}
	if cid := strings.TrimSpace(qy.Get("fcategory")); cid != "" {
		if id, err := strconv.ParseInt(cid, 10, 64); err == nil {
			f.CategoryID = &id
		}
	}
	for _, code := range qy["fstatus"] {
		if s := domain.TransactionStatus(strings.TrimSpace(code)); s.Valid() {
			f.Statuses = append(f.Statuses, s)
		}
	}
	return f
}

func (h *Handlers) txnInput(r *http.Request) (services.TxnInput, error) {
	lang := middleware.From(r.Context()).Lang
	in := services.TxnInput{
		Label:    r.PostFormValue("label"),
		Status:   domain.TransactionStatus(orDefault(r.PostFormValue("status"), string(domain.StatusCleared))),
		FlowType: domain.FlowType(r.PostFormValue("flow_type")),
	}
	mag, err := i18n.ParseMoney(strings.TrimSpace(r.PostFormValue("amount")), lang)
	if err != nil || mag <= 0 {
		return in, validation("amount", domain.MsgAmountInvalid)
	}
	in.Magnitude = mag
	if aid := r.PostFormValue("account_id"); aid != "" {
		in.AccountID, _ = strconv.ParseInt(aid, 10, 64)
	}
	if cid := r.PostFormValue("category_id"); cid != "" {
		if id, e := strconv.ParseInt(cid, 10, 64); e == nil {
			in.CategoryID = &id
		}
	}
	if did := r.PostFormValue("dest_account_id"); did != "" {
		if id, e := strconv.ParseInt(did, 10, 64); e == nil {
			in.DestAccountID = &id
		}
	}
	period := h.period(r)
	if dd := strings.TrimSpace(r.PostFormValue("op_date")); dd != "" {
		if dt, e := dayMonth(dd, period); e == nil {
			in.OpDate = &dt
		}
	}
	if in.OpDate != nil {
		in.BudgetPeriod = in.OpDate.Period()
	} else {
		in.BudgetPeriod = period
	}
	if in.FlowType != domain.FlowTransfer {
		in.FlowType = "" // derived from the category server-side
	}
	return in, nil
}

// txnField returns the single (field, value) for an inline PATCH. The amount is
// normalised to canonical minor units here (the service stays locale-free).
func (h *Handlers) txnField(r *http.Request) (field, value string, err error) {
	lang := middleware.From(r.Context()).Lang
	for _, f := range []string{"label", "status", "op_date", "budget_period", "amount", "category_id", "account_id"} {
		if v, ok := firstFormValue(r, f); ok {
			if f == "amount" {
				m, e := i18n.ParseMoney(strings.TrimSpace(v), lang)
				if e != nil || m <= 0 {
					return "", "", validation("amount", domain.MsgAmountInvalid)
				}
				return f, strconv.FormatInt(m, 10), nil
			}
			return f, v, nil
		}
	}
	return "", "", validation("field", domain.MsgFieldInvalid)
}

// --- view assembly ---

func (h *Handlers) journalView(r *http.Request, d *services.JournalData) view.JournalView {
	base := h.base(r)
	c := middleware.From(r.Context())
	v := view.JournalView{
		Base:       base,
		Email:      c.User.Email,
		Nav:        "budget",
		Period:     d.Period,
		MonthLabel: h.monthLabel(base, d.Period),
		PrevPeriod: shiftPeriod(d.Period, -1),
		NextPeriod: shiftPeriod(d.Period, +1),
		Scope:      d.Scope,
		Scopes:     h.journalScopes(base, d),
		Savings:    h.railSavings(r, base),
		Sort:       d.Sort,
		Dir:        d.Dir,
		NotCreated: !d.Exists,
		Locked:     d.Locked,
		Editable:   d.Editable,
		Empty:      d.Exists && len(d.Rows) == 0,
	}
	if y, m, ok := splitPeriod(d.Period); ok {
		v.YearLabel = strconv.Itoa(y)
		v.MonthIndex = m - 1
		v.PrevYearPeriod = fmt.Sprintf("%04d-%02d", y-1, m)
		v.NextYearPeriod = fmt.Sprintf("%04d-%02d", y+1, m)
		for i := 1; i <= 12; i++ {
			v.MonthCells = append(v.MonthCells, view.MonthCell{Period: fmt.Sprintf("%04d-%02d", y, i), Label: base.T("month." + strconv.Itoa(i)), On: i == m})
		}
	}
	v.PickerOpen = strings.TrimSpace(r.URL.Query().Get("pick")) == "1"
	v.CatsJSON, v.AcctsJSON, v.StatusJSON = h.journalOptions(base, d)
	v.LabelsJSON = h.journalLabels(r)
	v.FQ, v.FCategory, v.FTransfers, v.FStatuses = h.journalFilterState(base, r)
	if !d.Exists {
		return v
	}
	for _, row := range d.Rows {
		v.Rows = append(v.Rows, h.journalRow(base, d, row))
	}
	v.Summary = h.journalSummary(base, d.Summary)
	return v
}

func (h *Handlers) journalRow(base view.Base, d *services.JournalData, row services.JournalRow) view.JRow {
	t := row.Txn
	jr := view.JRow{
		ID:             t.ID,
		AccountID:      t.AccountID,
		Period:         d.Period,
		Scope:          d.Scope,
		DateStr:        txnDateDisplay(t, row.ExpectedDay),
		DateApprox:     t.OpDate == nil,
		BudgetPeriod:   t.BudgetPeriod,
		PeriodLabel:    monthShort(base, t.BudgetPeriod),
		Label:          t.Label,
		CategoryName:   row.CategoryName,
		AccountName:    row.AccountName,
		DestName:       row.DestName,
		IsTransfer:     row.IsTransfer,
		AmountStr:      base.Amount(magabs(t.Amount)),
		Status:         string(t.Status),
		StatusLabel:    base.T("txn.status." + string(t.Status)),
		StatusClass:    statusClass(t.Status),
		StatusIcon:     statusIcon(t.Status),
		Editable:       d.Editable,
		DelTitle:       base.T("action.delete"),
		AccountDisplay: row.AccountName,
	}
	switch {
	case row.IsTransfer:
		jr.AmountMuted = true
		jr.CategoryName = base.T("flow.transfer")
		jr.AccountDisplay = row.AccountName + " → " + row.DestName
	case t.FlowType == domain.FlowIncome:
		jr.AmountPos = true
		jr.AmountStr = "+" + jr.AmountStr
	default:
		jr.AmountStr = "−" + jr.AmountStr
	}
	if t.CategoryID != nil {
		jr.CategoryID = *t.CategoryID
	}
	jr.PeriodHighlight = t.OpDate != nil && t.OpDate.Period() != t.BudgetPeriod
	return jr
}

func (h *Handlers) journalScopes(base view.Base, d *services.JournalData) []view.FScope {
	scopes := []view.FScope{{Key: services.ScopeAll, Name: base.T("shell.scope.all"), Note: base.T("forecast.scope.aggregated"), IsAll: true, On: d.Scope == services.ScopeAll}}
	for _, a := range d.Accounts {
		note := base.T("forecast.scope.carry")
		if a.MonthEndPolicy == domain.PolicySweep {
			note = base.T("forecast.scope.sweep")
		}
		key := strconv.FormatInt(a.ID, 10)
		scopes = append(scopes, view.FScope{Key: key, Name: a.Name, Note: note, On: d.Scope == key})
	}
	return scopes
}

func (h *Handlers) journalSummary(base view.Base, s services.JournalSummary) view.JSummary {
	return view.JSummary{
		IncomeStr:    "+" + base.Money(s.IncomeReceived),
		RealStr:      "−" + base.Money(s.RealExpenses),
		PendingStr:   base.Money(s.Pending),
		PendingCount: s.PendingCount,
		AwaitedStr:   base.Money(s.Awaited),
		AwaitedCount: s.AwaitedCount,
		NetStr:       signedMoney(base, s.NetBalance),
		NetPos:       s.NetBalance >= 0,
	}
}

// journalOptions builds the JSON option sets for the quick-entry custom selects.
func (h *Handlers) journalOptions(base view.Base, d *services.JournalData) (cats, accts, statuses template.JS) {
	acctByID := map[int64]string{}
	for _, a := range d.Accounts {
		acctByID[a.ID] = a.Name
	}
	catOpts := []view.JOption{}
	for _, c := range d.Categories {
		o := view.JOption{Value: strconv.FormatInt(c.ID, 10), Label: c.Name, Flow: string(c.FlowType)}
		if a, ok := d.CatAccount[c.ID]; ok {
			o.Acct = strconv.FormatInt(a, 10)
		}
		catOpts = append(catOpts, o)
	}
	catOpts = append(catOpts, view.JOption{Value: "transfer", Label: base.T("flow.transfer"), Flow: "transfer"})

	acctOpts := []view.JOption{}
	for _, a := range d.Accounts {
		acctOpts = append(acctOpts, view.JOption{Value: strconv.FormatInt(a.ID, 10), Label: a.Name})
	}
	statusOpts := []view.JOption{
		{Value: "cleared", Label: base.T("txn.status.cleared"), Icon: "✓"},
		{Value: "pending", Label: base.T("txn.status.pending"), Icon: "⏳"},
		{Value: "awaited", Label: base.T("txn.status.awaited"), Icon: "🕐"},
	}
	return asJSON(catOpts), asJSON(acctOpts), asJSON(statusOpts)
}

func (h *Handlers) journalFilterState(base view.Base, r *http.Request) (q, category string, transfers bool, statuses []view.JFilterStatus) {
	qy := r.URL.Query()
	on := map[string]bool{"cleared": true, "pending": true, "awaited": true}
	transfers = true
	if qy.Get("filtered") == "1" {
		q = strings.TrimSpace(qy.Get("fq"))
		category = strings.TrimSpace(qy.Get("fcategory"))
		transfers = qy.Get("ftransfers") == "1"
		on = map[string]bool{}
		for _, code := range qy["fstatus"] {
			on[strings.TrimSpace(code)] = true
		}
	}
	statuses = []view.JFilterStatus{
		{Code: "cleared", Label: base.T("txn.status.cleared"), Icon: "✓", On: on["cleared"]},
		{Code: "pending", Label: base.T("txn.status.pending"), Icon: "⏳", On: on["pending"]},
		{Code: "awaited", Label: base.T("txn.status.awaited"), Icon: "🕐", On: on["awaited"]},
	}
	return q, category, transfers, statuses
}

// --- small helpers ---

// journalLabels embeds the user's most-used labels for the emAutocomplete widget
// ([{label, count}], M21). Bounded to a top-N; the static widget ranks
// prefix>substring + usage client-side.
func (h *Handlers) journalLabels(r *http.Request) template.JS {
	c := middleware.From(r.Context())
	rows, err := h.svc.TopLabels(r.Context(), c.User.ID, 200)
	if err != nil {
		return template.JS("[]")
	}
	type lbl struct {
		Label string `json:"label"`
		Count int    `json:"count"`
	}
	out := make([]lbl, 0, len(rows))
	for _, m := range rows {
		out = append(out, lbl{Label: m.Label, Count: m.UsageCount})
	}
	return asJSON(out)
}

func asJSON(v any) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return template.JS("[]")
	}
	return template.JS(b) //nolint:gosec // server-built JSON option set, no user script
}

func statusIcon(s domain.TransactionStatus) string {
	switch s {
	case domain.StatusCleared:
		return "✓"
	case domain.StatusPending:
		return "⏳"
	case domain.StatusAwaited:
		return "🕐"
	default:
		return "🕐"
	}
}

// txnDateDisplay renders a row's date: "DD/MM" when dated; "~DD/MM" for a
// recurring awaited (its expected day); "—" otherwise.
func txnDateDisplay(t domain.Transaction, expectedDay *int) string {
	if t.OpDate != nil {
		return fmt.Sprintf("%02d/%02d", t.OpDate.Day, t.OpDate.Month)
	}
	if t.Status == domain.StatusAwaited && expectedDay != nil {
		if _, m, ok := splitPeriod(t.BudgetPeriod); ok {
			return fmt.Sprintf("~%02d/%02d", *expectedDay, m)
		}
	}
	return "—"
}

func monthShort(base view.Base, period string) string {
	if _, m, ok := splitPeriod(period); ok {
		return base.T("month." + strconv.Itoa(m))
	}
	return period
}

func dayMonth(value, period string) (domain.Date, error) {
	value = strings.TrimSpace(value)
	y, _, ok := splitPeriod(period)
	if !ok || len(value) != 5 || value[2] != '/' {
		return domain.Date{}, errBadDate
	}
	dd, e1 := strconv.Atoi(value[:2])
	mm, e2 := strconv.Atoi(value[3:])
	if e1 != nil || e2 != nil || mm < 1 || mm > 12 || dd < 1 || dd > 31 {
		return domain.Date{}, errBadDate
	}
	return domain.NewDate(y, mm, dd), nil
}

var errBadDate = fmt.Errorf("bad date")

func magabs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func signedMoney(base view.Base, minor int64) string {
	if minor >= 0 {
		return "+" + base.Money(minor)
	}
	return "−" + base.Money(-minor)
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func firstFormValue(r *http.Request, key string) (string, bool) {
	if vs, ok := r.Form[key]; ok && len(vs) > 0 {
		return vs[0], true
	}
	return "", false
}

func validation(field, msg string) error {
	ve := &domain.ValidationError{}
	ve.Add(field, msg)
	return ve
}
