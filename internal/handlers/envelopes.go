package handlers

import (
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

// Configuration handlers — Envelopes screen (PR-b). Reuses the PR-a foundations:
// the #modal-host choreography, the central mutationError mapper, and the
// CSP-clean app.js. The combined form writes a leaf category (+ optional new
// parent) and an envelope in one transaction (I-021).

// EnvelopesGet renders the full Envelopes screen.
func (h *Handlers) EnvelopesGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	v, err := h.envelopesView(r, c.User.ID)
	if err != nil {
		slog.Error("envelopes", "err", err)
		h.render(w, http.StatusInternalServerError, "envelopes", v)
		return
	}
	h.render(w, http.StatusOK, "envelopes", v)
}

// EnvelopeFormGet renders the create/edit envelope modal.
func (h *Handlers) EnvelopeFormGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	base := h.base(r)
	fv := view.EnvelopeFormView{
		Base:      base,
		FlowType:  string(domain.FlowExpense),
		Mode:      string(domain.ModeVariable),
		Frequency: string(domain.FreqMonthly),
	}
	if err := h.fillEnvelopeFormOptions(r, c.User.ID, &fv); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	if idStr := r.PathValue("id"); idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		e, cat, err := h.svc.GetEnvelope(r.Context(), c.User.ID, id)
		if err != nil {
			h.mutationError(w, r, err, nil)
			return
		}
		fv.IsEdit = true
		fv.ID = e.ID
		fv.Name = cat.Name
		fv.FlowType = string(cat.FlowType)
		fv.Mode = string(e.Mode)
		fv.AccountID = e.AccountID
		fv.DefaultExpanded = cat.DefaultExpanded
		if cat.ParentID != nil {
			fv.ParentID = *cat.ParentID
		}
		if e.DefaultAmount != nil {
			fv.DefaultStr = base.Amount(*e.DefaultAmount)
		}
		if e.Frequency != nil {
			fv.Frequency = string(*e.Frequency)
		}
		if len(e.DueMonths) > 0 {
			fv.DueMonth = strconv.Itoa(e.DueMonths[0])
		}
		if e.ExpectedDay != nil {
			fv.ExpectedDayStr = strconv.Itoa(*e.ExpectedDay)
		}
	}
	setEnvelopeFormFlags(&fv)
	h.render(w, http.StatusOK, "envelope-form", fv)
}

// EnvelopeCreate handles POST /config/envelopes.
func (h *Handlers) EnvelopeCreate(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	in, perr := h.parseEnvelopeForm(r)
	if perr != nil {
		h.renderEnvelopeForm(w, r, false, 0, in, perr)
		return
	}
	if _, err := h.svc.CreateEnvelope(r.Context(), c.User.ID, in); err != nil {
		h.envelopeWriteFailure(w, r, false, 0, in, err)
		return
	}
	h.renderEnvelopesOOB(w, r, c.User.ID)
}

// EnvelopeUpdate handles PATCH /config/envelopes/{id}.
func (h *Handlers) EnvelopeUpdate(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	in, perr := h.parseEnvelopeForm(r)
	if perr != nil {
		h.renderEnvelopeForm(w, r, true, id, in, perr)
		return
	}
	if _, err := h.svc.UpdateEnvelope(r.Context(), c.User.ID, id, in); err != nil {
		h.envelopeWriteFailure(w, r, true, id, in, err)
		return
	}
	h.renderEnvelopesOOB(w, r, c.User.ID)
}

// EnvelopeArchive handles POST /config/envelopes/{id}/archive.
func (h *Handlers) EnvelopeArchive(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.svc.ArchiveEnvelope(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.renderEnvelopesList(w, r, c.User.ID)
}

// EnvelopeUnarchive handles POST /config/envelopes/{id}/unarchive.
func (h *Handlers) EnvelopeUnarchive(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.svc.UnarchiveEnvelope(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.renderEnvelopesList(w, r, c.User.ID)
}

// EnvelopeDelete handles DELETE /config/envelopes/{id} (archives if dependents).
func (h *Handlers) EnvelopeDelete(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := h.svc.DeleteEnvelope(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.renderEnvelopesList(w, r, c.User.ID)
}

// --- form parsing ---

func (h *Handlers) parseEnvelopeForm(r *http.Request) (services.EnvelopeInput, *domain.ValidationError) {
	lang := middleware.From(r.Context()).Lang
	in := services.EnvelopeInput{
		Name:            strings.TrimSpace(r.PostFormValue("name")),
		FlowType:        r.PostFormValue("flow_type"),
		Mode:            r.PostFormValue("mode"),
		Frequency:       r.PostFormValue("frequency"),
		DefaultExpanded: r.PostFormValue("default_expanded") == "1",
	}
	if v := r.PostFormValue("account_id"); v != "" {
		in.AccountID, _ = strconv.ParseInt(v, 10, 64)
	}
	// Parent: "__new__" reveals the name input; "0"/"" = none; else an existing id.
	switch pv := r.PostFormValue("parent_id"); pv {
	case "", "0":
	case "__new__":
		in.NewParentName = strings.TrimSpace(r.PostFormValue("new_parent_name"))
	default:
		if pid, err := strconv.ParseInt(pv, 10, 64); err == nil {
			in.ParentID = &pid
		}
	}

	ve := &domain.ValidationError{}
	if v := strings.TrimSpace(r.PostFormValue("default_amount")); v != "" && in.Mode != string(domain.ModeResidual) {
		m, err := i18n.ParseMoney(v, lang)
		if err != nil {
			ve.Add("default_amount", domain.MsgAmountInvalid)
		} else {
			in.DefaultAmount = &m
		}
	}
	if v := strings.TrimSpace(r.PostFormValue("expected_day")); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil {
			ve.Add("expected_day", domain.MsgExpectedDayInvalid)
		} else {
			in.ExpectedDay = &d
		}
	}
	if v := strings.TrimSpace(r.PostFormValue("due_month")); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			in.DueMonths = []int{m}
		}
	}
	if ve.HasErrors() {
		return in, ve
	}
	return in, nil
}

