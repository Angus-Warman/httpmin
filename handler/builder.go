package handler

import (
	"encoding/json"
	"net/http"
	"strings"
)

type Store[E any] interface {
	GetAll() ([]E, error)
	Get(string) (E, error)
	Add(E) (E, error)
	Update(E) (E, error)
	Delete(E) (bool, error)
}

type endpointBuilder[E any] struct {
	store Store[E]
}

func Builder[E any](mux *http.ServeMux, root string, store Store[E]) {
	b := endpointBuilder[E]{
		store: store,
	}

	mux.HandleFunc("GET "+root, b.getAll)
}

func (b *endpointBuilder[E]) getAll(w http.ResponseWriter, r *http.Request) {
	rows, err := b.store.GetAll()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if acceptsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rows)
		return
	}

}

func acceptsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}
