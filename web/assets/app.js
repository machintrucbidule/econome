/* EconoMe — application interactivity for the server-rendered screens.
   Separate from econome.js (the byte-for-byte validated design system, never
   hand-edited). Strictly CSP-clean: no inline handlers or inline scripts run
   under the app's Content-Security-Policy (script-src 'self'), so every
   behaviour is bound here by delegation off data-action attributes, native form
   controls carry every value, and mutations go through htmx attributes. */
(function () {
  "use strict";
  var $ = function (s, r) { return (r || document).querySelector(s); };
  var $$ = function (s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); };

  /* let htmx swap validation (422) and conflict (409) bodies into their target
     so inline field errors / "unlock to edit" hints render in place */
  function allowErrorSwap(e) {
    var s = e.detail.xhr.status;
    if (s === 422 || s === 409) { e.detail.shouldSwap = true; e.detail.isError = false; }
  }

  /* current fill-priority order as account-id strings, in DOM order */
  function cascadeIDs() {
    return $$("#cascade li.casc-item").map(function (li) { return li.getAttribute("data-id"); });
  }
  function cascadePost(ids) {
    var ul = $("#cascade");
    if (!ul || !window.htmx) return;
    htmx.ajax("POST", "/config/accounts/reorder", {
      values: { order: ids.join(","), _csrf: ul.getAttribute("data-csrf") },
      target: "#cascade", swap: "outerHTML",
    });
  }

  /* SortableJS drag-reorder of the savings cascade */
  function initCascade() {
    var ul = $("#cascade");
    if (!ul || !window.Sortable || ul.dataset.sortable) return;
    ul.dataset.sortable = "1";
    window.Sortable.create(ul, {
      handle: ".grip", draggable: "li.casc-item", animation: 120,
      onEnd: function () { cascadePost(cascadeIDs()); },
    });
  }

  function off(id, isOff) { var el = document.getElementById(id); if (el) el.classList.toggle("off", isOff); }

  /* envelope modal field adaptation: frequency/day only for fixed_recurring,
     month only for non-monthly fixed, amount disabled for residual, the new-parent
     name input shown only when "+ Nouvelle catégorie…" is picked */
  function adaptEnvelope() {
    var modeEl = document.getElementById("e-mode");
    if (!modeEl) return;
    var mode = modeEl.value;
    var freqEl = document.getElementById("e-freq");
    var freq = freqEl ? freqEl.value : "monthly";
    var isFixed = mode === "fixed_recurring";
    off("w-freq", !isFixed);
    off("w-day", !isFixed);
    off("w-month", !(isFixed && freq !== "monthly"));
    off("w-amount", mode === "residual");
    var flowEl = document.getElementById("e-flow");
    off("w-dest", !(flowEl && flowEl.value === "transfer"));
    var parent = document.getElementById("e-parent");
    var nw = document.getElementById("w-newparent");
    if (parent && nw) nw.classList.toggle("hidden", parent.value !== "__new__");
  }

  /* account modal field adaptation: month-end policy only for current accounts,
     ceiling only for savings accounts */
  function adaptAccount() {
    var t = $("#a-type"); if (!t) return;
    var isCurrent = t.value === "current";
    var wp = $("#w-policy"); if (wp) wp.classList.toggle("off", !isCurrent);
    var wc = $("#w-ceiling"); if (wc) wc.classList.toggle("off", isCurrent);
    var pol = $("#a-policy");
    if (pol) {
      if (isCurrent) {
        if (pol.value === "none") pol.value = "sweep";
      } else {
        pol.value = "none";
      }
    }
  }

  /* ---- Journal (increment 6c) — quick-entry + inline editing wired to the
     CSP-clean econome.js widgets + htmx. selSet is ported here (not in the
     never-edited econome.js). ---- */
  window.selSet = function (btn, value, label, ph) {
    if (!btn) return;
    btn.dataset.value = value == null ? "" : value;
    var lab = btn.querySelector(".lab"); if (lab) lab.textContent = label;
    btn.classList.toggle("ph", ph === true);
  };
  function jData(id) { var el = document.getElementById(id); if (!el) return []; try { return JSON.parse(el.textContent) || []; } catch (e) { return []; } }
  function jCats() { return jData("j-cats"); }
  function jAccts() { return jData("j-accts"); }
  function jStatuses() { return jData("j-status"); }
  function setHidden(id, v) { var h = document.getElementById(id); if (h) h.value = v == null ? "" : v; }
  function fireChange(el) { if (el) el.dispatchEvent(new Event("change", { bubbles: true })); }
  function acctLabel(id) { var a = jAccts().filter(function (x) { return x.value === String(id); })[0]; return a ? a.label : String(id); }

  /* quick-entry / filter custom-select trigger */
  function jPick(btn) {
    var kind = btn.getAttribute("data-kind"), targetId = btn.getAttribute("data-target");
    if (kind === "date") {
      window.emCal(btn, btn.dataset.value || "", function (v) { window.selSet(btn, v, v || "(date)"); setHidden(targetId, v); });
      return;
    }
    var opts, current = btn.dataset.value || "";
    if (kind === "cat") opts = jCats();
    else if (kind === "fcat") opts = [{ value: "", label: (btn.querySelector(".lab") || {}).textContent || "—" }].concat(jCats().filter(function (o) { return o.value !== "transfer"; }));
    else if (kind === "acct") opts = jAccts();
    else if (kind === "status") opts = jStatuses();
    else return;
    window.emMenu(btn, opts, current, function (v, l, o) {
      window.selSet(btn, v, l, kind === "cat" && v === "");
      setHidden(targetId, v);
      if (kind === "cat") jCatChosen(o);
      if (kind === "fcat") fireChange(document.getElementById(targetId));
    });
  }
  function jCatChosen(o) {
    if (!o) return;
    var flow = o.value === "transfer" ? "transfer" : (o.flow || "expense");
    setHidden("q-flow-v", flow);
    if (flow === "transfer") setHidden("q-cat-v", "");
    var dw = document.getElementById("q-dest-wrap"); if (dw) dw.classList.toggle("hidden", flow !== "transfer");
    if (flow !== "transfer" && o.acct) {
      var ab = document.getElementById("q-acct");
      if (ab) { window.selSet(ab, o.acct, acctLabel(o.acct)); setHidden("q-acct-v", o.acct); }
    }
  }

  /* inline cell editing → htmx PATCH /transactions/:id */
  function jPatch(id, field, value) {
    var jb = document.getElementById("jbody"); if (!jb || !window.htmx) return;
    var vals = {}; vals[field] = value;
    window.htmx.ajax("PATCH", "/transactions/" + id + "?period=" + encodeURIComponent(jb.dataset.period) + "&scope=" + encodeURIComponent(jb.dataset.scope),
      { values: vals, target: "#jrow-" + id, swap: "outerHTML" });
  }
  function pad2(n) { return (n < 10 ? "0" : "") + n; }
  function jEdit(cell) {
    var kind = cell.getAttribute("data-edit"), id = cell.getAttribute("data-id");
    if (kind === "op_date") {
      var cur = (cell.textContent || "").replace("~", "").trim(); if (!/^\d{2}\/\d{2}$/.test(cur)) cur = "";
      window.emCal(cell, cur, function (v) { jPatch(id, "op_date", v); });
    } else if (kind === "budget_period") {
      var per = cell.getAttribute("data-period") || "", m = parseInt(per.slice(5), 10) - 1, y = parseInt(per.slice(0, 4), 10);
      window.emMonth(cell, m, y, function (i, label, year) { jPatch(id, "budget_period", year + "-" + pad2(i + 1)); });
    } else if (kind === "category_id") {
      window.emMenu(cell, jCats().filter(function (o) { return o.value !== "transfer"; }), cell.getAttribute("data-cat"), function (v) { jPatch(id, "category_id", v); });
    } else if (kind === "account_id") {
      window.emMenu(cell, jAccts(), cell.getAttribute("data-acct"), function (v) { jPatch(id, "account_id", v); });
    } else if (kind === "status") {
      window.emMenu(cell, jStatuses(), cell.getAttribute("data-status"), function (v) { jPatch(id, "status", v); });
    } else if (kind === "label") {
      jInlineText(cell, "label", id, cell.querySelector(".ltext"));
    } else if (kind === "amount") {
      jInlineText(cell, "amount", id, cell.querySelector(".vtext"));
    }
  }
  function jInlineText(cell, field, id, span) {
    if (!span || cell.querySelector("input")) return;
    var raw = field === "amount" ? span.textContent.replace(/[^\d.,]/g, "") : span.textContent;
    var inp = document.createElement("input"); inp.className = field === "amount" ? "amt-inp" : "lbl-inp"; inp.value = raw;
    cell.replaceChild(inp, span); inp.focus(); inp.select();
    var closed = false;
    var done = function (commit) {
      if (closed) return; closed = true;
      if (commit) jPatch(id, field, inp.value);
      else if (inp.parentNode) cell.replaceChild(span, inp);
    };
    inp.addEventListener("blur", function () { done(true); });
    inp.addEventListener("keydown", function (e) {
      if (e.key === "Enter") { e.preventDefault(); inp.blur(); }
      else if (e.key === "Escape") { done(false); }
    });
  }
  function jSort(th) {
    var col = th.getAttribute("data-col"), fs = $("#f-sort"), fd = $("#f-dir");
    if (!fs || !fd) return;
    if (fs.value === col) fd.value = fd.value === "asc" ? "desc" : "asc";
    else { fs.value = col; fd.value = (col === "date" || col === "amount" || col === "period") ? "desc" : "asc"; }
    $$(".jtable th.sortable").forEach(function (t) { t.classList.remove("asc", "desc"); if (t.getAttribute("data-col") === col) t.classList.add(fd.value); });
    fireChange(fs);
  }
  function jResetQform() {
    var q = $("#qform"); if (!q) return;
    var lf = q.querySelector('[name="label"]'); if (lf) lf.value = "";
    var af = q.querySelector('[name="amount"]'); if (af) af.value = "";
    setHidden("q-cat-v", ""); setHidden("q-flow-v", ""); setHidden("q-dest-v", ""); setHidden("q-date-v", "");
    var cb = $("#q-cat"); if (cb) window.selSet(cb, "", "—", true);
    var db = $("#q-date"); if (db) window.selSet(db, "", "(date)", true);
    var dw = $("#q-dest-wrap"); if (dw) dw.classList.add("hidden");
    if (lf) lf.focus();
  }

  /* delegated click handling (CSP-clean) */
  document.addEventListener("click", function (e) {
    if (e.target.closest(".em-menu,.em-cal,.em-mp,.em-auto,input")) return; // let widgets/inline inputs handle their own clicks
    var el = e.target.closest("[data-action]");
    if (!el) return;
    var action = el.getAttribute("data-action");
    if (action === "j-pick") { jPick(el); return; }
    if (action === "j-edit") { jEdit(el); return; }
    if (action === "j-sort") { jSort(el); return; }
    if (action === "close-drawers") { if (window.closeDrawers) window.closeDrawers(); return; }
    if (action === "theme-toggle") {
      var html = document.documentElement;
      html.setAttribute("data-theme", html.getAttribute("data-theme") === "dark" ? "light" : "dark");
    } else if (action === "nav-left") {
      if (window.toggleLeft) window.toggleLeft();
    } else if (action === "nav-right") {
      if (window.toggleRight) window.toggleRight();
    } else if (action === "toggle-picker") {
      var mp = $("#mp"); if (mp) mp.classList.toggle("open");
    } else if (action === "goto") {
      var href = el.getAttribute("data-href"); if (href) window.location.assign(href);
    } else if (action === "toggle-row") {
      /* forecast row expand: parent → its children, leaf → its drill-down */
      var key = el.getAttribute("data-k");
      var open = el.classList.toggle("open");
      var chev = el.querySelector(".chev");
      if (chev) chev.classList.toggle("open", open);
      $$('tr[data-c="' + key + '"], tr[data-d="' + key + '"]').forEach(function (tr) {
        tr.classList.toggle("hidden", !open);
      });
    } else if (action === "modal-close") {
      var host = $("#modal-host"); if (host) host.innerHTML = "";
    } else if (action === "cascade-remove") {
      var id = el.getAttribute("data-id");
      cascadePost(cascadeIDs().filter(function (x) { return x !== id; }));
    } else if (action === "toggle-group") {
      var k = el.getAttribute("data-k");
      el.classList.toggle("open");
      var open = el.classList.contains("open");
      $$('tr[data-c="' + k + '"]').forEach(function (tr) { tr.classList.toggle("hidden", !open); });
    }
  });

  /* delegated change handling */
  document.addEventListener("change", function (e) {
    var el = e.target;
    if (el.id === "a-type") { adaptAccount(); return; }
    if (el.id === "e-mode" || el.id === "e-freq" || el.id === "e-parent" || el.id === "e-flow") { adaptEnvelope(); return; }
    var action = el.getAttribute && el.getAttribute("data-action");
    if (action === "toggle-arch") {
      var on = el.checked;
      $$("tr.arch").forEach(function (tr) { tr.classList.toggle("hidden", !on); });
    } else if (action === "theme-pref") {
      var h = document.documentElement;
      h.setAttribute("data-theme", el.checked ? "dark" : "light");
    } else if (action === "cascade-add") {
      var v = el.value; if (!v) return;
      var ids = cascadeIDs(); ids.push(v); cascadePost(ids);
    }
  });

  /* keyboard: open an inline editor / sort header with Enter or Space */
  document.addEventListener("keydown", function (e) {
    if (e.key !== "Enter" && e.key !== " ") return;
    if (e.target.closest("input,.em-menu,.em-cal,.em-mp")) return;
    var el = e.target.closest('[data-action="j-edit"],[data-action="j-sort"]');
    if (!el) return;
    e.preventDefault();
    if (el.getAttribute("data-action") === "j-sort") jSort(el); else jEdit(el);
  });

  function init() {
    if (document.body) {
      document.body.addEventListener("htmx:beforeSwap", allowErrorSwap);
      document.body.addEventListener("htmx:afterSwap", function (e) {
        if (window.htmx && e.detail && e.detail.target) window.htmx.process(e.detail.target);
        initCascade();
        adaptAccount();
        adaptEnvelope();
      });
      document.body.addEventListener("htmx:afterRequest", function (e) {
        if (e.detail && e.detail.elt && e.detail.elt.id === "qform" && e.detail.successful) jResetQform();
      });
    }
    initCascade();
    adaptAccount();
    adaptEnvelope();
  }
  if (document.readyState !== "loading") init();
  else document.addEventListener("DOMContentLoaded", init);
})();
