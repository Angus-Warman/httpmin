package handler

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
)

// Panics if templateName not found in templatesFS, or template cannot be parsed.
//
// Does nothing if dataFn returns nil, expects dataFn to have sent response.
//
//	//go:embed templates
//	var templatesFS embed.FS
func Template(templatesFS embed.FS, templateName string, dataFn func(w http.ResponseWriter, r *http.Request) (any, error)) http.Handler {
	innerFolder := substituteTopLevelDir(templatesFS)

	tmpl, err := template.ParseFS(innerFolder, templateName)

	if err != nil {
		panic(err)
	}

	if tmpl == nil {
		panic(fmt.Sprintf("no template found for %v", templateName))
	}

	fn := func(w http.ResponseWriter, r *http.Request) {
		data, err := dataFn(w, r)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if data == nil {
			return
		}

		w.Header().Add("Content-Type", "text/html")

		err = tmpl.Execute(w, data)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	return http.HandlerFunc(fn)
}
