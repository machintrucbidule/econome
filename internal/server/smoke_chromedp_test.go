//go:build chromedp

// The chromedp browser smoke runs only under the `chromedp` build tag (CI's
// e2e-chrome job, O-7) because it needs a headless Chrome. The httptest-based
// e2e backbone in e2e_test.go runs everywhere and is the primary coverage; this
// adds a real-browser check that login renders the three-pane shell.
package server_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestSmokeLoginRendersShell(t *testing.T) {
	ts, _ := newTestServer(t)
	seedOwner(t, ts)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("no-sandbox", true))...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, 40*time.Second)
	defer cancelT()

	var shellClass, h1Text string
	err := chromedp.Run(
		ctx,
		chromedp.Navigate(ts.URL+"/login"),
		chromedp.WaitVisible(`#email`, chromedp.ByID),
		chromedp.SendKeys(`#email`, "owner@example.org", chromedp.ByID),
		chromedp.SendKeys(`#password`, "Tr0ub4dour&3xtra", chromedp.ByID),
		chromedp.Submit(`#password`, chromedp.ByID),
		chromedp.WaitVisible(`.app`, chromedp.ByQuery), // the three-pane budget shell (forecast)
		chromedp.AttributeValue(`.app`, "class", &shellClass, nil),
		chromedp.Text(`.center h1`, &h1Text, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("chromedp smoke: %v", err)
	}
	if !strings.Contains(shellClass, "app") {
		t.Errorf("shell not rendered (class=%q)", shellClass)
	}
	if !strings.Contains(h1Text, "Prévisionnel") {
		t.Errorf("forecast landing not rendered (h1=%q)", h1Text)
	}
}

// TestSmokeForecastRowExpand proves the forecast row drill-down toggles client-
// side under the app's CSP (no inline handlers): clicking a leaf row reveals its
// read-only transaction drill-down via the delegated data-action in app.js.
func TestSmokeForecastRowExpand(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL
	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Loyers"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"fixed_recurring"}, "default_amount": {"1050,00"}, "frequency": {"monthly"}, "expected_day": {"5"},
	})
	tok := csrfToken(t, client, base, "/config/parameters")
	bodyOf(t, formReq(t, client, "POST", base+"/month-init?period=2026-06", url.Values{"_csrf": {tok}}))

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("no-sandbox", true))...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, 50*time.Second)
	defer cancelT()

	var drillHidden bool
	err := chromedp.Run(
		ctx,
		chromedp.Navigate(ts.URL+"/login"),
		chromedp.WaitVisible(`#email`, chromedp.ByID),
		chromedp.SendKeys(`#email`, "owner@example.org", chromedp.ByID),
		chromedp.SendKeys(`#password`, "Tr0ub4dour&3xtra", chromedp.ByID),
		chromedp.Submit(`#password`, chromedp.ByID),
		chromedp.WaitVisible(`.app`, chromedp.ByQuery),
		chromedp.Navigate(ts.URL+"/?period=2026-06&scope="+fID),
		chromedp.WaitVisible(`tr.tog`, chromedp.ByQuery),
		chromedp.Click(`tr.tog`, chromedp.ByQuery),
		chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(`document.querySelector('tr.drill').classList.contains('hidden')`, &drillHidden),
	)
	if err != nil {
		t.Fatalf("chromedp forecast expand smoke: %v", err)
	}
	if drillHidden {
		t.Error("clicking a leaf row should reveal its drill-down (still hidden)")
	}
}

