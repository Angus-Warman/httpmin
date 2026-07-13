package main

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates
var templatesFS embed.FS

type UserRenderer struct {
	index *template.Template
	one   *template.Template
	many  *template.Template
}

func NewUserRenderer() *UserRenderer {
	tmpl := template.Must(template.ParseFS(templatesFS, "templates/*.tmpl"))

	return &UserRenderer{
		index: tmpl.Lookup("index.tmpl"),
		one:   tmpl.Lookup("user.tmpl"),
		many:  tmpl.Lookup("users.tmpl"),
	}
}

func (r *UserRenderer) RenderIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	r.index.Execute(w, nil)
}

func (r *UserRenderer) RenderOne(w http.ResponseWriter, user User) {
	w.Header().Set("Content-Type", "text/html")
	r.one.Execute(w, user)
}

func (r *UserRenderer) RenderMany(w http.ResponseWriter, users []User) {
	w.Header().Set("Content-Type", "text/html")
	r.many.Execute(w, users)
}
