package main

import (
	"net/http"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/handler"
)

var indexHTML = `<!DOCTYPE html>
<html>
<body>
	<h1>Builder Example</h1>
	<p>Try these curl commands:</p>
	<pre>
curl -X POST localhost:8080/api/users -H 'Content-Type: application/json' -d '{"name":"Alice","active":true}'
curl -X POST localhost:8080/api/users -H 'Content-Type: application/json' -d '{"name":"Bob","active":false}'
curl localhost:8080/api/users
curl localhost:8080/api/users/1
curl -X PUT localhost:8080/api/users/1 -H 'Content-Type: application/json' -d '{"id":"1","name":"Alice Updated","active":true}'
curl -X DELETE localhost:8080/api/users/2
	</pre>
</body>
</html>`

func main() {
	store := &UserStore{}

	c := httpmin.New().
		Route("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(indexHTML))
		})

	handler.Builder(c.Mux, "/api/users", store, nil)

	c.Run()
}
