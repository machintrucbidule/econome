package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"econome/internal/domain"
	"econome/internal/i18n"
	"econome/internal/server/middleware"
	"econome/internal/services"
	"econome/internal/view"
)

// Forecast (Prévisionnel) — the read-only budget landing (functional/05,
// increment 6a). It renders the shared month + account scope: the envelope
// hierarchy with five-state badges, the right insights panel (figures + savings
// encart + à surveiller) and the server-rendered treasury timeline. Every figure
// is the pure engine's output (derived-not-stored); this screen never mutates.

// ForecastGet renders the forecast for the shared month/scope, or the
// "month not created" landing state offering the initialisation assistant.
func (h *Handlers) ForecastGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	period := h.period(r)
	scope := h.scope(r)
	d, err := h.svc.Forecast(r.Context(), c.User.ID, period, scope)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.render(w, http.StatusOK, "forecast", h.forecastView(r, d))
}

// AllocationPatch is the inline `Prévu` edit (PATCH /allocations/{env}). It
// upserts the allocation and returns the edited row plus the OOB fragments that
// change (parent rollup, footer total, savings panel, figures) — the recalc
// matrix for an allocation edit (functional/04 §6, technical/04 §3.2). A bad or
// negative amount snaps the field back (re-render unchanged), never a partial
// write.
func (h *Handlers) AllocationPatch(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	envID, err := strconv.ParseInt(r.PathValue("env"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period, scope := h.period(r), h.scope(r)

	_ = r.ParseForm()
	planned, perr := i18n.ParseMoney(strings.TrimSpace(r.PostFormValue("planned")), c.Lang)
	if perr == nil && planned >= 0 {
		if err := h.svc.EditAllocation(r.Context(), c.User.ID, envID, period, planned); err != nil {
			h.mutationError(w, r, err, nil)
			return
		}
	}
	// Re-read and re-render (unchanged on a bad amount → the field reverts).
	d, err := h.svc.Forecast(r.Context(), c.User.ID, period, scope)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.writeAllocationFragments(w, r, d, envID)
}

// EndOfMonthTransfer generates the residual savings sweep (POST
// /transfers/end-of-month) and returns the savings panel + OOB figures + timeline
// (the balances and low point shift; technical/04 §3.2).
func (h *Handlers) EndOfMonthTransfer(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	period, scope := h.period(r), h.scope(r)
	sweepID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("account")), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.svc.EndOfMonthTransfer(r.Context(), c.User.ID, sweepID, period); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	d, err := h.svc.Forecast(r.Context(), c.User.ID, period, scope)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v := h.forecastView(r, d)
	v.OOB = true
	h.writeFragments(w, fragment{"fc-panel", v}, fragment{"fc-figures", v}, fragment{"fc-timeline", v})
}

type fragment struct {
	name string
	data any
}

func (h *Handlers) writeFragments(w http.ResponseWriter, frags ...fragment) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	for _, f := range frags {
		if err := h.rdr.Render(w, f.name, f.data); err != nil {
			slog.Error("render fragment", "name", f.name, "err", err)
		}
	}
}

// writeAllocationFragments emits the edited row (primary swap) + the OOB
// fragments an allocation edit changes.
func (h *Handlers) writeAllocationFragments(w http.ResponseWriter, r *http.Request, d *services.ForecastData, envID int64) {
	v := h.forecastView(r, d)
	frags := make([]fragment, 0, 5)

	if row, ok := findRowByEnv(v.Rows, envID); ok {
		row.Hidden = false // the edited row is visible (the user just edited it)
		row.OOB = false    // primary swap into #fc-row-<key>
		frags = append(frags, fragment{"fc-row", row})
		if row.ParentKey != "" {
			if parent, ok := findRowByKey(v.Rows, row.ParentKey); ok {
				parent.OOB = true
				frags = append(frags, fragment{"fc-row", parent})
			}
		}
	}
	vo := v
	vo.OOB = true
	frags = append(frags, fragment{"fc-total", vo}, fragment{"fc-panel", vo}, fragment{"fc-figures", vo})
	h.writeFragments(w, frags...)
}

