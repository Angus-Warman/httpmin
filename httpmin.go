package httpmin

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Equivalent to httpmin.Setup().Run()
func Run() {
	Setup().Run()
}

// Equivalent to httpmin.Setup().ServeEmbedded(folder).Run()
func RunWithEmbedded(folder fs.FS) {
	Setup().ServeEmbedded(folder).Run()
}

func readEnvFile() {
	bytes, err := os.ReadFile(".env")

	if err != nil {
		return
	}

	for line := range strings.Lines(string(bytes)) {
		line = strings.TrimSpace(line)
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

func getAddresses(protocol, ip, port string) []string {
	addresses := []string{}

	if ip != "0.0.0.0" {
		addresses = append(addresses, fmt.Sprintf("%v://%v:%v", protocol, ip, port))
		return addresses
	}

	ifaces, err := net.Interfaces()

	if err != nil {
		addresses = append(addresses, fmt.Sprintf("%v://%v:%v", protocol, ip, port))
		return addresses
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
				continue
			}
			addresses = append(addresses, fmt.Sprintf("%v://%v:%v", protocol, ip, port))
		}
	}

	return addresses
}

func printAddresses(protocol, ip, port string) {
	addresses := getAddresses(protocol, ip, port)

	if len(addresses) == 1 {
		fmt.Printf("Listening on %v\n", addresses[0])
		return
	}

	fmt.Println("Listening on:")

	for _, address := range addresses {
		fmt.Printf("  %v\n", address)
	}
}

func listenAndServeProtocol(server *http.Server) error {
	if server.TLSConfig != nil {
		return server.ListenAndServeTLS("", "")
	} else {
		return server.ListenAndServe()
	}
}

func serveWithIntercept(server *http.Server) error {
	// Collect errors
	serverErr := make(chan error, 1)

	// Intercept OS signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Run the server
	go func() {
		err := listenAndServeProtocol(server)

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Block until server error or signal
	select {
	case err := <-serverErr:
		return err
	case sig := <-quit:
		fmt.Printf("signal %v, shutting down...\n", sig)
	}

	// Give in-flight requests time to complete
	timeout := 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := server.Shutdown(ctx)

	if err != nil {
		return err
	}

	fmt.Println("goodbye")
	return nil
}

func (c *Chassis) Serve() error {
	c.addDefaultMiddleWare()

	handler := c.handlerWithMiddleware()

	port := envOrDefault("PORT", c.defaultPort)
	ip := envOrDefault("IP", c.ip)
	addr := fmt.Sprintf("%v:%v", ip, port)

	printAddresses(c.protocol, ip, port)

	server := &http.Server{
		Addr:     addr,
		Handler:  handler,
		ErrorLog: c.logger,
	}

	if c.protocol == "https" {
		cert, err := createCertificate()

		if err != nil {
			return err
		}

		server.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

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
