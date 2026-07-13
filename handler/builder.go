package handler

import (
	"net/http"
	"strings"

	"github.com/Angus-Warman/httpmin/parserequest"
	"github.com/Angus-Warman/httpmin/response"
)

type Store[E any] interface {
	GetAll() ([]E, error)
	Get(string) (E, error)
	Add(E) (E, error)
	Update(E) (E, error)
	Delete(string) (bool, error)
}

type Renderer[E any] interface {
	RenderOne(http.ResponseWriter, E)
	RenderMany(http.ResponseWriter, []E)
}

type endpointBuilder[E any] struct {
	store    Store[E]
	renderer Renderer[E]
}

func Builder[E any](mux *http.ServeMux, root string, store Store[E], renderer Renderer[E]) {
	b := endpointBuilder[E]{
		store:    store,
		renderer: renderer,
	}

	mux.HandleFunc("GET "+root, b.getAll)
	mux.HandleFunc("GET "+root+"/{id}", b.get)
	mux.HandleFunc("POST "+root, b.add)
	mux.HandleFunc("PUT "+root+"/{id}", b.update)
	mux.HandleFunc("DELETE "+root+"/{id}", b.delete)
}

func (b *endpointBuilder[E]) getAll(w http.ResponseWriter, r *http.Request) {
	rows, err := b.store.GetAll()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if acceptsJSON(r) {
		response.JSON(w, rows)
		return
	}

	b.renderer.RenderMany(w, rows)
}

func (b *endpointBuilder[E]) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := b.store.Get(id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if acceptsJSON(r) {
		response.JSON(w, item)
		return
	}

	b.renderer.RenderOne(w, item)
}

func (b *endpointBuilder[E]) add(w http.ResponseWriter, r *http.Request) {
	item, err := parserequest.As[E](r)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := b.store.Add(item)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if acceptsJSON(r) {
		response.JSON(w, created)
		return
	}

	b.renderer.RenderOne(w, created)
}

func (b *endpointBuilder[E]) update(w http.ResponseWriter, r *http.Request) {
	item, err := parserequest.As[E](r)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updated, err := b.store.Update(item)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if acceptsJSON(r) {
		response.JSON(w, updated)
		return
	}

	b.renderer.RenderOne(w, updated)
}

func (b *endpointBuilder[E]) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := b.store.Delete(id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func acceptsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}