func findRowByEnv(rows []view.FRow, envID int64) (view.FRow, bool) {
	for _, r := range rows {
		if r.Kind != "parent" && r.EnvID == envID {
			return r, true
		}
		if c, ok := findRowByEnv(r.Children, envID); ok {
			return c, true
		}
	}
	return view.FRow{}, false
}

func findRowByKey(rows []view.FRow, key string) (view.FRow, bool) {
	for _, r := range rows {
		if r.Key == key {
			return r, true
		}
	}
	return view.FRow{}, false
}

func (h *Handlers) forecastView(r *http.Request, d *services.ForecastData) view.ForecastView {
	base := h.base(r)
	c := middleware.From(r.Context())

	v := view.ForecastView{
		Base:       base,
		Email:      c.User.Email,
		Nav:        "budget",
		Period:     d.Period,
		MonthLabel: h.monthLabel(base, d.Period),
		PrevPeriod: shiftPeriod(d.Period, -1),
		NextPeriod: shiftPeriod(d.Period, +1),
		Scope:      d.Scope,
		ScopeKind:  d.ScopeKind,
		Scopes:     h.forecastScopes(base, d),
		NotCreated: !d.Exists,
		Locked:     d.Locked,
		Empty:      d.Empty,
	}
	if y, m, ok := splitPeriod(d.Period); ok {
		v.YearLabel = strconv.Itoa(y)
		v.MonthIndex = m - 1
		v.PrevYearPeriod = fmt.Sprintf("%04d-%02d", y-1, m)
		v.NextYearPeriod = fmt.Sprintf("%04d-%02d", y+1, m)
		for i := 1; i <= 12; i++ {
			v.MonthCells = append(v.MonthCells, view.MonthCell{
				Period: fmt.Sprintf("%04d-%02d", y, i),
				Label:  base.T("month." + strconv.Itoa(i)),
				On:     i == m,
			})
		}
	}
	v.PickerOpen = strings.TrimSpace(r.URL.Query().Get("pick")) == "1"
	if !d.Exists {
		return v
	}

	v.Editable = d.Editable
	v.ShowPills = d.ScopeKind == "aggregated"
	v.HasHiddenTransfers = d.HasHiddenTransfers
	for _, row := range d.Rows {
		v.Rows = append(v.Rows, h.forecastRow(base, row, d.Editable, d.Period, d.Scope))
	}
	v.Total = view.FTotal{
		PlannedStr: base.Amount(d.Total.Planned),
		RealStr:    base.Amount(d.Total.Real),
		RemainStr:  base.Amount(d.Total.Remaining),
		RemainNeg:  d.Total.Remaining < 0,
	}
	v.Figures = h.forecastFigures(base, d)
	v.Encarts = h.forecastEncarts(base, d)
	v.CarryNote = d.CarryNote
	if d.CarryNote {
		v.CarryNext = h.monthLabel(base, shiftPeriod(d.Period, +1))
	}
	v.Watch = h.forecastWatch(base, d.Watch)
	v.HasWatch = len(v.Watch) > 0
	h.forecastTimeline(base, d, &v)
	return v
}

