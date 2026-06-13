package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/parserequest"
)

var indexHTML = `
<!DOCTYPE html>
<html>

<body>
    <form method="POST" action="/submit">
		<label>
            Email
            <input name="email">
        </label>

        <label>
            Name (optional)
            <input name="name">
        </label>

		<label>
			Updates
			<input name="subscriptions" type="checkbox" value="updates">
		</label>

		<label>
			New offers
			<input name="subscriptions" type="checkbox" value="new offers">
		</label>

		<label>
			Events
			<input name="subscriptions" type="checkbox" value="events">
		</label>

        <button>Submit</button>
    </form>
</body>

</html>`

func indexPage(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(indexHTML))
}

type SignUp struct {
	Email         string
	Name          *string
	Subscriptions []string
}

func submit(w http.ResponseWriter, r *http.Request) {
	signUp, err := parserequest.As[SignUp](r)

	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), 400)
		return
	}

	data := fmt.Sprintf("%#v", signUp)
	log.Println(data)
	w.Write([]byte(data))
}

func main() {
	httpmin.New().Route("GET /", indexPage).Route("POST /submit", submit).Run()
}
