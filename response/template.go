package response

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

func Template(tmpl *template.Template, w http.ResponseWriter, templateName string, data any) {
	templateToUse := tmpl.Lookup(templateName)

	if templateToUse == nil {
		http.Error(w, fmt.Sprintf("template %q not found", templateName), http.StatusNotFound)
		return
	}

	w.Header().Add("Content-Type", "text/html")

	err := templateToUse.Execute(w, data)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Removes the top-level folder, as well as file extensions.
// Panics on errors.
//
//	//go:embed templates
//	var templateFiles embed.FS
//	var tmpls = response.PrepareTemplate(templateFiles)
//	response.Template(tmpls, w, "template name", data)
func PrepareTemplate(templateFiles embed.FS) *template.Template {
	root := template.New("")

	err := fs.WalkDir(templateFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := templateFiles.ReadFile(path)
		if err != nil {
			return err
		}

		name := templateName(path)

		_, err = root.New(name).Parse(string(data))
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		panic(err)
	}

	return root
}

// Drops the top-level folder and file extension
func templateName(path string) string {
	if idx := strings.Index(path, "/"); idx != -1 {
		path = path[idx+1:]
	}

	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext)
}
