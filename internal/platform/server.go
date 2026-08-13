package platform

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// NewRouter builds the gin engine for this service's public API surface
// and registers routes that belong to the platform itself (currently just
// the health check). cmd/server registers internal/transport/publicapi's
// own routes on this same engine — platform never imports publicapi or
// any domain module (ARCHITECTURE.md rule 5).
//
// The three middlewares (RequestID, RequestLogging, Recovery —
// middleware.go) are cross-cutting gin infrastructure every engine this
// service builds needs, registered here in the order that lets each one
// see what it depends on: RequestID first (so the other two can tag their
// output with it), RequestLogging second, Recovery last — mirroring
// gin.Default()'s own Logger-then-Recovery order so a recovered panic's
// final status still reaches the request log line (middleware.go's own
// doc comments explain why). gin.Recovery() is deliberately not used
// here — Recovery replaces it so panics are logged through slog, not
// gin's own unstructured default logger.
func NewRouter(logger *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(RequestID(), RequestLogging(logger), Recovery(logger))

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return router
}

// RunServer starts an HTTP server on addr serving handler, and shuts it
// down gracefully when ctx is canceled.
func RunServer(ctx context.Context, logger *slog.Logger, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-serveErr
	case err := <-serveErr:
		return err
	}
}