// TestSmokeForecastInlineEdit proves the forecast inline `Prévu` edit recomputes
// server-side in a real browser (CSP-clean): changing a leaf amount fires an htmx
// PATCH that swaps the edited row and the OOB savings panel. Dropping income to 0
// flips the residual band to the negative state.
func TestSmokeForecastInlineEdit(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL
	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Salaire"}, "flow_type": {"income"}, "account_id": {fID},
		"mode": {"fixed_recurring"}, "default_amount": {"2600,00"}, "frequency": {"monthly"}, "expected_day": {"1"},
	})
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Courses"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"variable"}, "default_amount": {"600,00"},
	})
	tok := csrfToken(t, client, base, "/config/parameters")
	bodyOf(t, formReq(t, client, "POST", base+"/month-init?period=2026-06", url.Values{"_csrf": {tok}}))

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("no-sandbox", true))...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, 50*time.Second)
	defer cancelT()

	var panelText string
	err := chromedp.Run(
		ctx,
		chromedp.Navigate(ts.URL+"/login"),
		chromedp.WaitVisible(`#email`, chromedp.ByID),
		chromedp.SendKeys(`#email`, "owner@example.org", chromedp.ByID),
		chromedp.SendKeys(`#password`, "Tr0ub4dour&3xtra", chromedp.ByID),
		chromedp.Submit(`#password`, chromedp.ByID),
		chromedp.WaitVisible(`.app`, chromedp.ByQuery),
		chromedp.Navigate(ts.URL+"/?period=2026-06&scope="+fID),
		chromedp.WaitVisible(`#fc-panel`, chromedp.ByID),
		// Raise the Courses expense (first inline input — rows sort by name, so
		// "Courses" precedes "Salaire") past income → htmx PATCH recompute negative.
		chromedp.Evaluate(`(function(){var i=document.querySelector('.amt-inp');i.value='9000,00';i.dispatchEvent(new Event('change',{bubbles:true}));})()`, nil),
		chromedp.Sleep(400*time.Millisecond),
		chromedp.Text(`#fc-panel`, &panelText, chromedp.ByID),
	)
	if err != nil {
		t.Fatalf("chromedp forecast inline-edit smoke: %v", err)
	}
	if !strings.Contains(panelText, "Solde insuffisant") {
		t.Errorf("residual did not recompute to negative after dropping income: %q", panelText)
	}
}

// TestSmokeParametersAccountModal proves the Paramètres screen works in a real
// browser under the app's CSP (no inline handlers): the htmx-loaded account
// modal opens, the native form submits, and the new account appears in the
// out-of-band-swapped Comptes card.
func TestSmokeParametersAccountModal(t *testing.T) {
	ts, _ := newTestServer(t)
	seedOwner(t, ts)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("no-sandbox", true))...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, 50*time.Second)
	defer cancelT()

	var cardText string
	err := chromedp.Run(
		ctx,
		chromedp.Navigate(ts.URL+"/login"),
		chromedp.WaitVisible(`#email`, chromedp.ByID),
		chromedp.SendKeys(`#email`, "owner@example.org", chromedp.ByID),
		chromedp.SendKeys(`#password`, "Tr0ub4dour&3xtra", chromedp.ByID),
		chromedp.Submit(`#password`, chromedp.ByID),
		chromedp.WaitVisible(`.app`, chromedp.ByQuery),
		chromedp.Navigate(ts.URL+"/config/parameters"),
		chromedp.WaitVisible(`#comptes-card`, chromedp.ByID),
		// htmx loads the modal (no inline onclick — CSP-clean).
		chromedp.Click(`button[hx-get="/config/accounts/new"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#account-modal`, chromedp.ByID),
		chromedp.SendKeys(`input[name="name"]`, "Fortuneo", chromedp.ByQuery),
		// native submit → POST → OOB swap of the Comptes card.
		chromedp.Click(`#account-modal button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#comptes-card`, chromedp.ByID),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Text(`#comptes-card`, &cardText, chromedp.ByID),
	)
	if err != nil {
		t.Fatalf("chromedp parameters smoke: %v", err)
	}
	if !strings.Contains(cardText, "Fortuneo") {
		t.Errorf("new account not shown in the Comptes card: %q", cardText)
	}
}

