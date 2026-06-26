// Package view builds the pre-formatted view-models templates render, and
// registers the central set of pure template functions. It turns the engine's
// minor-unit integers and English codes into localised display values via
// internal/i18n. Templates contain no business logic and do no formatting of
// their own beyond calling view/i18n helpers (G5).
//
// View-models land alongside the screens they serve, from increment 1 onward.
package view
