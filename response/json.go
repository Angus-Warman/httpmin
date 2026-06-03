package response

import (
	"encoding/json"
	"net/http"
)

func JSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	err := enc.Encode(data)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