func (h *Handlers) envelopeWriteFailure(w http.ResponseWriter, r *http.Request, isEdit bool, id int64, in services.EnvelopeInput, err error) {
	if ve := asValidation(err); ve != nil {
		h.renderEnvelopeForm(w, r, isEdit, id, in, ve)
		return
	}
	h.mutationError(w, r, err, nil)
}

// --- fragment rendering ---

func (h *Handlers) renderEnvelopeForm(w http.ResponseWriter, r *http.Request, isEdit bool, id int64, in services.EnvelopeInput, ve *domain.ValidationError) {
	c := middleware.From(r.Context())
	base := h.base(r)
	fv := view.EnvelopeFormView{
		Base:            base,
		IsEdit:          isEdit,
		ID:              id,
		Name:            in.Name,
		FlowType:        in.FlowType,
		Mode:            in.Mode,
		AccountID:       in.AccountID,
		Frequency:       in.Frequency,
		DefaultExpanded: in.DefaultExpanded,
		FieldErrors:     h.localizeFields(r, ve),
	}
	if in.ParentID != nil {
		fv.ParentID = *in.ParentID
	}
	if in.DefaultAmount != nil {
		fv.DefaultStr = base.Amount(*in.DefaultAmount)
	}
	if in.ExpectedDay != nil {
		fv.ExpectedDayStr = strconv.Itoa(*in.ExpectedDay)
	}
	if len(in.DueMonths) > 0 {
		fv.DueMonth = strconv.Itoa(in.DueMonths[0])
	}
	if err := h.fillEnvelopeFormOptions(r, c.User.ID, &fv); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	setEnvelopeFormFlags(&fv)
	h.render(w, http.StatusUnprocessableEntity, "envelope-form", fv)
}

