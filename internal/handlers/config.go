package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"econome/internal/domain"
	"econome/internal/i18n"
	"econome/internal/server/middleware"
	"econome/internal/services"
	"econome/internal/view"
)

// Configuration handlers — Paramètres (PR-a). Each parses the request, calls one
// service use-case, and renders a fragment. The userID comes only from
// TenantContext (never a request parameter); the central error mapper
// (mutationError) turns sentinels into status codes (G3).

// ParametersGet renders the full Paramètres screen.
func (h *Handlers) ParametersGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	v, err := h.parametersView(r, c.User.ID)
	if err != nil {
		slog.Error("parameters", "err", err)
		h.render(w, http.StatusInternalServerError, "parameters", v)
		return
	}
	h.render(w, http.StatusOK, "parameters", v)
}

// AccountFormGet renders the create/edit account modal fragment.
func (h *Handlers) AccountFormGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	base := h.base(r)
	fv := view.AccountFormView{
		Base:        base,
		Type:        string(domain.AccountCurrent),
		Policy:      string(domain.PolicySweep),
		IsCurrent:   true,
		TypeOptions: accountTypeOptions(base),
	}
	if idStr := r.PathValue("id"); idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		a, err := h.svc.GetAccount(r.Context(), c.User.ID, id)
		if err != nil {
			h.mutationError(w, r, err, nil)
			return
		}
		fv.IsEdit = true
		fv.ID = a.ID
		fv.Name = a.Name
		fv.Type = string(a.Type)
		fv.Policy = string(a.MonthEndPolicy)
		fv.IsCurrent = a.Type == domain.AccountCurrent
		fv.IsSavings = a.IsSavings()
		if a.Ceiling != nil {
			fv.CeilingStr = base.Amount(*a.Ceiling)
		}
	}
	fv.TypeLabel = base.T("account.type." + fv.Type)
	fv.PolicyLabel = policyLabel(base, domain.MonthEndPolicy(fv.Policy))
	h.render(w, http.StatusOK, "account-form", fv)
}

// AccountCreate handles POST /config/accounts.
func (h *Handlers) AccountCreate(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	in, perr := h.parseAccountForm(r)
	if perr != nil {
		h.renderAccountForm(w, r, false, 0, in, perr)
		return
	}
	if _, err := h.svc.CreateAccount(r.Context(), c.User.ID, in); err != nil {
		h.accountWriteFailure(w, r, false, 0, in, err)
		return
	}
	h.renderComptesOOB(w, r, c.User.ID)
}

// AccountUpdate handles PATCH /config/accounts/{id}.
func (h *Handlers) AccountUpdate(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	in, perr := h.parseAccountForm(r)
	if perr != nil {
		h.renderAccountForm(w, r, true, id, in, perr)
		return
	}
	if _, err := h.svc.UpdateAccount(r.Context(), c.User.ID, id, in); err != nil {
		h.accountWriteFailure(w, r, true, id, in, err)
		return
	}
	h.renderComptesOOB(w, r, c.User.ID)
}

