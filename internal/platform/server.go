package platform

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// NewRouter builds the gin engine and registers routes that belong to the
// platform itself (currently just the health check). Domain modules
// (internal/todo, internal/identity) register their own routes on this
// engine from their own handler.go files — platform never imports them.
func NewRouter(logger *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())

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
