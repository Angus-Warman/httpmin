package theme

import (
	"bytes"
	"embed"
	"fmt"
	"net/http"
	"strings"

	"github.com/Angus-Warman/httpmin/handler"
)

//go:embed all:parts
var embedded embed.FS

func Basic() http.Handler {
	return CustomTheme("reset.css", "layout.css")
}

func Minimal() http.Handler {
	return CustomTheme("reset.css", "layout.css", "minimal.css")
}

func Modern() http.Handler {
	return CustomTheme("reset.css", "layout.css", "modern.css")
}

func Paper() http.Handler {
	return CustomTheme("reset.css", "layout.css", "paper.css")
}

func Console() http.Handler {
	return CustomTheme("reset.css", "layout.css", "console.css")
}

// Any part that does not end in .css is directly appended to the theme file
func CustomTheme(parts ...string) http.Handler {
	if len(parts) == 0 {
		panic(fmt.Errorf("cannot build theme with no parts"))
	}

	data := mustBuildTheme(parts...)

	return handler.FromBytesAsType(data, "text/css")
}

func mustBuildTheme(parts ...string) []byte {
	data, err := buildTheme(parts...)

	if err != nil {
		panic(err)
	}

	return data
}

func buildTheme(parts ...string) ([]byte, error) {
	data := []byte{}
	buf := bytes.NewBuffer(data)

	for _, part := range parts {
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}

		if strings.HasSuffix(part, ".css") {
			partData, err := embedded.ReadFile("parts/" + part)

			if err != nil {
				return nil, err
			}

			buf.Write(partData)
		} else {
			buf.WriteString(part)
		}
	}

	return buf.Bytes(), nil
}
