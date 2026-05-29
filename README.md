## httpmin

The bare minimum required to launch a sensible Go server:

Environment variables, request logging, recovers from panics, HTTPS, configuration with minimal fuss.

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
	httpmin.Setup().Route("/", helloWorld).Run()
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
	httpmin.Setup().ServeEmbedded(publicFiles).Run()
}
```

### Features

- CORS
- JWT based authentication
- Self-signed HTTPS
- [Server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events) response handler