func (h *Handlers) renderEnvelopesOOB(w http.ResponseWriter, r *http.Request, userID int64) {
	v, err := h.envelopesView(r, userID)
	if err != nil {
		slog.Error("envelopes-oob", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	h.render(w, http.StatusOK, "envelopes-oob", v)
}

func (h *Handlers) renderEnvelopesList(w http.ResponseWriter, r *http.Request, userID int64) {
	v, err := h.envelopesView(r, userID)
	if err != nil {
		slog.Error("envelopes-list", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	h.render(w, http.StatusOK, "envelopes-list", v)
}

// --- view-model assembly ---

func (h *Handlers) envelopesView(r *http.Request, userID int64) (view.EnvelopesView, error) {
	base := h.base(r)
	c := middleware.From(r.Context())
	v := view.EnvelopesView{Base: base, Email: c.User.Email, Nav: "configuration"}

	ov, err := h.svc.EnvelopesOverview(r.Context(), userID, true)
	if err != nil {
		return v, err
	}
	v.HasArchived = ov.HasArchived
	for _, g := range ov.Parents {
		pg := view.ParentGroupVM{
			Key:        "cat-" + strconv.FormatInt(g.Category.ID, 10),
			Name:       g.Category.Name,
			ChildCount: len(g.Children),
			SumStr:     base.Money(g.SumDefault),
			Expanded:   g.Category.DefaultExpanded,
		}
		for _, row := range g.Children {
			pg.Children = append(pg.Children, h.envelopeRowVM(base, row))
		}
		v.Parents = append(v.Parents, pg)
	}
	for _, row := range ov.TopLevel {
		v.TopLevel = append(v.TopLevel, h.envelopeRowVM(base, row))
	}
	v.IsEmpty = len(v.Parents) == 0 && len(v.TopLevel) == 0
	return v, nil
}

func (h *Handlers) envelopeRowVM(base view.Base, row services.EnvelopeRow) view.EnvelopeRowVM {
	cls, label := envelopeBadge(base, row.Envelope.Mode, row.Category.FlowType)
	vm := view.EnvelopeRowVM{
		ID:          row.Envelope.ID,
		Name:        row.Category.Name,
		AccountName: row.AccountName,
		BadgeClass:  cls,
		BadgeLabel:  label,
		FreqLabel:   freqLabel(base, row.Envelope),
		DefaultStr:  "—",
		DayStr:      "—",
		Archived:    row.Envelope.Status == domain.ArchiveArchived,
	}
	switch {
	case row.Envelope.Mode == domain.ModeResidual:
		vm.DefaultStr = base.T("env.auto")
	case row.Envelope.DefaultAmount != nil:
		vm.DefaultStr = base.Money(*row.Envelope.DefaultAmount)
	}
	if row.Envelope.ExpectedDay != nil {
		vm.DayStr = strconv.Itoa(*row.Envelope.ExpectedDay)
	}
	return vm
}

func (h *Handlers) fillEnvelopeFormOptions(r *http.Request, userID int64, fv *view.EnvelopeFormView) error {
	base := h.base(r)
	accts, err := h.svc.ListAccounts(r.Context(), userID)
	if err != nil {
		return err
	}
	for _, a := range accts {
		if a.Status == domain.ArchiveActive {
			fv.AccountOptions = append(fv.AccountOptions, view.SelectOption{Value: strconv.FormatInt(a.ID, 10), Label: a.Name})
		}
	}
	parents, err := h.svc.ParentOptions(r.Context(), userID)
	if err != nil {
		return err
	}
	for _, p := range parents {
		fv.ParentOptions = append(fv.ParentOptions, view.SelectOption{Value: strconv.FormatInt(p.ID, 10), Label: p.Name})
	}
	fv.FlowOptions = []view.SelectOption{
		{Value: "expense", Label: base.T("flow.expense")},
		{Value: "income", Label: base.T("flow.income")},
		{Value: "transfer", Label: base.T("flow.transfer")},
	}
	fv.ModeOptions = []view.SelectOption{
		{Value: "variable", Label: base.T("envelope.mode.variable")},
		{Value: "fixed_recurring", Label: base.T("envelope.mode.fixed")},
		{Value: "residual", Label: base.T("envelope.mode.residual")},
	}
	fv.FreqOptions = []view.SelectOption{
		{Value: "monthly", Label: base.T("freq.monthly")},
		{Value: "quarterly", Label: base.T("freq.quarterly")},
		{Value: "semiannual", Label: base.T("freq.semiannual")},
		{Value: "annual", Label: base.T("freq.annual")},
	}
	for m := 1; m <= 12; m++ {
		fv.MonthOptions = append(fv.MonthOptions, view.SelectOption{Value: strconv.Itoa(m), Label: base.T("month." + strconv.Itoa(m))})
	}
	return nil
}

func setEnvelopeFormFlags(fv *view.EnvelopeFormView) {
	fv.IsResidual = fv.Mode == string(domain.ModeResidual)
	fv.IsFixed = fv.Mode == string(domain.ModeFixedRecurring)
	fv.NonMonthly = fv.IsFixed && fv.Frequency != string(domain.FreqMonthly)
}

// envelopeBadge picks the mode/flow badge: residual > income/transfer flow > mode.
func envelopeBadge(base view.Base, mode domain.Mode, flow domain.FlowType) (class, label string) {
	switch {
	case mode == domain.ModeResidual:
		return "mb res", base.T("envelope.mode.residual")
	case flow == domain.FlowIncome:
		return "mb rev", base.T("flow.income")
	case flow == domain.FlowTransfer:
		return "mb xfer", base.T("flow.transfer")
	case mode == domain.ModeFixedRecurring:
		return "mb fixe", base.T("envelope.mode.fixed")
	default:
		return "mb var", base.T("envelope.mode.variable")
	}
}

func freqLabel(base view.Base, e domain.Envelope) string {
	if e.Mode != domain.ModeFixedRecurring || e.Frequency == nil {
		return "—"
	}
	var label string
	switch *e.Frequency {
	case domain.FreqMonthly:
		return base.T("freq.monthly")
	case domain.FreqQuarterly:
		label = base.T("freq.quarterly")
	case domain.FreqSemiannual:
		label = base.T("freq.semiannual")
	case domain.FreqAnnual:
		label = base.T("freq.annual")
	default:
		return "—"
	}
	if len(e.DueMonths) > 0 {
		names := make([]string, 0, len(e.DueMonths))
		for _, m := range e.DueMonths {
			names = append(names, base.T("month."+strconv.Itoa(m)))
		}
		return label + " (" + strings.Join(names, ", ") + ")"
	}
	return label
}
