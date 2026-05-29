package httpmin

import (
	"crypto/tls"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/Angus-Warman/httpmin/middleware"
)

type Chassis struct {
	mux                  *http.ServeMux
	protocol             string
	ip                   string
	port                 string
	logger               *log.Logger
	middlewares          []func(http.Handler) http.Handler
	certFile             string
	keyFile              string
	certFolder           string
	useDefaultMiddleware bool
}

// Plain option, will serve http://localhost:8080 unless env variables specify
func New() *Chassis {
	chassis := &Chassis{
		protocol:             "http",
		ip:                   "localhost",
		port:                 "8080",
		logger:               log.Default(),
		mux:                  http.NewServeMux(),
		useDefaultMiddleware: false,
	}

	return chassis
}

// Default option, reads .env file, logs incoming requests, handles panics, will serve http://localhost:8080 unless env variables specify
func Setup() *Chassis {
	c := New()
	c.EnvFile(".env")
	c.useDefaultMiddleware = true
	return c
}

// Read a .env formatted file, setting any environment variables that aren't already set
func (c *Chassis) EnvFile(path string) *Chassis {
	readEnvFile(path)
	return c
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

// If root contains exactly one top-level dir and nothing else, substitute it
func substituteTopLevelDir(root fs.FS) fs.FS {
	entries, err := fs.ReadDir(root, ".")

	if err != nil {
		return root
	}

	if len(entries) != 1 {
		return root
	}

	entry := entries[0]

	if !entry.IsDir() {
		return root
	}

	newRoot, err := fs.Sub(root, entry.Name())

	if err != nil {
		return root
	}

	return newRoot
}

// Serves files from embedded directory. Also maps requests from "/page" to "/page.html".
//
//	//go:embed all:public
//	var publicFiles embed.FS
//	c.ServeEmbedded(publicFiles)
func (c *Chassis) ServeEmbedded(folder fs.FS) *Chassis {

	// By default, embedded folder is expecting a path like "/public/index.html"
	// Moving down one level results in normal behaviour
	folder = substituteTopLevelDir(folder)

	c.mux.Handle("/", serveEmbeddedFiles(folder))
	return c
}

// Serves files from directory
//
// Identical to:
//
//	c.mux.Handle("/", http.FileServer(http.Dir(path)))
func (c *Chassis) ServeFolder(path string) *Chassis {
	c.mux.Handle("/", http.FileServer(http.Dir(path)))
	return c
}

// The port used comes from: env variables, .env file, this function, "8080" (in that order)
func (c *Chassis) OnPort(port string) *Chassis {
	c.port = port
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

// Folder will contain cert.pem and key.pem after generating
func (c *Chassis) UseSelfSignedHTTPSFromFolder(certFolder string) *Chassis {
	c.protocol = "https"
	c.certFolder = certFolder
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

func envOrDefault(key, defaultValue string) string {
	value, ok := os.LookupEnv(key)

	if ok {
		return value
	}

	return defaultValue
}

// Builds the underlying http.Server, sets up HTTPS as configured, and uses serverWithIntercept()
func (c *Chassis) Serve() error {
	if c.useDefaultMiddleware {
		c.addDefaultMiddleware()
	}

	handler := c.handlerWithMiddleware()

	port := envOrDefault("PORT", c.port)
	ip := envOrDefault("IP", c.ip)
	addr := fmt.Sprintf("%v:%v", ip, port)

	server := &http.Server{
		Addr:     addr,
		Handler:  handler,
		ErrorLog: c.logger,
	}

	if c.protocol == "https" {
		var cert tls.Certificate
		var err error

		if c.certFile != "" && c.keyFile != "" {
			cert, err = certificateFromFiles(c.certFile, c.keyFile)
		} else if c.certFolder != "" {
			cert, err = selfSignedFromFolder(c.certFolder)
		} else {
			cert, err = selfSignedCertificate()
		}

		if err != nil {
			return err
		}

		server.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	printAddresses(c.protocol, ip, port)

	return serveWithIntercept(server)
}

// Serves, printing any errors and exiting on a failure
func (c *Chassis) Run() {
	err := c.Serve()

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
