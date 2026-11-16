package http

import (
	"encoding/json"
	"net/http"
)

// Handler serves the items API.
type Handler struct {
	store Store
	queue Queue
}

// GetItem returns one item.
func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(item)
}

// UpdateItem writes an item, then asks for its cache entries to be evicted.
func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	var item Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.Put(r.Context(), &item); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The write is durable before the eviction is requested, so a crash here
	// leaves a stale cache entry, never a lost write.
	if err := h.queue.Enqueue(r.Context(), Invalidate{ID: item.ID}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
