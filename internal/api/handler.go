package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"kvstore/internal/store"
)

type Handler struct {
	store *store.Store
}

func NewHandler(s *store.Store) *Handler {
	return &Handler{store: s}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/kv", h.handleKV)
}

func (h *Handler) handleKV(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.get(w, key)

	case http.MethodPut:
		h.put(w, r, key)

	case http.MethodDelete:
		h.delete(w, key)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) get(w http.ResponseWriter, key string) {
	value, err := h.store.Get([]byte(key))
	if err != nil {
		if errors.Is(err, store.ErrKeyNotFound) {
			http.NotFound(w, nil)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(map[string]string{
		"key":   key,
		"value": string(value),
	})
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request, key string) {
	var req struct {
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if err := h.store.Set([]byte(key), []byte(req.Value)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) delete(w http.ResponseWriter, key string) {
	if _, err := h.store.Del([]byte(key)); err != nil {
		if errors.Is(err, store.ErrKeyNotFound) {
			http.NotFound(w, nil)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}