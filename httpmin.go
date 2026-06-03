// The bare minimum required to launch a sensible Go server.
//
// Three goals:
//   - Minimal boilerplate
//   - Sensible defaults
//   - Easy configuration
//
// Features:
//   - Environment variable handling
//   - Efficient embedded asset serving
//   - Graceful shutdown
//   - Request logging
//   - Recovers from panics
//
// Optional:
//   - HTTPS (including self-signed)
//   - CORS
//   - JWT authentication
//   - SSE streaming
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
