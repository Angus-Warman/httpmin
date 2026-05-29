package httpmin

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

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
		server.ErrorLog.Printf("signal %v, shutting down...\n", sig)
	}

	// Give in-flight requests time to complete
	timeout := 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := server.Shutdown(ctx)

	if err != nil {
		return err
	}

	server.ErrorLog.Println("goodbye")
	return nil
}

func listenAndServeProtocol(server *http.Server) error {
	if server.TLSConfig != nil {
		return server.ListenAndServeTLS("", "")
	} else {
		return server.ListenAndServe()
	}
}