func (h *Handlers) forecastRow(base view.Base, row services.ForecastRow, editable bool, period, scope string) view.FRow {
	kind := "leaf"
	switch {
	case row.IsParent:
		kind = "parent"
	case row.ShowPill:
		kind = "flat"
	}
	r := view.FRow{
		Kind:        kind,
		Key:         row.Key,
		EnvID:       row.EnvelopeID,
		Name:        row.Name,
		AccountName: row.AccountName,
		ShowPill:    row.ShowPill,
		AggBadge:    row.IsParent,
		Income:      row.Income,
		PlannedStr:  base.Amount(row.Planned),
		RealStr:     base.Amount(row.Real),
		Period:      period,
		Scope:       scope,
		// The Prévu cell is an inline input only on an active month, for a
		// posting leaf/child (a parent shows its read-only rollup sum); the
		// aggregated flat row stays read-only (the overview, not the edit surface).
		Editable: editable && (kind == "leaf"),
	}
	if row.IsParent {
		r.AggLabel = base.T("forecast.agg")
	}
	class, label := badgeFor(base, row)
	r.BadgeClass, r.BadgeLabel = class, label

	if row.Income {
		r.RemainDash = true
	} else {
		r.RemainStr = base.Amount(row.Remaining)
		r.RemainNeg = row.Remaining < 0
		r.RealNeg = row.State == domain.StateOverrun
	}
	if row.HasBar {
		r.HasBar = true
		r.BarPercent = capPercent(row.Percent)
		r.BarClass = "warn"
		if row.State == domain.StateOverrun {
			r.BarClass = "bad"
		}
	}
	for _, ch := range row.Children {
		cr := h.forecastRow(base, ch, editable, period, scope)
		cr.Kind = "child"
		cr.ParentKey = row.Key
		cr.Hidden = true
		cr.Editable = editable // children are posting leaves → editable
		r.Children = append(r.Children, cr)
	}
	for _, t := range row.Drill {
		r.Drill = append(r.Drill, view.FTxn{
			Label:       t.Label,
			DateStr:     t.DateStr,
			Approx:      t.Approx,
			AmountStr:   base.Amount(t.Amount),
			AmountNeg:   t.Amount < 0,
			StatusClass: statusClass(t.Status),
			StatusLabel: base.T("txn.status." + string(t.Status)),
		})
	}
	r.HasDrill = kind == "leaf"
	return r
}

// badgeFor maps an envelope row to its state badge (class, label). Income rows
// show received-vs-expected without overrun red (rules §4); expenses use the
// five states with the percent suffix on partial/overrun (functional/05 §2).
func badgeFor(base view.Base, row services.ForecastRow) (class, label string) {
	if row.Income {
		if row.Real > 0 {
			return "ok", base.T("forecast.income.received")
		}
		return "info", base.T("forecast.income.expected")
	}
	switch row.State {
	case domain.StateNone:
		return "mut", base.T("state.none")
	case domain.StateExpected:
		return "info", base.T("state.expected")
	case domain.StatePartial:
		return "warn", base.T("state.partial") + " · " + strconv.Itoa(capPercent(row.Percent)) + "%"
	case domain.StatePaid:
		return "ok", base.T("state.paid")
	case domain.StateOverrun:
		return "bad", base.T("state.overrun") + " · " + strconv.Itoa(row.Percent) + "%"
	default:
		return "mut", base.T("state.none")
	}
}

func (h *Handlers) forecastScopes(base view.Base, d *services.ForecastData) []view.FScope {
	scopes := []view.FScope{{Key: services.ScopeAll, Name: base.T("shell.scope.all"), Note: base.T("forecast.scope.aggregated"), IsAll: true, On: d.Scope == services.ScopeAll}}
	for _, a := range d.Accounts {
		if a.Type != domain.AccountCurrent {
			continue
		}
		note := base.T("forecast.scope.carry")
		if a.MonthEndPolicy == domain.PolicySweep {
			note = base.T("forecast.scope.sweep")
		}
		key := strconv.FormatInt(a.ID, 10)
		scopes = append(scopes, view.FScope{Key: key, Name: a.Name, Note: note, On: d.Scope == key})
	}
	return scopes
}

