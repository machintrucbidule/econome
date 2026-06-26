package domain

// Configuration validation message keys (accounts, settings, categories,
// envelopes). Resolved to localised text by the view layer (technical/06 §4);
// they are catalog keys, never user-facing strings. Behaviour: functional/04
// §3.1–§3.3/§3.7, functional/08, functional/10.
const (
	// Accounts (functional/04 §3.1, functional/10 §2).
	MsgNameRequired         = "validation.name.required"
	MsgAccountNameDuplicate = "validation.account.name_duplicate"
	MsgAccountTypeInvalid   = "validation.account.type_invalid"
	MsgAccountPolicyInvalid = "validation.account.policy_invalid"
	MsgCeilingNegative      = "validation.account.ceiling_negative"
	MsgEffectivePeriod      = "validation.account.effective_period"

	// Savings cascade + Épargne & fiscalité (functional/10 §3).
	MsgCascadeNotSavings   = "validation.cascade.not_savings"
	MsgDefaultAccountWrong = "validation.settings.default_account"
	MsgAmountNegative      = "validation.amount.negative"
	MsgAmountInvalid       = "validation.amount.invalid"
	MsgRateInvalid         = "validation.rate.invalid"

	// Localisation & préférences (functional/10 §4–§5).
	MsgLanguageInvalid = "validation.settings.language"
	MsgCurrencyInvalid = "validation.settings.currency"
	MsgThemeInvalid    = "validation.settings.theme"
	MsgBasisInvalid    = "validation.settings.basis"

	// Categories & envelopes (functional/04 §3.2–§3.3, functional/08) — PR-b.
	MsgEnvelopeDuplicate    = "validation.envelope.duplicate"
	MsgFlowTypeInvalid      = "validation.envelope.flow_type"
	MsgModeInvalid          = "validation.envelope.mode"
	MsgFrequencyRequired    = "validation.envelope.frequency_required"
	MsgFrequencyInvalid     = "validation.envelope.frequency_invalid"
	MsgDueMonthsRequired    = "validation.envelope.due_months_required"
	MsgDueMonthsInvalid     = "validation.envelope.due_months_invalid"
	MsgExpectedDayInvalid   = "validation.envelope.expected_day"
	MsgResidualNoAmount     = "validation.envelope.residual_no_amount"
	MsgResidualNotDeletable = "validation.envelope.residual_locked"
	MsgParentCycle          = "validation.category.parent_cycle"
	MsgFlowTypeConflict     = "validation.category.flow_type_conflict"
	MsgCategoryHasChildren  = "validation.category.has_children"
	MsgAccountRequired      = "validation.envelope.account_required"

	// Month-initialisation assistant (functional/09).
	MsgPeriodInvalid = "validation.period.invalid"

	// Transfer-envelope destination (T11, functional/09 §3.4).
	MsgDestRequired    = "validation.envelope.dest_required"
	MsgDestNotAllowed  = "validation.envelope.dest_not_allowed"
	MsgDestSameAccount = "validation.envelope.dest_same_account"
	MsgDestInvalid     = "validation.envelope.dest_invalid"
	MsgDestNotCurrent  = "validation.envelope.dest_not_current"
)