// TestSmokeEnvelopeModal proves the Enveloppes screen works in a real browser:
// the modal opens, mode-driven field adaptation runs (CSP-clean delegated JS),
// and a submitted envelope appears in the OOB-swapped list.
func TestSmokeEnvelopeModal(t *testing.T) {
	ts, client := setupOwner(t)
	// Seed an account so the form has an account option.
	tok := csrfToken(t, client, ts.URL, "/config/parameters")
	bodyOf(t, formReq(t, client, "POST", ts.URL+"/config/accounts", map[string][]string{
		"_csrf": {tok}, "name": {"Fortuneo"}, "type": {"current"}, "month_end_policy": {"sweep"},
	}))

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("no-sandbox", true))...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, 50*time.Second)
	defer cancelT()

	var freqOff bool
	var listText string
	err := chromedp.Run(
		ctx,
		chromedp.Navigate(ts.URL+"/login"),
		chromedp.WaitVisible(`#email`, chromedp.ByID),
		chromedp.SendKeys(`#email`, "owner@example.org", chromedp.ByID),
		chromedp.SendKeys(`#password`, "Tr0ub4dour&3xtra", chromedp.ByID),
		chromedp.Submit(`#password`, chromedp.ByID),
		chromedp.WaitVisible(`.app`, chromedp.ByQuery),
		chromedp.Navigate(ts.URL+"/config/envelopes"),
		chromedp.WaitVisible(`#env-list`, chromedp.ByID),
		chromedp.Click(`button[hx-get="/config/envelopes/new"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#envelope-modal`, chromedp.ByID),
		// switch mode to fixed_recurring and fire change → frequency field un-dims.
		chromedp.SetValue(`#e-mode`, "fixed_recurring", chromedp.ByID),
		chromedp.Evaluate(`document.getElementById('e-mode').dispatchEvent(new Event('change',{bubbles:true}))`, nil),
		chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(`document.getElementById('w-freq').classList.contains('off')`, &freqOff),
		chromedp.SendKeys(`input[name="name"]`, "Loyers", chromedp.ByQuery),
		chromedp.Click(`#envelope-modal button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#env-list`, chromedp.ByID),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Text(`#env-list`, &listText, chromedp.ByID),
	)
	if err != nil {
		t.Fatalf("chromedp envelope smoke: %v", err)
	}
	if freqOff {
		t.Error("frequency field should be enabled for a fixed_recurring envelope (adaptation did not run)")
	}
	if !strings.Contains(listText, "Loyers") {
		t.Errorf("new envelope not shown in the list: %q", listText)
	}
}

// TestSmokeMonthInitRecompute proves the month-init draft recomputes the residual
// server-side in a real browser (CSP-clean): editing a leaf amount fires an htmx
// PATCH that swaps the engine-computed figures fragment. Dropping the income to 0
// flips the residual band to the negative-residual state.
func TestSmokeMonthInitRecompute(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL
	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Salaire"}, "flow_type": {"income"}, "account_id": {fID},
		"mode": {"fixed_recurring"}, "default_amount": {"2600,00"}, "frequency": {"monthly"}, "expected_day": {"27"},
	})
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Loyers"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"fixed_recurring"}, "default_amount": {"1050,00"}, "frequency": {"monthly"}, "expected_day": {"5"},
	})

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("no-sandbox", true))...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, 50*time.Second)
	defer cancelT()

	var figuresText string
	err := chromedp.Run(
		ctx,
		chromedp.Navigate(ts.URL+"/login"),
		chromedp.WaitVisible(`#email`, chromedp.ByID),
		chromedp.SendKeys(`#email`, "owner@example.org", chromedp.ByID),
		chromedp.SendKeys(`#password`, "Tr0ub4dour&3xtra", chromedp.ByID),
		chromedp.Submit(`#password`, chromedp.ByID),
		chromedp.WaitVisible(`.app`, chromedp.ByQuery),
		chromedp.Navigate(ts.URL+"/month-init?period=2026-06"),
		chromedp.WaitVisible(`#mi-figures`, chromedp.ByID),
		// Drop the income (first leaf) to 0 and fire change → htmx PATCH recompute.
		chromedp.Evaluate(`(function(){var i=document.querySelector('.amt-inp');i.value='0,00';i.dispatchEvent(new Event('change',{bubbles:true}));})()`, nil),
		chromedp.Sleep(400*time.Millisecond),
		chromedp.Text(`#mi-figures`, &figuresText, chromedp.ByID),
	)
	if err != nil {
		t.Fatalf("chromedp month-init smoke: %v", err)
	}
	if !strings.Contains(figuresText, "négatif") {
		t.Errorf("residual did not recompute to negative after dropping income: %q", figuresText)
	}
}