func (h *Handlers) forecastFigures(base view.Base, d *services.ForecastData) []view.FFig {
	f := d.Figures
	clearedSub := ""
	if f.InProgress > 0 {
		clearedSub = base.T("forecast.fig.pending", base.Money(f.InProgress))
	}
	balCompte := view.FFig{Label: base.T("forecast.fig.balance"), Value: base.Money(f.BalanceCleared), Sub: clearedSub, Help: base.T("forecast.fig.balance_help")}
	balReal := view.FFig{Label: base.T("forecast.fig.real"), Value: base.Money(f.BalanceReal), Sub: base.T("forecast.fig.real_sub"), Help: base.T("forecast.fig.real_help")}

	switch d.ScopeKind {
	case "carry":
		return []view.FFig{
			balCompte, balReal,
			{Label: base.T("forecast.fig.incoming"), Value: base.Money(f.IncomingXfer), Sub: base.T("forecast.fig.incoming_sub"), Mod: "hl"},
			{Label: base.T("forecast.fig.eom"), Value: base.Money(f.ProjectedEnd), Sub: base.T("forecast.fig.eom_sub", h.monthLabel(base, shiftPeriod(d.Period, +1))), Mod: "good"},
		}
	case "sweep":
		return []view.FFig{
			balCompte, balReal,
			{Label: base.T("forecast.fig.income"), Value: base.Money(f.Income), Sub: base.T("forecast.fig.income_sub"), Mod: "hl"},
			lowFig(base, f.LowPoint, f.LowBreaches, f.ResidualNeg, ""),
		}
	default: // aggregated
		balCompte.Sub = base.T("forecast.fig.aggregated") + clearedTail(base, f.InProgress)
		return []view.FFig{
			balCompte, balReal,
			{Label: base.T("forecast.fig.income"), Value: base.Money(f.Income), Sub: base.T("forecast.fig.income_sub"), Mod: "hl"},
			lowFig(base, f.LowPoint, f.LowBreaches, f.ResidualNeg, f.LowAccountName),
		}
	}
}

// lowFig builds the Point bas card. It turns red on a real overdraft (the
// transaction low point breaches zero) or when an in-scope sweep residual is
// negative (functional/05 §4a — coherent with the red savings encart).
func lowFig(base view.Base, low int64, breaches, residualNeg bool, account string) view.FFig {
	mod, sub := "good", base.T("forecast.fig.low_ok")
	label := base.T("forecast.fig.low")
	if account != "" {
		label = base.T("forecast.fig.low_critical")
		sub = account
	}
	if breaches || residualNeg {
		mod = "bad"
		if account == "" {
			sub = base.T("forecast.fig.low_breach")
		}
	}
	return view.FFig{Label: label, Value: base.Money(low), Sub: sub, Mod: mod, Help: base.T("forecast.fig.low_help")}
}

func clearedTail(base view.Base, inProgress int64) string {
	if inProgress > 0 {
		return " · " + base.T("forecast.fig.pending", base.Money(inProgress))
	}
	return ""
}

func (h *Handlers) forecastEncarts(base view.Base, d *services.ForecastData) []view.FEncart {
	var out []view.FEncart
	for _, e := range d.Encarts {
		switch e.Kind {
		case "negative":
			out = append(out, view.FEncart{
				Kind: "negative", Title: base.T("forecast.save.negative"),
				BigStr: base.Money(e.Projected), Sub: base.T("forecast.save.negative_sub"),
				ActionLabel: base.T("forecast.save.transfer"), SweepID: e.SweepID, Disabled: true,
			})
		case "cascade":
			out = append(out, view.FEncart{
				Kind: "cascade", Title: base.T("forecast.save.cascade"),
				BigStr: base.Money(e.Secured), Sub: base.T("forecast.save.cascade_sub"),
				ActionLabel: base.T("forecast.save.cascade_btn"),
			})
		default:
			title := base.T("forecast.save.residual")
			if e.TargetName != "" {
				title += " → " + e.TargetName
			}
			out = append(out, view.FEncart{
				Kind: "residual", Title: title, AccountName: e.AccountName,
				BigStr:      base.Money(e.Secured),
				Sub:         base.T("forecast.save.residual_sub", base.Money(e.Secured), base.Money(e.Projected)),
				ActionLabel: base.T("forecast.save.transfer"),
				SweepID:     e.SweepID,
				// "Virer en fin de mois" is live only on an active month with a
				// realised residual to sweep (to_save > 0); otherwise disabled.
				Disabled: !d.Editable || e.ToSave <= 0,
			})
		}
	}
	return out
}

