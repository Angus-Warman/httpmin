package main

import (
	"embed"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/middleware"
)

//go:embed all:public
var publicFiles embed.FS

func main() {
	httpmin.Setup().ServeEmbedded(publicFiles).Use(middleware.HotReload()).Run()
}
