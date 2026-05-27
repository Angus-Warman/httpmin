package httpmin

import (
	"crypto/tls"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
)

// Equivalent to httpmin.Setup().Run()
func Run() {
	Setup().Run()
}

// Equivalent to httpmin.Setup().ServeEmbedded(folder).Run()
func RunWithEmbedded(folder fs.FS) {
	Setup().ServeEmbedded(folder).Run()
}

type Chassis struct {
	mux         *http.ServeMux
	muxSet      bool
	protocol    string
	ip          string
	defaultPort string
	logger      *log.Logger
	middlewares []func(http.Handler) http.Handler
}

// Uses log.Default(), http.DefaultServeMux, port 8080 and localhost
func Setup() *Chassis {
	chassis := &Chassis{
		protocol:    "http",
		ip:          "localhost",
		defaultPort: "8080",
		logger:      log.Default(),
		mux:         http.DefaultServeMux,
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

// Trims the top-level folder from paths. Use the following:
//
// //go:embed all:public
//
// var publicFiles embed.FS
func (c *Chassis) ServeEmbedded(folder fs.FS) *Chassis {
	folder = subFirstDir(folder)

	c.mux.Handle("/", http.FileServerFS(folder))
	return c
}

func (c *Chassis) ServeFolder(path string) *Chassis {
	c.mux.Handle("/", http.FileServer(http.Dir(path)))
	return c
}

// Alternatively, use a "PORT" env variable
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

func (c *Chassis) UseHTTPS() *Chassis {
	c.protocol = "https"
	return c
}

func readEnvFile() {
	bytes, err := os.ReadFile(".env")

	if err != nil {
		return
	}

	for line := range strings.Lines(string(bytes)) {
		key, value, lineOk := strings.Cut(line, "=")

		if !lineOk {
			continue
		}

		_, keyExists := os.LookupEnv(key)

		if keyExists {
			continue
		}

		os.Setenv(key, value)
	}
}

func envOrDefault(key, defaultValue string) string {
	value, ok := os.LookupEnv(key)

	if ok {
		return value
	}

	return defaultValue
}

// Applied in registration order
func (c *Chassis) Use(middleware func(http.Handler) http.Handler) *Chassis {
	c.middlewares = append(c.middlewares, middleware)
	return c
}

func requestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	f := func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			logger.Printf("%v %v\n", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}

	return f
}

func (c *Chassis) handlerWithMiddleware() http.Handler {
	var handler http.Handler = c.mux

	if c.logger != nil {
		c.Use(requestLogger(c.logger))
	}

	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i](handler)
	}

	return handler
}

func (c *Chassis) Serve() error {
	readEnvFile()

	handler := c.handlerWithMiddleware()

	port := envOrDefault("PORT", c.defaultPort)
	ip := envOrDefault("IP", c.ip)
	addr := fmt.Sprintf("%v:%v", ip, port)

	fmt.Printf("Serving %v://%v:%v\n", c.protocol, ip, port)

	if c.protocol == "http" {
		return http.ListenAndServe(addr, handler)
	}

	cert, err := createCertificate()

	if err != nil {
		return err
	}

	server := http.Server{
		Addr:    addr,
		Handler: handler,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	return server.ListenAndServeTLS("", "")
}

// Serves, printing any errors and exiting on a failure
func (c *Chassis) Run() {
	err := c.Serve()

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
