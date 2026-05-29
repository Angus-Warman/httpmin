## httpmin

The bare minimum required to launch a sensible Go server:

Environment variables, request logging, recovers from panics, HTTPS, configuration with minimal fuss.

```bash
go get https://github.com/Angus-Warman/httpmin
```

#### Hello World

```go
package main

import (
	"net/http"

	"github.com/Angus-Warman/httpmin"
)

func helloWorld(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello World"))
}

func main() {
	http.HandleFunc("/", helloWorld)
	httpmin.Run()

    // Or, httpmin.Setup().Route("/", helloWorld).Run()
}
```

#### Serve files

```go
package main

import (
	"embed"

	"github.com/Angus-Warman/httpmin"
)

//go:embed all:public
var folder embed.FS

func main() {
	httpmin.RunWithEmbedded(folder)

    // Equivalent to httpmin.Setup().ServeEmbedded(folder).Run()
}
```

#### More features

```go
package main

import (
	"net/http"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/middleware"
)

func helloWorld(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello World"))
}

func myCustomMiddleware() func(http.Handler) http.Handler {
	f := func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			// Do something here
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}

	return f
}

func main() {
	c := httpmin.Setup()

	c.OnPort("8081") // Port comes from: env variables, .env file, this function, "8080" (in that order)
	c.Route("/hello", helloWorld).Route("/world", helloWorld) // httpmin.Chassis call chaining
	c.ServeFolder("public") // Not embedded
	c.Use(middleware.Cors())
	c.Use(myCustomMiddleware())
	c.PublicIP() // Listen on "0.0.0.0"
	c.UseSelfSignedHTTPS()

	c.Run()
}
```
