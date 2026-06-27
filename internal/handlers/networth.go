package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"econome/internal/domain"
	"econome/internal/i18n"
	"econome/internal/server/middleware"
	"econome/internal/services"
	"econome/internal/view"
)

// Net worth (Patrimoine) handlers — Synthèse + Registre (functional/07,
// technical/04 §3.4). Snapshots and the per-month comment are the only inputs;
// PEA net, subtotals, the total and every delta are the pure engine's output.
// Net-worth mutations are **always allowed regardless of the budget month lock**
// (L7) — none funnels through the locked-month guard.

// NetWorthGet renders the Synthèse for the shared month.
func (h *Handlers) NetWorthGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	d, err := h.svc.NetWorthSynthesis(r.Context(), c.User.ID, h.period(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v, err := h.networthView(r, d)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.render(w, http.StatusOK, "networth", v)
}

// SnapshotUpsert enters/corrects one (account, month) gross value (POST
// /snapshots, upsert by natural identity, I-035) and returns the recomputed
// table + OOB cards. Always allowed regardless of the budget lock (L7).
func (h *Handlers) SnapshotUpsert(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	_ = r.ParseForm()
	accountID, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("account_id")), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := h.period(r)
	gross, perr := i18n.ParseMoney(strings.TrimSpace(r.PostFormValue("gross_value")), c.Lang)
	if perr != nil {
		h.mutationError(w, r, validation("gross_value", domain.MsgAmountInvalid), nil)
		return
	}
	if err := h.svc.UpsertSnapshot(r.Context(), c.User.ID, accountID, period, gross); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.snapshotResponse(w, r, c.User.ID, period)
}

