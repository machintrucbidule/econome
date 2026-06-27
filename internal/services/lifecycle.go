package services

import (
	"context"
	"time"

	"econome/internal/domain"
	"econome/internal/repo"
)

// Month-lifecycle controls (functional/04 §4, technical/04 §3.7, L1/L9). The
// locked-month guard (ensureEditable) lives in accounts.go and is already wired
// into every budget mutation; this file adds the explicit close/unlock controls
// and the additive regenerate. Closing/unlocking writes a period_event audit row
// (technical/05 §9). The pre-close to_save sweep (O-18) reuses the existing
// EndOfMonthTransfer + the right-panel residual encart — surfaced, not forced.

// O-18 (pre-close sweep): the realised residual `to_save` is already surfaced on
// the active month by the forecast right-panel residual encart (the "Virer en fin
// de mois" action), and the close control's confirmation explicitly reminds the
// user to sweep before locking. Closing never forces or blocks on the sweep.

// LockMonth closes a month: it must exist and be ACTIVE; the state moves to
// LOCKED and a `lock` audit event is appended, in one transaction (functional/04
// §4 L1). A not-created period is a 404; an already-locked period is a 409.
func (s *Service) LockMonth(ctx context.Context, userID int64, period string, actorID int64) error {
	return s.setPeriodState(ctx, userID, period, actorID, domain.PeriodLocked)
}

// UnlockMonth returns a LOCKED month to ACTIVE for correction, appending an
// `unlock` audit event (functional/04 §4 L1 — the only way to edit a locked
// month). A not-created period is a 404; an already-active period is a 409.
func (s *Service) UnlockMonth(ctx context.Context, userID int64, period string, actorID int64) error {
	return s.setPeriodState(ctx, userID, period, actorID, domain.PeriodActive)
}

func (s *Service) setPeriodState(ctx context.Context, userID int64, period string, actorID int64, target domain.PeriodState) error {
	if _, _, ok := parsePeriodYM(period); !ok {
		v := &domain.ValidationError{}
		v.Add("period", domain.MsgPeriodInvalid)
		return v
	}
	now := s.now().UTC()
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		p, err := s.periods.ByPeriod(ctx, q, userID, period)
		if err != nil {
			return err // ErrNotFound ⇒ 404
		}
		if p.State == target {
			return domain.ErrConflict // already in the requested state — nothing to do
		}
		var lockedAt *time.Time
		action := domain.ActionUnlock
		if target == domain.PeriodLocked {
			lockedAt = &now
			action = domain.ActionLock
		}
		if err := s.periods.UpdateState(ctx, q, userID, period, target, lockedAt); err != nil {
			return err
		}
		_, err = s.periodEvents.Append(ctx, q, &domain.PeriodEvent{
			UserID: userID, Period: period, Action: action, At: now, ActorUserID: actorID,
		})
		return err
	})
}

// RegenerateMissingRecurring adds the recurring/variable lines now due for a
// period that are absent — e.g. an envelope created after the month was
// initialised — without touching anything already entered (functional/04 §4 L9).
// It is purely additive and idempotent: the per-envelope allocation (and, for a
// transfer, the awaited transfer) is the presence marker, so re-running adds
// nothing. Refused on a locked month. Returns the number of envelopes added.
func (s *Service) RegenerateMissingRecurring(ctx context.Context, userID int64, period string) (int, error) {
	if _, _, ok := parsePeriodYM(period); !ok {
		v := &domain.ValidationError{}
		v.Add("period", domain.MsgPeriodInvalid)
		return 0, v
	}
	now := s.now().UTC()
	added := 0
	err := s.tx.WithTx(ctx, func(q repo.DBTX) error {
		if _, err := s.periods.ByPeriod(ctx, q, userID, period); err != nil {
			return err // a not-created period has nothing to regenerate (404)
		}
		if err := s.ensureEditable(ctx, q, userID, period); err != nil {
			return err
		}
		accounts, err := s.accounts.ListByUser(ctx, q, userID)
		if err != nil {
			return err
		}
		categories, err := s.categories.ListByUser(ctx, q, userID)
		if err != nil {
			return err
		}
		envelopes, err := s.envelopes.ListByUser(ctx, q, userID)
		if err != nil {
			return err
		}
		existingAllocs, err := s.allocations.ListByPeriod(ctx, q, userID, period)
		if err != nil {
			return err
		}
		hasAlloc := make(map[int64]bool, len(existingAllocs))
		for _, a := range existingAllocs {
			hasAlloc[a.EnvelopeID] = true
		}
		existingTxns, err := s.transactions.ListByPeriod(ctx, q, userID, period)
		if err != nil {
			return err
		}
		hasTransfer := make(map[[2]int64]bool, len(existingTxns))
		for _, t := range existingTxns {
			if t.FlowType == domain.FlowTransfer && t.DestAccountID != nil {
				hasTransfer[[2]int64{t.AccountID, *t.DestAccountID}] = true
			}
		}

		posts := buildDraftPosts(envelopes, categories, accounts, period, nil)
		for _, p := range posts {
			switch {
			case p.HasAllocation:
				if hasAlloc[p.EnvelopeID] {
					continue // already materialised — leave it untouched
				}
				if _, err := s.allocations.Create(ctx, q, &domain.Allocation{
					UserID: userID, EnvelopeID: p.EnvelopeID, Period: period,
					PlannedAmount: p.Amount, CreatedAt: now, UpdatedAt: now,
				}); err != nil {
					return err
				}
				if p.HasTxn {
					if err := s.createAwaitedPost(ctx, q, userID, period, p, now); err != nil {
						return err
					}
				}
				added++
			case p.HasTxn && p.DestAccountID != nil: // recurring transfer (no allocation)
				if hasTransfer[[2]int64{p.AccountID, *p.DestAccountID}] {
					continue
				}
				if err := s.createAwaitedPost(ctx, q, userID, period, p, now); err != nil {
					return err
				}
				added++
			}
		}
		return nil
	})
	return added, err
}

func (s *Service) createAwaitedPost(ctx context.Context, q repo.DBTX, userID int64, period string, p DraftPost, now time.Time) error {
	_, err := s.transactions.Create(ctx, q, &domain.Transaction{
		UserID: userID, AccountID: p.AccountID, DestAccountID: p.DestAccountID,
		CategoryID: postCategoryID(p), FlowType: p.Flow,
		Amount: signedAmount(p.Flow, p.Amount), BudgetPeriod: period,
		Status: domain.StatusAwaited, Source: domain.SourceManual,
		CreatedAt: now, UpdatedAt: now,
	})
	return err
}

// LifecycleEvents returns the period's audit trail (most recent first not
// required — the view orders), for surfacing lock/unlock history if needed.
func (s *Service) LifecycleEvents(ctx context.Context, userID int64, period string) ([]domain.PeriodEvent, error) {
	return s.periodEvents.ListByPeriod(ctx, s.tx.DB(), userID, period)
}
