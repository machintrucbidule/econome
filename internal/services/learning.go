package services

import (
	"context"
	"strings"

	"econome/internal/domain"
	"econome/internal/repo"
)

// Learned label mappings (M21) and per-user expand/collapse state (M4). Both are
// UI conveniences, not derived figures — they persist user behaviour, never a
// computed value.

// NodeKey identifies a hierarchy node for an expand preference.
type NodeKey struct {
	Type domain.NodeType
	ID   int64
}

// TopLabels returns the user's most-used labels (for the autocomplete embed).
func (s *Service) TopLabels(ctx context.Context, userID int64, limit int) ([]domain.LabelMapping, error) {
	return s.labels.Search(ctx, s.tx.DB(), userID, "", limit)
}

// recordLabel reinforces a learned label→category/account mapping on write (the
// repo Upsert increments usage_count). Blank labels are ignored.
func (s *Service) recordLabel(ctx context.Context, q repo.DBTX, userID int64, label string, categoryID *int64, accountID int64) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	acc := accountID
	return s.labels.Upsert(ctx, q, &domain.LabelMapping{
		UserID: userID, Label: label, LabelKey: labelKey(label),
		CategoryID: categoryID, AccountID: &acc,
	})
}

// labelKey normalises a label for case-insensitive prefix search.
func labelKey(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}

// ExpandPrefs returns the user's persisted node open/closed state (M4).
func (s *Service) ExpandPrefs(ctx context.Context, userID int64) (map[NodeKey]bool, error) {
	prefs, err := s.uiPrefs.ListByUser(ctx, s.tx.DB(), userID)
	if err != nil {
		return nil, err
	}
	out := make(map[NodeKey]bool, len(prefs))
	for _, p := range prefs {
		out[NodeKey{Type: p.NodeType, ID: p.NodeID}] = p.Expanded
	}
	return out, nil
}

// SetExpand upserts one node's open/closed state (PUT /ui/expand, M4).
func (s *Service) SetExpand(ctx context.Context, userID int64, nodeType domain.NodeType, nodeID int64, expanded bool) error {
	if !nodeType.Valid() || nodeID <= 0 {
		v := &domain.ValidationError{}
		v.Add("node", domain.MsgFieldInvalid)
		return v
	}
	return s.uiPrefs.Upsert(ctx, s.tx.DB(), &domain.UIPreference{
		UserID: userID, NodeType: nodeType, NodeID: nodeID, Expanded: expanded,
	})
}