// AccountArchive handles POST /config/accounts/{id}/archive.
func (h *Handlers) AccountArchive(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.svc.ArchiveAccount(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.renderComptesCard(w, r, c.User.ID)
}

// AccountUnarchive handles POST /config/accounts/{id}/unarchive.
func (h *Handlers) AccountUnarchive(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.svc.UnarchiveAccount(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.renderComptesCard(w, r, c.User.ID)
}

// AccountDelete handles DELETE /config/accounts/{id} (archives if dependents).
func (h *Handlers) AccountDelete(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := h.svc.DeleteAccount(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.renderComptesCard(w, r, c.User.ID)
}

// SettingsPatch handles PATCH /config/settings (Épargne / Localisation /
// Préférences cards; the hidden `card` field selects which to re-render).
func (h *Handlers) SettingsPatch(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	lang := c.Lang
	in := services.SettingsInput{}
	ve := &domain.ValidationError{}
	card := r.PostFormValue("card")

	switch card {
	case "epargne":
		if v := r.PostFormValue("default_account_id"); v != "" {
			id, _ := strconv.ParseInt(v, 10, 64)
			in.DefaultAccountID = &id
		}
		parseMoneyField(r, "pea_initial_deposit", lang, ve, func(m int64) { in.PEAInitialDeposit = &m })
		parseRateField(r, "pea_social_charge_rate", lang, ve, func(bp int) { in.PEASocialChargeRate = &bp })
		parseRateField(r, "near_cap_threshold", lang, ve, func(bp int) { in.NearCapThreshold = &bp })
		if v := r.PostFormValue("secured_savings_basis"); v != "" {
			in.SecuredSavingsBasis = &v
		}
	case "localisation":
		if v := r.PostFormValue("language"); v != "" {
			in.Language = &v
		}
		if v := r.PostFormValue("currency"); v != "" {
			in.Currency = &v
		}
	case "preferences":
		cp := r.PostFormValue("comment_autoprefill") == "1"
		in.CommentAutoprefill = &cp
		if v := r.PostFormValue("theme"); v != "" {
			in.Theme = &v
		}
	}

	if ve.HasErrors() {
		h.renderSettingsCard(w, r, c.User.ID, card, h.localizeFields(r, ve))
		return
	}
	if _, err := h.svc.UpdateSettings(r.Context(), c.User.ID, in); err != nil {
		var verr *domain.ValidationError
		if errors.As(err, &verr) {
			h.renderSettingsCard(w, r, c.User.ID, card, h.localizeFields(r, verr))
			return
		}
		h.mutationError(w, r, err, nil)
		return
	}
	h.renderSettingsCard(w, r, c.User.ID, card, nil)
}

// CascadeReorder handles POST /config/accounts/reorder (SortableJS drop / add /
// remove): `order` is a CSV of account ids in the new fill-priority order.
func (h *Handlers) CascadeReorder(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	ids := parseCSVIDs(r.PostFormValue("order"))
	if err := h.svc.ReorderCascade(r.Context(), c.User.ID, ids); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.renderCascade(w, r, c.User.ID)
}

// --- form parsing ---

func (h *Handlers) parseAccountForm(r *http.Request) (services.AccountInput, *domain.ValidationError) {
	lang := middleware.From(r.Context()).Lang
	in := services.AccountInput{
		Name:            strings.TrimSpace(r.PostFormValue("name")),
		Type:            r.PostFormValue("type"),
		MonthEndPolicy:  r.PostFormValue("month_end_policy"),
		EffectivePeriod: r.PostFormValue("effective_period"),
	}
	ve := &domain.ValidationError{}
	if v := strings.TrimSpace(r.PostFormValue("ceiling")); v != "" {
		m, err := i18n.ParseMoney(v, lang)
		if err != nil {
			ve.Add("ceiling", domain.MsgAmountInvalid)
		} else {
			in.Ceiling = &m
		}
	}
	if ve.HasErrors() {
		return in, ve
	}
	return in, nil
}

func parseMoneyField(r *http.Request, field string, lang domain.Language, ve *domain.ValidationError, set func(int64)) {
	v := strings.TrimSpace(r.PostFormValue(field))
	if v == "" {
		return
	}
	m, err := i18n.ParseMoney(v, lang)
	if err != nil {
		ve.Add(field, domain.MsgAmountInvalid)
		return
	}
	set(m)
}

func parseRateField(r *http.Request, field string, lang domain.Language, ve *domain.ValidationError, set func(int)) {
	v := strings.TrimSpace(r.PostFormValue(field))
	if v == "" {
		return
	}
	bp, err := i18n.ParsePercent(v, lang)
	if err != nil {
		ve.Add(field, domain.MsgRateInvalid)
		return
	}
	set(bp)
}

func parseCSVIDs(s string) []int64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		if id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// --- error mapping (G3) ---

// mutationError maps a service error to an HTTP status. A ValidationError is
// handled by onValidation (if provided); otherwise a generic status is written.
func (h *Handlers) mutationError(w http.ResponseWriter, r *http.Request, err error, onValidation func(map[string]string)) {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		if onValidation != nil {
			onValidation(h.localizeFields(r, ve))
			return
		}
		http.Error(w, "validation", http.StatusUnprocessableEntity)
	case errors.Is(err, domain.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, domain.ErrLocked):
		http.Error(w, h.t(r, "error.locked"), http.StatusConflict)
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrDuplicate):
		http.Error(w, h.t(r, "error.conflict"), http.StatusConflict)
	default:
		slog.Error("mutation", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handlers) accountWriteFailure(w http.ResponseWriter, r *http.Request, isEdit bool, id int64, in services.AccountInput, err error) {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		h.renderAccountForm(w, r, isEdit, id, in, ve)
		return
	}
	h.mutationError(w, r, err, nil)
}

// asValidation returns err as a *domain.ValidationError, or nil.
func asValidation(err error) *domain.ValidationError {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		return ve
	}
	return nil
}

func (h *Handlers) localizeFields(r *http.Request, ve *domain.ValidationError) map[string]string {
	lang := middleware.From(r.Context()).Lang
	out := make(map[string]string, len(ve.Fields))
	for _, f := range ve.Fields {
		out[f.Field] = h.rdr.Catalog().T(lang, f.MsgKey)
	}
	return out
}

func (h *Handlers) t(r *http.Request, key string) string {
	return h.rdr.Catalog().T(middleware.From(r.Context()).Lang, key)
}

// --- fragment rendering ---

func (h *Handlers) renderAccountForm(w http.ResponseWriter, r *http.Request, isEdit bool, id int64, in services.AccountInput, ve *domain.ValidationError) {
	base := h.base(r)
	t := domain.AccountType(in.Type)
	fv := view.AccountFormView{
		Base:        base,
		IsEdit:      isEdit,
		ID:          id,
		Name:        in.Name,
		Type:        in.Type,
		TypeLabel:   base.T("account.type." + in.Type),
		Policy:      in.MonthEndPolicy,
		PolicyLabel: policyLabel(base, domain.MonthEndPolicy(in.MonthEndPolicy)),
		IsCurrent:   t == domain.AccountCurrent,
		IsSavings:   t.Valid() && t != domain.AccountCurrent,
		TypeOptions: accountTypeOptions(base),
		FieldErrors: h.localizeFields(r, ve),
	}
	if in.Ceiling != nil {
		fv.CeilingStr = base.Amount(*in.Ceiling)
	}
	h.render(w, http.StatusUnprocessableEntity, "account-form", fv)
}

func (h *Handlers) renderComptesOOB(w http.ResponseWriter, r *http.Request, userID int64) {
	v, err := h.parametersView(r, userID)
	if err != nil {
		slog.Error("comptes-oob", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	h.render(w, http.StatusOK, "comptes-oob", v)
}

func (h *Handlers) renderComptesCard(w http.ResponseWriter, r *http.Request, userID int64) {
	v, err := h.parametersView(r, userID)
	if err != nil {
		slog.Error("comptes-card", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	h.render(w, http.StatusOK, "comptes-card", v)
}

func (h *Handlers) renderCascade(w http.ResponseWriter, r *http.Request, userID int64) {
	v, err := h.parametersView(r, userID)
	if err != nil {
		slog.Error("cascade", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	h.render(w, http.StatusOK, "cascade-card", v)
}

func (h *Handlers) renderSettingsCard(w http.ResponseWriter, r *http.Request, userID int64, card string, fieldErrors map[string]string) {
	v, err := h.parametersView(r, userID)
	if err != nil {
		slog.Error("settings-card", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	v.FieldErrors = fieldErrors
	status := http.StatusOK
	if len(fieldErrors) > 0 {
		status = http.StatusUnprocessableEntity
	}
	name := "card-" + card
	switch card {
	case "epargne", "localisation", "preferences":
	default:
		name = "parameters"
	}
	h.render(w, status, name, v)
}

// --- view-model assembly ---

func (h *Handlers) parametersView(r *http.Request, userID int64) (view.ParametersView, error) {
	base := h.base(r)
	c := middleware.From(r.Context())
	v := view.ParametersView{
		Base:            base,
		Email:           c.User.Email,
		Nav:             "configuration",
		IsAdmin:         c.IsAdmin,
		LangOptions:     langOptions(base),
		CurrencyOptions: currencyOptions(base),
	}

	accounts, err := h.svc.ListAccounts(r.Context(), userID)
	if err != nil {
		return v, err
	}
	settings, err := h.svc.Settings(r.Context(), userID)
	if err != nil {
		return v, err
	}

	var cascade []view.CascadeRow
	for _, a := range accounts {
		if a.Status == domain.ArchiveArchived {
			v.ArchivedCount++
		}
		row := view.AccountRow{
			ID:         a.ID,
			Name:       a.Name,
			TypeLabel:  base.T("account.type." + string(a.Type)),
			IsCurrent:  a.Type == domain.AccountCurrent,
			PolicyCode: string(a.MonthEndPolicy),
			Archived:   a.Status == domain.ArchiveArchived,
			CeilingStr: "—",
		}
		if a.Type == domain.AccountCurrent {
			switch a.MonthEndPolicy {
			case domain.PolicySweep:
				row.ChipClass, row.PolicyText = "fin-sweep", base.T("account.policy.sweep_chip")
			case domain.PolicyCarry:
				row.ChipClass, row.PolicyText = "fin-carry", base.T("account.policy.carry_chip")
			case domain.PolicyNone:
				row.PolicyText = "—"
			}
			v.CurrentOptions = append(v.CurrentOptions, view.SelectOption{Value: strconv.FormatInt(a.ID, 10), Label: a.Name})
		} else {
			row.PolicyText = "—"
		}
		if a.Ceiling != nil {
			row.CeilingStr = base.Amount(*a.Ceiling)
		}
		v.Accounts = append(v.Accounts, row)

		if a.IsSavings() && a.Status == domain.ArchiveActive {
			if a.FillPriority != nil {
				cascade = append(cascade, view.CascadeRow{ID: a.ID, Name: a.Name, Order: *a.FillPriority})
			} else {
				v.CascadeAddable = append(v.CascadeAddable, view.SelectOption{Value: strconv.FormatInt(a.ID, 10), Label: a.Name})
			}
		}
	}
	sort.Slice(cascade, func(i, j int) bool { return cascade[i].Order < cascade[j].Order })
	v.Cascade = cascade

	v.Settings = view.SettingsVM{
		PEAInitialStr: base.Amount(settings.PEAInitialDeposit),
		PEARateStr:    base.Rate(settings.PEASocialChargeRate),
		NearCapStr:    base.Rate(settings.NearCapThreshold),
		Basis:         string(settings.SecuredSavingsBasis),
		Comment:       settings.CommentAutoprefill,
		Language:      string(settings.Language),
		LanguageLabel: base.T("lang." + string(settings.Language)),
		Currency:      settings.Currency,
		CurrencyLabel: base.T("currency." + settings.Currency),
		ThemeDark:     settings.Theme == domain.ThemeDark,
	}
	if settings.DefaultAccountID != nil {
		v.Settings.DefaultAccountID = *settings.DefaultAccountID
	}

	if err := h.securityPanel(r, &v, userID); err != nil {
		return v, err
	}
	return v, nil
}

// securityPanel fills the Security (self) + Users (admin) panel data.
func (h *Handlers) securityPanel(r *http.Request, v *view.ParametersView, userID int64) error {
	base := h.base(r)
	c := middleware.From(r.Context())
	ctx := r.Context()

	v.TOTPEnabled = c.User.TOTPEnabled
	if v.TOTPEnabled {
		n, err := h.svc.BackupCodesRemaining(ctx, userID)
		if err != nil {
			return err
		}
		v.BackupRemaining = n
	}

	curHash := ""
	if c.Session != nil {
		curHash = c.Session.TokenHash
	}
	sessions, err := h.svc.ListSessions(ctx, userID, curHash)
	if err != nil {
		return err
	}
	for _, sv := range sessions {
		v.Sessions = append(v.Sessions, view.SessionRow{
			ID:       sv.Session.ID,
			Device:   deviceLabel(sv.Session.UserAgent),
			IP:       strOrDash(sv.Session.IP),
			LastSeen: sv.Session.LastSeenAt.Format("2006-01-02 15:04"),
			Current:  sv.Current,
		})
	}

	if !c.IsAdmin {
		return nil
	}
	users, err := h.svc.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		row := view.UserRow{
			ID: u.ID, Email: u.Email, IsSelf: u.ID == userID,
			Deactivated: u.Status == domain.StatusDeactivated, TOTPEnabled: u.TOTPEnabled,
		}
		if u.IsAdmin {
			row.RoleLabel, row.RoleClass = base.T("users.role.admin"), "info"
		} else {
			row.RoleLabel, row.RoleClass = base.T("users.role.member"), "mut"
		}
		if row.Deactivated {
			row.StatusLabel, row.StatusClass = base.T("users.status.deactivated"), "mut"
		} else {
			row.StatusLabel, row.StatusClass = base.T("users.status.active"), "ok"
		}
		v.Users = append(v.Users, row)
	}
	invs, err := h.svc.ListInvitations(ctx, userID)
	if err != nil {
		return err
	}
	now := h.svc.Now()
	for _, inv := range invs {
		row := view.InvitationRow{ID: inv.ID, Email: strOrDash(inv.Email)}
		switch {
		case inv.ConsumedAt != nil:
			row.StatusLabel, row.StatusClass = base.T("users.inv.accepted"), "ok"
		case inv.RevokedAt != nil:
			row.StatusLabel, row.StatusClass = base.T("users.inv.revoked"), "mut"
		case now.After(inv.ExpiresAt):
			row.StatusLabel, row.StatusClass = base.T("users.inv.expired"), "mut"
		default:
			row.StatusLabel, row.StatusClass, row.Pending = base.T("users.inv.pending"), "warn", true
		}
		v.Invitations = append(v.Invitations, row)
	}
	return nil
}

func deviceLabel(ua *string) string {
	if ua == nil || *ua == "" {
		return "—"
	}
	s := *ua
	if len(s) > 60 {
		s = s[:60] + "…"
	}
	return s
}

func strOrDash(p *string) string {
	if p == nil || *p == "" {
		return "—"
	}
	return *p
}

// --- option builders ---

func accountTypeOptions(base view.Base) []view.SelectOption {
	types := []domain.AccountType{domain.AccountCurrent, domain.AccountPassbook, domain.AccountSecurities, domain.AccountEmployeeSavings}
	out := make([]view.SelectOption, 0, len(types))
	for _, t := range types {
		out = append(out, view.SelectOption{Value: string(t), Label: base.T("account.type." + string(t))})
	}
	return out
}

func langOptions(base view.Base) []view.SelectOption {
	return []view.SelectOption{
		{Value: "fr", Label: base.T("lang.fr")},
		{Value: "en", Label: base.T("lang.en")},
	}
}

func currencyOptions(base view.Base) []view.SelectOption {
	codes := []string{"EUR", "USD", "GBP", "CHF"}
	out := make([]view.SelectOption, 0, len(codes))
	for _, code := range codes {
		out = append(out, view.SelectOption{Value: code, Label: base.T("currency." + code)})
	}
	return out
}

func policyLabel(base view.Base, p domain.MonthEndPolicy) string {
	switch p {
	case domain.PolicySweep:
		return base.T("account.policy.sweep")
	case domain.PolicyCarry:
		return base.T("account.policy.carry")
	case domain.PolicyNone:
		return base.T("account.policy.none")
	default:
		return base.T("account.policy.none")
	}
}
