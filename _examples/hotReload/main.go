package main

import (
	"embed"

	"github.com/Angus-Warman/httpmin"
)

//go:embed all:public
var publicFiles embed.FS

func main() {
	// httpmin.Setup().ServeEmbedded(publicFiles).Use(middleware.HotReload()).Run()
	httpmin.Setup().ServeEmbedded(publicFiles).Run()
}
