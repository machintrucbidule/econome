package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"econome/internal/domain"
	"econome/internal/server/middleware"
)

// UIExpand persists one hierarchy node's open/closed state (PUT /ui/expand, M4).
// It is per-user UI state, not a derived figure; returns 204 with no body (the
// client already toggled optimistically).
func (h *Handlers) UIExpand(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	_ = r.ParseForm()
	nodeType := domain.NodeType(strings.TrimSpace(r.FormValue("node_type")))
	nodeID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("node_id")), 10, 64)
	expanded := r.FormValue("expanded") == "1"
	if err != nil {
		http.Error(w, "bad node", http.StatusBadRequest)
		return
	}
	if err := h.svc.SetExpand(r.Context(), c.User.ID, nodeType, nodeID, expanded); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
