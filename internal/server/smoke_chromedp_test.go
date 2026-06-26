//go:build chromedp

// The chromedp browser smoke runs only under the `chromedp` build tag (CI's
// e2e-chrome job, O-7) because it needs a headless Chrome. The httptest-based
// e2e backbone in e2e_test.go runs everywhere and is the primary coverage; this
// adds a real-browser check that login renders the three-pane shell.
package server_test

import (
	"context"
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

	var shellClass, demoText string
	err := chromedp.Run(
		ctx,
		chromedp.Navigate(ts.URL+"/login"),
		chromedp.WaitVisible(`#email`, chromedp.ByID),
		chromedp.SendKeys(`#email`, "owner@example.org", chromedp.ByID),
		chromedp.SendKeys(`#password`, "Tr0ub4dour&3xtra", chromedp.ByID),
		chromedp.Submit(`#password`, chromedp.ByID),
		chromedp.WaitVisible(`.app`, chromedp.ByQuery), // the three-pane shell
		chromedp.AttributeValue(`.app`, "class", &shellClass, nil),
		chromedp.Text(`#demo-balance`, &demoText, chromedp.ByID),
	)
	if err != nil {
		t.Fatalf("chromedp smoke: %v", err)
	}
	if !strings.Contains(shellClass, "app") {
		t.Errorf("shell not rendered (class=%q)", shellClass)
	}
	if strings.TrimSpace(demoText) == "" {
		t.Error("demo balance not rendered in the insights panel")
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
