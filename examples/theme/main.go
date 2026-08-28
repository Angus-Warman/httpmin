package main

import (
	"bytes"
	"net/http"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/handler"
	"github.com/Angus-Warman/httpmin/theme"

	_ "embed"
)

//go:embed index.html
var indexBytes []byte

func main() {
	styles := []string{
		"basic",
		"minimal",
		"modern",
		"paper",
		"console",
	}

	c := httpmin.New()

	for _, name := range styles {
		demo := bytes.ReplaceAll(indexBytes, []byte("styles.css"), []byte(name+".css"))
		c.RouteHandler("/"+name, handler.FromBytes(demo))
	}

	c.
		RouteHandler("/basic.css", theme.Basic()).
		RouteHandler("/minimal.css", theme.Minimal()).
		RouteHandler("/modern.css", theme.Modern()).
		RouteHandler("/paper.css", theme.Paper()).
		RouteHandler("/console.css", theme.Console()).
		RouteHandler("/", http.RedirectHandler("/modern", http.StatusSeeOther)).
		Run()
}