// SnapshotDelete removes one snapshot (DELETE /snapshots/{id}); the following
// month's delta recomputes on read (L7). The account+period for the re-render
// come from the query (the cell carries them).
func (h *Handlers) SnapshotDelete(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.svc.DeleteSnapshot(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.snapshotResponse(w, r, c.User.ID, h.period(r))
}

// snapshotResponse re-reads the Synthèse and emits the table fragment (primary
// swap into #nw-table) + the OOB cards.
func (h *Handlers) snapshotResponse(w http.ResponseWriter, r *http.Request, userID int64, period string) {
	d, err := h.svc.NetWorthSynthesis(r.Context(), userID, period)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v, err := h.networthView(r, d)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	vo := v
	vo.OOB = true
	h.writeFragments(w, fragment{"nw-table", v}, fragment{"nw-cards", vo})
}

// CommentPut saves the per-month comment shared by both surfaces (PUT
// /networth/{period}/comment, B.2). Never locked (L7). Returns 204 — the client
// keeps the typed value.
func (h *Handlers) CommentPut(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	period := strings.TrimSpace(r.PathValue("period"))
	_ = r.ParseForm()
	if err := h.svc.SaveComment(r.Context(), c.User.ID, period, strings.TrimSpace(r.PostFormValue("comment"))); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RegisterGet renders the Registre (history table + evolution curve).
func (h *Handlers) RegisterGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	d, err := h.svc.Register(r.Context(), c.User.ID, r.URL.Query().Get("range"), h.period(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v, err := h.registerView(r, d)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.render(w, http.StatusOK, "register", v)
}

// RegisterChart re-renders just the evolution curve for a range change (M24/D3).
func (h *Handlers) RegisterChart(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	d, err := h.svc.Register(r.Context(), c.User.ID, r.URL.Query().Get("range"), h.period(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v, err := h.registerView(r, d)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.render(w, http.StatusOK, "nw-chart", v)
}

// --- view assembly ---

func (h *Handlers) networthView(r *http.Request, d *services.NetWorthData) (view.NetWorthView, error) {
	base := h.base(r)
	c := middleware.From(r.Context())
	v := view.NetWorthView{
		Base:          base,
		Email:         c.User.Email,
		Nav:           "patrimoine",
		Tab:           "synthese",
		Period:        d.Period,
		MonthLabel:    h.monthLabel(base, d.Period),
		PrevPeriod:    shiftPeriod(d.Period, -1),
		NextPeriod:    shiftPeriod(d.Period, +1),
		AutoprefillOn: d.AutoprefillOn,
		Empty:         d.Empty,
		HasAnyHistory: d.HasAnyHistory,
		HasSavings:    d.HasSavings,
	}
	h.fillNavigator(base, d.Period, r, &v.YearLabel, &v.MonthIndex, &v.PrevYearPeriod, &v.NextYearPeriod, &v.MonthCells, &v.PickerOpen)
	if err := h.fillRail(r, &v.CurAccounts, &v.Savings); err != nil {
		return v, err
	}
	v.RelabelStr = base.T("networth.relabel", v.MonthLabel)
	v.Cards = h.networthCards(base, d)
	v.Lines = h.networthLines(base, d)
	if d.Comment != "" {
		v.CommentValue = d.Comment
	} else if d.AutoprefillOn {
		v.CommentValue = prefillString(d.Movements)
		v.Prefilled = v.CommentValue != ""
	}
	return v, nil
}

// networthCards builds the metric cards (I-037, user-chosen at D4): Patrimoine
// total, the two biggest passbook livrets (by value), and a "Le reste" card
// aggregating every other support (PEA net + further livrets + employee savings).
func (h *Handlers) networthCards(base view.Base, d *services.NetWorthData) []view.NWCard {
	cards := []view.NWCard{{
		Label: base.T("networth.card.total"), Value: base.Money(d.Total), Mod: "hl",
		Help:     base.T("networth.card.total_help"),
		HasDelta: d.TotalHasPrev, DeltaStr: base.Money(absDelta(d.TotalDelta)),
		DeltaPos: d.TotalDelta > 0, DeltaNeg: d.TotalDelta < 0,
	}}
	var livrets []services.NWSupport
	supportsWithSnapshot := 0
	for _, s := range d.Supports {
		if !s.HasSnapshot {
			continue
		}
		supportsWithSnapshot++
		if s.Type == domain.AccountPassbook {
			livrets = append(livrets, s)
		}
	}
	sortByValueDesc(livrets)
	shown := 0
	var shownValue, shownDelta int64
	for i, lv := range livrets {
		if i >= 2 {
			break
		}
		cards = append(cards, h.livretCard(base, lv, d.NearCap))
		shown++
		shownValue += lv.Value
		if lv.HasPrev {
			shownDelta += lv.Delta
		}
	}
	// "Le reste" — everything not individually carded (PEA net + the rest).
	if supportsWithSnapshot > shown {
		restValue := d.Total - shownValue
		restDelta := d.TotalDelta - shownDelta
		cards = append(cards, view.NWCard{
			Label: base.T("networth.card.rest"), Value: base.Money(restValue), Mod: "good",
			Help:     base.T("networth.card.rest_help"),
			HasDelta: d.TotalHasPrev, DeltaStr: base.Money(absDelta(restDelta)),
			DeltaPos: restDelta > 0, DeltaNeg: restDelta < 0,
		})
	}
	return cards
}

func (h *Handlers) livretCard(base view.Base, lv services.NWSupport, nearCap int) view.NWCard {
	card := view.NWCard{Label: lv.Name, Value: base.Money(lv.Gross)}
	if lv.Ceiling != nil && *lv.Ceiling > 0 {
		pct := int(lv.Gross * 100 / *lv.Ceiling)
		switch {
		case lv.Gross >= *lv.Ceiling:
			card.CapText = base.T("networth.cap.full")
		case pct*100 >= nearCap:
			card.CapText = base.T("networth.cap.near")
		default:
			card.CapText = base.T("networth.cap.pct", strconv.Itoa(pct))
		}
		return card
	}
	card.HasDelta = lv.HasPrev
	card.DeltaStr = base.Money(absDelta(lv.Delta))
	card.DeltaPos = lv.Delta > 0
	card.DeltaNeg = lv.Delta < 0
	return card
}

// networthLines builds the snapshot table: each livret (editable) + the livrets
// subtotal + the PEA gross (editable) and derived net + each employee-savings
// holding (editable) + the total.
func (h *Handlers) networthLines(base view.Base, d *services.NetWorthData) []view.NWLine {
	var lines []view.NWLine
	// passbook livrets, then their subtotal
	for _, s := range d.Supports {
		if s.Type != domain.AccountPassbook {
			continue
		}
		lines = append(lines, h.supportLine(base, d, s, "sav"))
	}
	lines = append(lines, view.NWLine{
		Kind: "subtotal", RowClass: "sub-row", Label: base.T("networth.subtotal"),
		ValueStr: base.Amount(d.Subtotal),
		DeltaStr: deltaStr(base, d.SubtotalDelta, d.TotalHasPrev),
		DeltaPos: d.TotalHasPrev && d.SubtotalDelta > 0, DeltaNeg: d.TotalHasPrev && d.SubtotalDelta < 0,
		DeltaDash: !d.TotalHasPrev,
	})
	// PEA gross (editable) + derived net
	for _, s := range d.Supports {
		if s.Type != domain.AccountSecurities {
			continue
		}
		gross := h.supportLine(base, d, s, "pea")
		gross.Kind = "pea_gross"
		gross.Label = base.T("networth.pea_gross")
		gross.ValueStr = base.Amount(s.Gross)
		gross.DeltaStr = deltaStr(base, s.GrossDelta, s.HasPrev)
		gross.DeltaPos = s.HasPrev && s.GrossDelta > 0
		gross.DeltaNeg = s.HasPrev && s.GrossDelta < 0
		gross.DeltaDash = !s.HasPrev
		lines = append(lines, gross)
		lines = append(lines, view.NWLine{
			Kind: "pea_net", Label: base.T("networth.pea_net"), Indent: true,
			Ann: base.T("networth.pea_net_ann"), ValueStr: base.Amount(s.Value),
			DeltaStr: deltaStr(base, s.Delta, s.HasPrev),
			DeltaPos: s.HasPrev && s.Delta > 0, DeltaNeg: s.HasPrev && s.Delta < 0,
			DeltaDash: !s.HasPrev,
		})
	}
	// employee savings
	for _, s := range d.Supports {
		if s.Type != domain.AccountEmployeeSavings {
			continue
		}
		lines = append(lines, h.supportLine(base, d, s, "emp"))
	}
	// total
	lines = append(lines, view.NWLine{
		Kind: "total", RowClass: "tot-row", Label: base.T("networth.total"),
		ValueStr: base.Amount(d.Total),
		DeltaStr: deltaStr(base, d.TotalDelta, d.TotalHasPrev),
		DeltaPos: d.TotalHasPrev && d.TotalDelta > 0, DeltaNeg: d.TotalHasPrev && d.TotalDelta < 0,
		DeltaDash: !d.TotalHasPrev,
	})
	return lines
}

// supportLine builds one editable gross-snapshot row. dotClass is the fixed
// palette class for the colour dot ("sav" | "pea" | "emp") — emitted as a CSS
// class, never an inline style (html/template strips a `var(--…)` style value
// to ZgotmplZ, and CSP forbids inline style).
func (h *Handlers) supportLine(base view.Base, d *services.NetWorthData, s services.NWSupport, dotClass string) view.NWLine {
	return view.NWLine{
		Kind: "support", Label: s.Name, DotClass: dotClass,
		Editable: true, AccountID: s.AccountID, Period: d.Period,
		SnapshotID: s.SnapshotID, HasSnapshot: s.HasSnapshot, DelTitle: base.T("action.delete"),
		ValueStr:  base.Amount(s.Gross),
		DeltaStr:  deltaStr(base, s.Delta, s.HasPrev),
		DeltaPos:  s.HasPrev && s.Delta > 0,
		DeltaNeg:  s.HasPrev && s.Delta < 0,
		DeltaDash: !s.HasPrev,
	}
}

func (h *Handlers) registerView(r *http.Request, d *services.RegisterData) (view.RegisterView, error) {
	base := h.base(r)
	c := middleware.From(r.Context())
	v := view.RegisterView{
		Base:       base,
		Email:      c.User.Email,
		Nav:        "patrimoine",
		Tab:        "registre",
		Period:     d.Period,
		MonthLabel: h.monthLabel(base, d.Period),
		PrevPeriod: shiftPeriod(d.Period, -1),
		NextPeriod: shiftPeriod(d.Period, +1),
		Range:      d.Range,
		HasHistory: d.HasHistory,
	}
	h.fillNavigator(base, d.Period, r, &v.YearLabel, &v.MonthIndex, &v.PrevYearPeriod, &v.NextYearPeriod, &v.MonthCells, &v.PickerOpen)
	if err := h.fillRail(r, &v.CurAccounts, &v.Savings); err != nil {
		return v, err
	}
	for _, row := range d.Rows {
		rr := view.RRow{
			MonthLabel: h.monthLabel(base, row.Period),
			Period:     row.Period,
			LivretsStr: base.Amount(row.Livrets),
			TotalStr:   base.Amount(row.Total),
			Comment:    row.Comment,
			CommentSet: row.Comment != "",
			Current:    row.Period == d.Period,
			DeltaStr:   deltaStr(base, row.TotalDelta, row.HasPrev),
			DeltaPos:   row.HasPrev && row.TotalDelta > 0,
			DeltaNeg:   row.HasPrev && row.TotalDelta < 0,
			DeltaDash:  !row.HasPrev,
		}
		if row.HasPEA {
			rr.PEAStr = base.Amount(row.PEANet)
		} else {
			rr.PEAStr = "—"
		}
		v.Rows = append(v.Rows, rr)
	}
	if d.HasHistory {
		v.FooterStr = base.T("register.footer", strconv.Itoa(len(d.Rows)), h.monthLabel(base, d.CurvePeriods[0]))
	}
	h.fillChart(base, d, &v)
	return v, nil
}

// fillChart builds the evolution-curve SVG + legend from the range-clipped series.
func (h *Handlers) fillChart(base view.Base, d *services.RegisterData, v *view.RegisterView) {
	if len(d.CurvePeriods) == 0 {
		return
	}
	palette := []string{"var(--c-sav)", "var(--brand)", "var(--warn)", "var(--bad)", "var(--muted)"}
	in := view.NWChartInput{EndLabel: base.Money(d.TotalLatest)}
	for _, p := range d.CurvePeriods {
		in.MonthLabels = append(in.MonthLabels, base.T("month."+monthNum(p)))
	}
	for i, s := range d.Series {
		if s.IsTotal {
			in.Series = append(in.Series, view.NWChartSeries{Color: "var(--c-all)", Width: 3.5, Points: s.Points})
			v.Legend = append(v.Legend, view.NWChartLegend{Label: base.T("register.legend.total"), Color: "var(--c-all)"})
			continue
		}
		color := palette[(i-1)%len(palette)]
		if s.Type == domain.AccountSecurities {
			color = "var(--ok)"
		}
		in.Series = append(in.Series, view.NWChartSeries{Color: color, Width: 2, Dash: s.Type == domain.AccountSecurities, Points: s.Points})
		v.Legend = append(v.Legend, view.NWChartLegend{Label: s.Name, Color: color})
	}
	v.ChartSVG = view.RenderNetWorthChart(in)
}

// fillRail loads the current + savings accounts for the Patrimoine rail.
func (h *Handlers) fillRail(r *http.Request, cur *[]view.FScope, savings *[]view.FSaving) error {
	c := middleware.From(r.Context())
	base := h.base(r)
	current, sav, err := h.svc.RailAccounts(r.Context(), c.User.ID)
	if err != nil {
		return err
	}
	for _, a := range current {
		note := base.T("forecast.scope.carry")
		if a.MonthEndPolicy == domain.PolicySweep {
			note = base.T("forecast.scope.sweep")
		}
		*cur = append(*cur, view.FScope{Key: strconv.FormatInt(a.ID, 10), Name: a.Name, Note: note})
	}
	*savings = savingAccounts(base, sav)
	return nil
}

// fillNavigator populates the shared month-navigator fields.
func (h *Handlers) fillNavigator(base view.Base, period string, r *http.Request,
	yearLabel *string, monthIndex *int, prevYear, nextYear *string, cells *[]view.MonthCell, pickerOpen *bool,
) {
	if y, m, ok := splitPeriod(period); ok {
		*yearLabel = strconv.Itoa(y)
		*monthIndex = m - 1
		*prevYear = fmt.Sprintf("%04d-%02d", y-1, m)
		*nextYear = fmt.Sprintf("%04d-%02d", y+1, m)
		for i := 1; i <= 12; i++ {
			*cells = append(*cells, view.MonthCell{Period: fmt.Sprintf("%04d-%02d", y, i), Label: base.T("month." + strconv.Itoa(i)), On: i == m})
		}
	}
	*pickerOpen = strings.TrimSpace(r.URL.Query().Get("pick")) == "1"
}

// railSavings lists the user's savings accounts for the budget screens' rail
// Épargne section (O-21). Best-effort: a load error yields no section.
func (h *Handlers) railSavings(r *http.Request, base view.Base) []view.FSaving {
	c := middleware.From(r.Context())
	sav, err := h.svc.SavingsAccounts(r.Context(), c.User.ID)
	if err != nil {
		return nil
	}
	return savingAccounts(base, sav)
}

// savingAccounts maps savings accounts to the rail's Épargne entries (O-21).
func savingAccounts(base view.Base, accounts []domain.Account) []view.FSaving {
	var out []view.FSaving
	for _, a := range accounts {
		note := base.T("networth.rail.savings")
		if a.Type == domain.AccountSecurities {
			note = base.T("networth.rail.securities")
		}
		out = append(out, view.FSaving{ID: a.ID, Name: a.Name, Note: note})
	}
	return out
}

// prefillString assembles the M25 comment suggestion from the ranked movements
// (I-036): "Name +++", largest first, comma-separated.
func prefillString(ms []services.NWMovement) string {
	var parts []string
	for _, m := range ms {
		sign := "+"
		if !m.Up {
			sign = "−"
		}
		parts = append(parts, m.Name+" "+strings.Repeat(sign, m.Intensity))
	}
	return strings.Join(parts, ", ")
}

func deltaStr(base view.Base, delta int64, hasPrev bool) string {
	if !hasPrev {
		return "—"
	}
	if delta > 0 {
		return "+" + base.Amount(delta)
	}
	if delta < 0 {
		return "−" + base.Amount(-delta)
	}
	return base.Amount(0)
}

func absDelta(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func sortByValueDesc(s []services.NWSupport) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Gross > s[j-1].Gross; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func monthNum(period string) string {
	if _, m, ok := splitPeriod(period); ok {
		return strconv.Itoa(m)
	}
	return "1"
}
