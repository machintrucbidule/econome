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

  /* delegated click handling (CSP-clean) */
  document.addEventListener("click", function (e) {
    var el = e.target.closest("[data-action]");
    if (!el) return;
    var action = el.getAttribute("data-action");
    if (action === "theme-toggle") {
      var html = document.documentElement;
      html.setAttribute("data-theme", html.getAttribute("data-theme") === "dark" ? "light" : "dark");
    } else if (action === "nav-left") {
      if (window.toggleLeft) window.toggleLeft();
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
    if (el.id === "e-mode" || el.id === "e-freq" || el.id === "e-parent") { adaptEnvelope(); return; }
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

  function init() {
    if (document.body) {
      document.body.addEventListener("htmx:beforeSwap", allowErrorSwap);
      document.body.addEventListener("htmx:afterSwap", function (e) {
        if (window.htmx && e.detail && e.detail.target) window.htmx.process(e.detail.target);
        initCascade();
        adaptAccount();
        adaptEnvelope();
      });
    }
    initCascade();
    adaptAccount();
    adaptEnvelope();
  }
  if (document.readyState !== "loading") init();
  else document.addEventListener("DOMContentLoaded", init);
})();
