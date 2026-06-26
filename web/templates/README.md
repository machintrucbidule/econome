# web/templates/

`html/template` files, embedded via `embed.FS` (`technical/02` §3, `G5`):

- **Layouts** — the three-pane shell (`02-navigation-shell.md`) and the signed-out layout
  (login/setup/invitation).
- **Screens** — one template per screen (forecast, journal, networth, register, envelopes,
  month-init, parameters, login).
- **Fragments** — the reusable units htmx swaps (`envelope-row`, `journal-row`, `figures`,
  `savings-encart`, `total`, `timeline`, `month-summary`, `snapshot-row`, …); OOB partials carry
  `hx-swap-oob` and a `-oob` suffix. Fragment names mirror the `frag:…` names in `technical/04` §3.

Templates reuse `web/assets/econome.css` + `econome.js` verbatim. The shell + signed-out layout land
in **increment 1**; screen templates from increment 4 onward. At increment 0 this folder only carries
the `embed.FS` home and this note.
