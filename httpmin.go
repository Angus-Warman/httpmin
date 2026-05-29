// httpmin includes the bare minimum for a sensible HTTP service.
//
// It has two goals, minimal boilerplate, and configuration with minimal fuss.
//
//   - Environment variables
//   - Request logging
//   - Panic handling
//   - Optional HTTPS
//   - Optional middleware.
//
// # Hello World
//
//	package main
//
//	import (
//		"net/http"
//
//		"github.com/Angus-Warman/httpmin"
//	)
//
//	func helloWorld(w http.ResponseWriter, r *http.Request) {
//		w.Write([]byte("Hello World"))
//	}
//
//	func main() {
//		httpmin.Setup().Route("/", helloWorld).Run()
//	}
//
// # Serve static files
//
//	package main
//
//	import (
//		"net/http"
//		"embed"
//
//		"github.com/Angus-Warman/httpmin"
//	)
//
//	//go:embed all:public
//	var publicFiles embed.FS
//
//	func main() {
//		httpmin.Setup().ServeEmbedded(publicFiles).Run()
//	}
//
// See [Chassis] for more details.
//
// See github.com/Angus-Warman/httpmin/tree/main/_examples for in-depth demos.
package httpmin
