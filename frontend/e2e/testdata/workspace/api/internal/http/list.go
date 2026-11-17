package http

import (
	"encoding/json"
	"net/http"
)

// ListItems returns every item.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respond(w, items)
}

// respond writes a value as the JSON body of a response.
func respond(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")

	_ = json.NewEncoder(w).Encode(value)
}
