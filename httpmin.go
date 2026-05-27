package httpmin

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
)

func Run() {
	chassis := Setup()
	chassis.Run()
}

func RunWithEmbedded(folder fs.FS) {
	chassis := Setup()
	chassis.ServeEmbedded(folder)
	chassis.Run()
}

type Chassis struct {
	mux         *http.ServeMux
	muxSet      bool
	ip          string
	defaultPort string
	logger      *log.Logger
}

func Setup() *Chassis {
	chassis := &Chassis{
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

func (c *Chassis) Route(pattern string, handler func(w http.ResponseWriter, r *http.Request)) *Chassis {
	c.mux.HandleFunc(pattern, handler)
	return c
}

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
func (c *Chassis) UseLogger(logger *log.Logger) *Chassis {
	c.logger = logger
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

func (c *Chassis) fullHandler() http.Handler {
	handler := loggingMiddleware(c.mux, c.logger)
	return handler
}

func loggingMiddleware(next http.Handler, logger *log.Logger) http.Handler {
	if logger == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		logger.Printf("%v %v\n", r.Method, r.URL.Path)
	})
}

func (c *Chassis) Run() {
	readEnvFile()

	port := envOrDefault("PORT", c.defaultPort)
	ip := envOrDefault("IP", c.ip)

	addr := fmt.Sprintf("%v:%v", ip, port)

	handler := c.fullHandler()

	fmt.Printf("Serving http://%v:%v\n", ip, port)

	err := http.ListenAndServe(addr, handler)

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
