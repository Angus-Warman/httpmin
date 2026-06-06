package handler

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"strings"
)

type stringWriter struct{ b *strings.Builder }

func (sw *stringWriter) Write(p []byte) (int, error) { return sw.b.Write(p) }

func Favicon(hex int) http.Handler {
	rgba := color.RGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}

	img := drawDot(32, rgba)
	var buf strings.Builder
	err := png.Encode(&stringWriter{&buf}, img)

	if err != nil {
		panic(err)
	}

	encoded := buf.String()

	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.WriteHeader(http.StatusOK)
		_, _ = strings.NewReader(encoded).WriteTo(w)
	}

	return http.HandlerFunc(fn)
}

func drawDot(size int, col color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	cx := float64(size)/2 - 0.5
	cy := float64(size)/2 - 0.5
	r := float64(size)/2 - 1.5

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)

			var alpha float64
			switch {
			case dist <= r-0.5:
				alpha = 1.0
			case dist <= r+0.5:
				alpha = r + 0.5 - dist
			}

			if alpha > 0 {
				img.SetRGBA(x, y, color.RGBA{
					R: col.R,
					G: col.G,
					B: col.B,
					A: uint8(alpha * 255),
				})
			}
		}
	}
	return img
}
