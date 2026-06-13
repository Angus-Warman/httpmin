## httpmin

The bare minimum required to launch a sensible Go server.

Three goals:
- Minimal boilerplate
- Sensible defaults
- Easy configuration

Features:
- Environment variable handling
- Efficient embedded asset serving
- Graceful shutdown
- Request logging
- Recovers from panics

Optional:
- HTTPS (including self-signed)
- CORS
- JWT authentication
- SSE streaming

### Install

```bash
go get -u github.com/Angus-Warman/httpmin
```

### Get started

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
	httpmin.New().Route("/", helloWorld).Run()
}
```

If you have a `public` folder:

```go
package main

import (
	"embed"

	"github.com/Angus-Warman/httpmin"
)

//go:embed all:public
var publicFiles embed.FS

func main() {
	httpmin.New().ServeEmbedded(publicFiles).Run()
}
```
