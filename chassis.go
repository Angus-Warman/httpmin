package httpmin

import (
	"io/fs"
	"log"
	"net/http"

	"github.com/Angus-Warman/httpmin/middleware"
)

type Chassis struct {
	mux                  *http.ServeMux
	muxSet               bool
	protocol             string
	ip                   string
	defaultPort          string
	logger               *log.Logger
	middlewares          []func(http.Handler) http.Handler
	certFile             string
	keyFile              string
	useDefaultMiddleware bool
}

// Reads environment variables, uses log.Default(), http.DefaultServeMux, port 8080 and localhost
func Setup() *Chassis {
	readEnvFile()

	chassis := &Chassis{
		protocol:             "http",
		ip:                   "localhost",
		defaultPort:          "8080",
		logger:               log.Default(),
		mux:                  http.DefaultServeMux,
		useDefaultMiddleware: true,
	}

	return chassis
}

// Use this before adding other routes
func (c *Chassis) UseMux(mux *http.ServeMux) *Chassis {
	c.mux = mux
	return c
}

// Registers the handler function for the given pattern. Panics if the pattern has already been registered.
func (c *Chassis) Route(pattern string, handler func(w http.ResponseWriter, r *http.Request)) *Chassis {
	c.mux.HandleFunc(pattern, handler)
	return c
}

// Registers the handler for the given pattern. Panics if the pattern has already been registered.
func (c *Chassis) RouteHandler(pattern string, handler http.Handler) *Chassis {
	c.mux.Handle(pattern, handler)
	return c
}

func subFirstDir(root fs.FS) fs.FS {
	entries, err := fs.ReadDir(root, ".")

	if err != nil {
		return root
	}

	for _, e := range entries {
		if e.IsDir() {
			subFS, err := fs.Sub(root, e.Name())

			if err != nil {
				return root
			}

			return subFS
		}
	}

	return root
}

// Trims the top-level folder from paths. Maps requests from "/page" to "/page.html".
//
// Use the following:
//
//	//go:embed all:public
//	var publicFiles embed.FS
func (c *Chassis) ServeEmbedded(folder fs.FS) *Chassis {
	folder = subFirstDir(folder)

	c.mux.Handle("/", serveEmbeddedFiles(folder))
	return c
}

func (c *Chassis) ServeFolder(path string) *Chassis {
	c.mux.Handle("/", http.FileServer(http.Dir(path)))
	return c
}

// The port used comes from: env variables, .env file, this function, "8080" (in that order)
func (c *Chassis) OnPort(port string) *Chassis {
	c.defaultPort = port
	return c
}

// Listen on 0.0.0.0, alternatively use a "IP" env variable
func (c *Chassis) PublicIP() *Chassis {
	c.ip = "0.0.0.0"
	return c
}

// Alternatively, use a "IP" env variable
func (c *Chassis) OnIP(ip string) *Chassis {
	c.ip = ip
	return c
}

// Set as nil to disable logging
func (c *Chassis) WithLogger(logger *log.Logger) *Chassis {
	c.logger = logger
	return c
}

func (c *Chassis) UseHTTPS(certFile, keyFile string) *Chassis {
	c.protocol = "https"
	c.certFile = certFile
	c.keyFile = keyFile
	return c
}

func (c *Chassis) UseSelfSignedHTTPS() *Chassis {
	c.protocol = "https"
	return c
}

// Applied in registration order
func (c *Chassis) Use(middleware func(http.Handler) http.Handler) *Chassis {
	c.middlewares = append(c.middlewares, middleware)
	return c
}

// middleware.LogRequests and middleware.RecoverPanics
func (c *Chassis) UseDefaultMiddleware(use bool) *Chassis {
	c.useDefaultMiddleware = use
	return c
}

func (c *Chassis) addDefaultMiddleware() {
	c.Use(middleware.LogRequests(c.logger))
	c.Use(middleware.RecoverPanics(c.logger)) // Added last
}

func (c *Chassis) handlerWithMiddleware() http.Handler {
	var handler http.Handler = c.mux

	for i := len(c.middlewares) - 1; i >= 0; i-- {
		mw := c.middlewares[i]

		if mw == nil {
			continue
		}

		handler = mw(handler)
	}

	return handler
}