func (h *Handlers) forecastWatch(base view.Base, items []services.ForecastWatch) []view.FWatch {
	var out []view.FWatch
	for _, w := range items {
		switch w.Kind {
		case "overrun":
			out = append(out, view.FWatch{Label: w.Label + " · " + base.T("forecast.watch.over"), ValueStr: "+" + base.Money(w.Amount), Bad: true})
		case "awaited":
			out = append(out, view.FWatch{Label: w.Label + " · " + base.T("forecast.watch.upcoming"), ValueStr: base.Money(w.Amount)})
		default:
			out = append(out, view.FWatch{Label: w.Label + " · " + base.T("forecast.watch.remaining"), ValueStr: base.Money(w.Amount)})
		}
	}
	return out
}

func (h *Handlers) forecastTimeline(base view.Base, d *services.ForecastData, v *view.ForecastView) {
	tl := d.Timeline
	if tl == nil {
		return
	}
	title := base.T("forecast.tl.title") + " — " + tl.AccountName
	endLabel := base.T("forecast.tl.low")
	switch {
	case d.ScopeKind == "aggregated":
		endLabel = base.T("forecast.tl.low_critical")
		if tl.CriticalSuffix != "" {
			title = base.T("forecast.tl.title_aggregated", tl.CriticalSuffix)
		}
	case tl.IsCarry:
		endLabel = base.T("forecast.tl.eom")
	}
	v.TimelineTitle = title

	in := view.TLInput{
		DaysInMonth: tl.DaysInMonth,
		LowValueStr: base.Money(tl.LowValue),
		LowDay:      tl.LowDay,
		LowBalance:  tl.LowValue,
		LowBreaches: tl.LowBreaches,
		EndLabel:    endLabel,
	}
	for _, p := range tl.Points {
		in.Points = append(in.Points, view.TLPoint{Day: p.Day, Balance: p.Balance, Kind: p.Kind})
	}
	v.TimelineSVG = view.RenderTimeline(in)
	v.TimelineLegend = []view.TLLegend{
		{Label: base.T("forecast.tl.leg.in"), Color: "var(--ok)"},
		{Label: base.T("forecast.tl.leg.debit"), Color: "var(--brand)"},
		{Label: base.T("forecast.tl.leg.over"), Color: "var(--bad)"},
		{Label: base.T("forecast.tl.leg.awaited"), Color: "var(--warn)"},
		{Label: base.T("forecast.tl.leg.low"), Color: "var(--ok)", Hollow: true},
	}
}

func statusClass(s domain.TransactionStatus) string {
	switch s {
	case domain.StatusCleared:
		return "ok"
	case domain.StatusPending:
		return "warn"
	case domain.StatusAwaited:
		return "warn"
	default:
		return "mut"
	}
}

func capPercent(p int) int {
	if p > 100 {
		return 100
	}
	if p < 0 {
		return 0
	}
	return p
}

// shiftPeriod returns the "YYYY-MM" delta months away from period (delta ±1).
func shiftPeriod(period string, delta int) string {
	y, m, ok := splitPeriod(period)
	if !ok {
		return period
	}
	m += delta
	for m < 1 {
		m += 12
		y--
	}
	for m > 12 {
		m -= 12
		y++
	}
	return fmt.Sprintf("%04d-%02d", y, m)
}

func splitPeriod(period string) (year, month int, ok bool) {
	if len(period) != 7 || period[4] != '-' {
		return 0, 0, false
	}
	y, err1 := strconv.Atoi(period[:4])
	m, err2 := strconv.Atoi(strings.TrimPrefix(period[5:], ""))
	if err1 != nil || err2 != nil || m < 1 || m > 12 {
		return 0, 0, false
	}
	return y, m, true
}
