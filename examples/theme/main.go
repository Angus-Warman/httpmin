package main

import (
	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/theme"
)

func main() {
	httpmin.New().
		OnPort("8081").
		Theme(theme.Modern).
		ServeFolder("public").
		Run()
}
