package services

import (
	"context"
	"errors"

	"econome/internal/domain"
)

// SettingsInput is the changed settings fields (functional/04 §3.7, functional/10
// §3–§5). Money/rate fields arrive already parsed to minor units / basis points
// from the HTTP boundary (i18n.ParseMoney/ParsePercent) — the service never sees
// a locale string or a float. Every field is optional (pointer): nil means
// "leave unchanged", so the per-card htmx PATCHes only touch what they submit
// (technical/04 §4).
type SettingsInput struct {
	DefaultAccountID    *int64 // *0 clears the default
	PEAInitialDeposit   *int64 // minor units, >= 0
	PEASocialChargeRate *int   // basis points, [0, 10000)
	NearCapThreshold    *int   // basis points, [0, 10000)
	SecuredSavingsBasis *string
	CommentAutoprefill  *bool
	Theme               *string
	Language            *string
	Currency            *string
}

// UpdateSettings validates and applies the changed settings fields, returning the
// updated row. No partial write: any validation failure aborts before the update.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, in SettingsInput) (*domain.Settings, error) {
	cur, err := s.settings.Get(ctx, s.tx.DB(), userID)
	if err != nil {
		return nil, err
	}

	v := &domain.ValidationError{}

	if in.DefaultAccountID != nil {
		if *in.DefaultAccountID == 0 {
			cur.DefaultAccountID = nil
		} else if _, gerr := s.accounts.Get(ctx, s.tx.DB(), userID, *in.DefaultAccountID); gerr != nil {
			if errors.Is(gerr, domain.ErrNotFound) {
				v.Add("default_account_id", domain.MsgDefaultAccountWrong)
			} else {
				return nil, gerr
			}
		} else {
			cur.DefaultAccountID = in.DefaultAccountID
		}
	}
	if in.PEAInitialDeposit != nil {
		if *in.PEAInitialDeposit < 0 {
			v.Add("pea_initial_deposit", domain.MsgAmountNegative)
		} else {
			cur.PEAInitialDeposit = *in.PEAInitialDeposit
		}
	}
	if in.PEASocialChargeRate != nil {
		if *in.PEASocialChargeRate < 0 || *in.PEASocialChargeRate >= 10000 {
			v.Add("pea_social_charge_rate", domain.MsgRateInvalid)
		} else {
			cur.PEASocialChargeRate = *in.PEASocialChargeRate
		}
	}
	if in.NearCapThreshold != nil {
		if *in.NearCapThreshold < 0 || *in.NearCapThreshold >= 10000 {
			v.Add("near_cap_threshold", domain.MsgRateInvalid)
		} else {
			cur.NearCapThreshold = *in.NearCapThreshold
		}
	}
	if in.SecuredSavingsBasis != nil {
		b := domain.SecuredSavingsBasis(*in.SecuredSavingsBasis)
		if !b.Valid() {
			v.Add("secured_savings_basis", domain.MsgBasisInvalid)
		} else {
			cur.SecuredSavingsBasis = b
		}
	}
	if in.CommentAutoprefill != nil {
		cur.CommentAutoprefill = *in.CommentAutoprefill
	}
	if in.Theme != nil {
		th := domain.Theme(*in.Theme)
		if !th.Valid() {
			v.Add("theme", domain.MsgThemeInvalid)
		} else {
			cur.Theme = th
		}
	}
	if in.Language != nil {
		l := domain.Language(*in.Language)
		if !l.Valid() {
			v.Add("language", domain.MsgLanguageInvalid)
		} else {
			cur.Language = l
		}
	}
	if in.Currency != nil {
		c := trim(*in.Currency)
		if c == "" {
			v.Add("currency", domain.MsgCurrencyInvalid)
		} else {
			// Single active currency: changing it changes display only; stored
			// minor-unit amounts are never converted (foundation §12).
			cur.Currency = c
		}
	}

	if err := v.OrNil(); err != nil {
		return nil, err
	}

	cur.UpdatedAt = s.now().UTC()
	if err := s.settings.Update(ctx, s.tx.DB(), cur); err != nil {
		return nil, err
	}
	return cur, nil
}
