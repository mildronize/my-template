package platform

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader is both the inbound header this service trusts a caller
// to have already set (a request arriving through a reverse proxy that
// already assigns one) and the outbound header every response carries —
// so a request id is stable across a proxy hop instead of being replaced.
const RequestIDHeader = "X-Request-ID"

const requestIDContextKey = "platform.request_id"

// RequestID is cross-cutting gin infrastructure (ARCHITECTURE.md: "Cross-
// cutting transport concerns live in platform/, not a shared transport
// package") — every engine this service builds (internal/transport/
// publicapi today, internal/transport/bff once task-4 adds it) registers
// it first, so RequestLogging and Recovery below can both tag their own
// output with the same id. It assigns a fresh UUID unless the request
// already carries one via RequestIDHeader, sets it on the gin context and
// on the response header, and never overwrites a caller-supplied id.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(requestIDContextKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// RequestIDFromContext returns the request id RequestID set for this
// request, or "" if RequestID isn't registered ahead of whatever called
// this.
func RequestIDFromContext(c *gin.Context) string {
	v, ok := c.Get(requestIDContextKey)
	if !ok {
		return ""
	}
	id, _ := v.(string)
	return id
}

// RequestLogging logs one structured line per request via slog (never
// gin's own default logger, which writes unstructured text straight to
// stdout, bypassing this service's log/slog convention — internal/
// platform/logging.go's own doc comment) — method, path, status, and
// latency, tagged with RequestID's id so every line for one request can
// be correlated. Registered before Recovery (mirrors gin.Default()'s own
// Logger-then-Recovery order) so it still gets to log the final 500 a
// recovered panic produces, not just requests that returned normally.
func RequestLogging(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		c.Next()

		logger.Info("http request",
			"request_id", RequestIDFromContext(c),
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency", time.Since(start),
		)
	}
}

// Recovery is panic recovery for a gin engine — recovers a panic in any
// downstream handler, logs it via slog (tagged with the request id, so it
// can be correlated with RequestLogging's own line for the same request),
// and responds 500, instead of gin.Recovery()'s default behavior of
// writing directly to os.Stderr in gin's own format. Registered after
// RequestLogging (see that function's doc comment) so a recovered panic's
// final status still reaches RequestLogging's log line.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					"request_id", RequestIDFromContext(c),
					"panic", r,
				)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
