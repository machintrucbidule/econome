// Package i18n loads the embedded FR/EN TOML catalogs once and provides message
// lookup and locale-aware formatting (money, dates, numbers). Together with
// internal/view it owns ALL server-side formatting (technical/06): the engine
// returns minor-unit integers and English codes; only here do they become
// localised strings such as "−635,00 €".
//
// Catalogs + formatting land in increment 1 (the money-formatting boundary) and
// fill out across later increments.
package i18n
