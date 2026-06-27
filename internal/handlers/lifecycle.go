package handlers

import (
	"net/http"
	"net/url"
	"regexp"

	"econome/internal/server/middleware"
)

// periodRe bounds the period path value to YYYY-MM so it cannot carry a redirect
// target (gosec G710 — the redirect destination is always app-relative).
var periodRe = regexp.MustCompile(`^\d{4}-\d{2}$`)

// Month-lifecycle controls (functional/04 §4, technical/04 §3.7). Each handler
// performs exactly one service use-case and bounces the user back to the budget
// screen so the new period state (and the regenerated rows) render. htmx requests
// get an HX-Redirect; a plain POST gets a 303.

// PeriodLock closes (locks) the month, then reloads the budget screen.
func (h *Handlers) PeriodLock(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	period := r.PathValue("period")
	if err := h.svc.LockMonth(r.Context(), c.User.ID, period, c.User.ID); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.redirectBudget(w, r, period)
}

// PeriodUnlock returns the month to ACTIVE, then reloads the budget screen.
func (h *Handlers) PeriodUnlock(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	period := r.PathValue("period")
	if err := h.svc.UnlockMonth(r.Context(), c.User.ID, period, c.User.ID); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.redirectBudget(w, r, period)
}

// PeriodRegenerate adds the missing recurring/variable lines (L9), then reloads.
func (h *Handlers) PeriodRegenerate(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	period := r.PathValue("period")
	if _, err := h.svc.RegenerateMissingRecurring(r.Context(), c.User.ID, period); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	h.redirectBudget(w, r, period)
}

// redirectBudget bounces back to the originating budget screen (forecast unless
// from=journal), preserving the month + scope. htmx → HX-Redirect; else 303.
func (h *Handlers) redirectBudget(w http.ResponseWriter, r *http.Request, period string) {
	// Build an app-relative destination only; the period/scope are sanitised so
	// no caller-supplied value can redirect off-site (gosec G710).
	dest := "/"
	if r.PostFormValue("from") == "journal" {
		dest = "/journal"
	}
	if !periodRe.MatchString(period) {
		period = ""
	}
	scope := r.PostFormValue("scope")
	if !periodScopeRe.MatchString(scope) {
		scope = "all"
	}
	q := url.Values{"period": {period}, "scope": {scope}}
	dest += "?" + q.Encode()
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", dest)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// periodScopeRe bounds the rail scope to "all" or a numeric account id.
var periodScopeRe = regexp.MustCompile(`^(all|\d+)$`)
