package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"econome/internal/domain"
	"econome/internal/i18n"
	"econome/internal/server/middleware"
	"econome/internal/services"
	"econome/internal/view"
)

// Month-initialisation assistant handlers (functional/09, technical/04 §3.6). The
// draft is computed and rendered but never persisted until POST (T3i). Every
// figure is the engine's output for the current config + the submitted overrides
// (T4/T5 — no client-side computation, I-025).

// MonthInitGet renders the editable draft, or redirects to the forecast when the
// month already exists (functional/09 §5, I-027).
func (h *Handlers) MonthInitGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	period := h.period(r)
	created, err := h.svc.IsCreated(r.Context(), c.User.ID, period)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	if created {
		// The forecast screen lands in increment 6 (it reads the month from the
		// shared context); until then "/" is the budget landing (I-027). The period
		// is not echoed into the Location to avoid any tainted-redirect concern.
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	d, err := h.svc.BuildDraft(r.Context(), c.User.ID, period, h.parseOverrides(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.render(w, http.StatusOK, "month-init", h.monthInitView(r, d, h.scope(r)))
}

// MonthInitDraft recomputes the residual/total after a leaf adjustment and returns
// just the figures fragment (the engine recomputes server-side, I-025).
func (h *Handlers) MonthInitDraft(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	period := h.period(r)
	d, err := h.svc.BuildDraft(r.Context(), c.User.ID, period, h.parseOverrides(r))
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	mv := h.monthInitView(r, d, h.scope(r))
	h.render(w, http.StatusOK, "month-init-figures", mv)
}

// MonthInitCreate materialises the draft and the active period row, then redirects
// to the forecast (functional/09 §4).
func (h *Handlers) MonthInitCreate(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	period := h.period(r)
	if err := h.svc.CreateMonth(r.Context(), c.User.ID, period, h.parseOverrides(r)); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	// The forecast screen lands in increment 6 at "/" (the budget landing); until
	// then this redirect resolves to the shell without a 500 (I-027).
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- request parsing ---

// period resolves the focus month from the query, defaulting to the current month.
func (h *Handlers) period(r *http.Request) string {
	if p := strings.TrimSpace(r.URL.Query().Get("period")); p != "" {
		return p
	}
	if p := strings.TrimSpace(r.PostFormValue("period")); p != "" {
		return p
	}
	return h.svc.CurrentPeriod()
}

// scope resolves the rail account scope ("all" or an account id string).
func (h *Handlers) scope(r *http.Request) string {
	if s := strings.TrimSpace(r.URL.Query().Get("scope")); s != "" {
		return s
	}
	return "all"
}

// parseOverrides reads the submitted leaf amounts (amt_<envelopeID>) into a map of
// envelope id → minor units; unparseable/blank fields are skipped (the default is
// kept).
func (h *Handlers) parseOverrides(r *http.Request) map[int64]int64 {
	lang := middleware.From(r.Context()).Lang
	_ = r.ParseForm()
	out := map[int64]int64{}
	for key, vals := range r.Form {
		if !strings.HasPrefix(key, "amt_") || len(vals) == 0 {
			continue
		}
		id, err := strconv.ParseInt(key[len("amt_"):], 10, 64)
		if err != nil {
			continue
		}
		v := strings.TrimSpace(vals[0])
		if v == "" {
			continue
		}
		if m, err := i18n.ParseMoney(v, lang); err == nil && m >= 0 {
			out[id] = m
		}
	}
	return out
}

// --- view assembly ---

func (h *Handlers) monthInitView(r *http.Request, d *services.MonthDraft, scope string) view.MonthInitView {
	base := h.base(r)
	c := middleware.From(r.Context())
	acctName := map[int64]string{}
	for _, a := range d.Accounts {
		acctName[a.ID] = a.Name
	}

	mv := view.MonthInitView{
		Base:       base,
		Email:      c.User.Email,
		Nav:        "budget",
		Period:     d.Period,
		MonthLabel: h.monthLabel(base, d.Period),
		Scope:      scope,
		Scopes:     h.miScopes(base, d, scope),
		Empty:      len(d.Posts) == 0,
	}

	// Start cards: current accounts in scope.
	for _, a := range d.Accounts {
		if a.Type != domain.AccountCurrent {
			continue
		}
		if scope != "all" && strconv.FormatInt(a.ID, 10) != scope {
			continue
		}
		note := base.T("mi.start.carry_note")
		if a.MonthEndPolicy == domain.PolicySweep {
			note = base.T("mi.start.sweep_note")
		}
		mv.StartCards = append(mv.StartCards, view.MIStartCard{
			AccountID: a.ID, Name: a.Name, ValueStr: base.Money(d.StartByAccount[a.ID]), Note: note,
		})
	}

	// Posts in scope.
	for _, p := range d.Posts {
		if scope != "all" && strconv.FormatInt(p.AccountID, 10) != scope {
			continue
		}
		mv.Posts = append(mv.Posts, view.MIPost{
			EnvelopeID:  p.EnvelopeID,
			Name:        p.Name,
			AccountID:   p.AccountID,
			AccountName: p.AccountName,
			AmountStr:   base.Amount(p.Amount),
			GenClass:    genClass(p.Recurring),
			GenLabel:    h.genLabel(base, p),
			IsTransfer:  p.Flow == domain.FlowTransfer,
		})
	}

	mv.Figures = h.miFigures(base, d, scope, acctName)
	return mv
}

func (h *Handlers) miScopes(base view.Base, d *services.MonthDraft, scope string) []view.MIScope {
	scopes := []view.MIScope{{Key: "all", Name: base.T("shell.scope.all"), Note: base.T("mi.scope.aggregated"), IsAll: true, On: scope == "all"}}
	for _, a := range d.Accounts {
		if a.Type != domain.AccountCurrent {
			continue
		}
		note := base.T("mi.scope.carry")
		if a.MonthEndPolicy == domain.PolicySweep {
			note = base.T("mi.scope.sweep")
		}
		key := strconv.FormatInt(a.ID, 10)
		scopes = append(scopes, view.MIScope{Key: key, Name: a.Name, Note: note, On: scope == key})
	}
	return scopes
}

// miFigures builds the residual encarts (scope-dependent) and the footer total.
func (h *Handlers) miFigures(base view.Base, d *services.MonthDraft, scope string, acctName map[int64]string) view.MIFigures {
	var f view.MIFigures
	sweep := map[int64]bool{}
	for _, id := range d.Sweeps {
		sweep[id] = true
	}
	for _, a := range d.Accounts {
		if a.Type != domain.AccountCurrent {
			continue
		}
		if scope != "all" && strconv.FormatInt(a.ID, 10) != scope {
			continue
		}
		if sweep[a.ID] {
			f.Encarts = append(f.Encarts, sweepEncart(base, a.Name, d.SavingsBySweep[a.ID], acctName))
		} else if scope != "all" {
			// A carry account shows its "no savings — carried" note (only when it is
			// the selected scope; in the aggregated view carry accounts just report).
			f.Encarts = append(f.Encarts, view.MIEncart{Kind: "carry", AccountName: a.Name})
		}
	}

	var total int64
	for _, p := range d.Posts {
		if scope != "all" && strconv.FormatInt(p.AccountID, 10) != scope {
			continue
		}
		if p.Flow == domain.FlowExpense {
			total += p.Amount
		}
	}
	f.TotalStr = base.Money(total)
	if scope == "all" {
		f.TotalLabel = base.T("mi.total.all")
	} else {
		f.TotalLabel = base.T("mi.total.account", acctName[scopeID(scope)])
	}
	return f
}

// sweepEncart maps a sweep account's residual figures to its display band:
// cascade-full > negative-residual > normal residual (rules §7/§9/§11.1).
func sweepEncart(base view.Base, name string, sv services.DraftSavings, acctName map[int64]string) view.MIEncart {
	switch {
	case sv.CascadeFull:
		return view.MIEncart{Kind: "cascade", AccountName: name, AmountStr: base.Money(sv.Projected)}
	case sv.ResidualNegative:
		return view.MIEncart{Kind: "negative", AccountName: name, AmountStr: base.Money(sv.Projected)}
	default:
		target := ""
		if sv.CascadeTargetID != nil {
			target = acctName[*sv.CascadeTargetID]
		}
		return view.MIEncart{Kind: "residual", AccountName: name, TargetName: target, AmountStr: base.Money(sv.Projected)}
	}
}

func (h *Handlers) monthLabel(base view.Base, period string) string {
	if len(period) != 7 {
		return period
	}
	month := strings.TrimPrefix(period[5:], "0")
	return base.T("month."+month) + " " + period[:4]
}

func (h *Handlers) genLabel(base view.Base, p services.DraftPost) string {
	if !p.Recurring {
		return base.T("mi.gen.alloc")
	}
	if p.ExpectedDay != nil {
		return base.T("mi.gen.prevu") + " · " + strconv.Itoa(*p.ExpectedDay)
	}
	return base.T("mi.gen.prevu")
}

func genClass(recurring bool) string {
	if recurring {
		return "gen-prevu"
	}
	return "gen-alloc"
}

func scopeID(scope string) int64 {
	id, _ := strconv.ParseInt(scope, 10, 64)
	return id
}
