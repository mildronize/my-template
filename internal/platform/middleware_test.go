package platform

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func testLoggerAndBuf() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

// TestNewRouter_AssignsRequestIDWhenCallerSuppliesNone — GOAL.md
// Done-when 3's own building block for internal/transport/publicapi's
// engine: every request gets a request id, echoed back on the response,
// even when the caller didn't supply one.
func TestNewRouter_AssignsRequestIDWhenCallerSuppliesNone(t *testing.T) {
	logger, _ := testLoggerAndBuf()
	router := NewRouter(logger)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get(RequestIDHeader), "a request id must be assigned and echoed back even when the caller sent none")
}

// TestNewRouter_PreservesCallerSuppliedRequestID — a request id set by an
// upstream reverse proxy hop must survive unchanged, not be replaced by a
// freshly generated one.
func TestNewRouter_PreservesCallerSuppliedRequestID(t *testing.T) {
	logger, _ := testLoggerAndBuf()
	router := NewRouter(logger)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(RequestIDHeader, "caller-supplied-id-123")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, "caller-supplied-id-123", rec.Header().Get(RequestIDHeader))
}

// TestNewRouter_RecoversPanicAndReturns500 — a panic anywhere downstream
// (a domain handler, a middleware bug) must never crash the process or
// hang the response; Recovery converts it to a plain 500.
func TestNewRouter_RecoversPanicAndReturns500(t *testing.T) {
	logger, _ := testLoggerAndBuf()
	router := NewRouter(logger)
	router.GET("/panics", func(c *gin.Context) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	require.NotPanics(t, func() {
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panics", nil))
	})
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestNewRouter_LogsEveryRequestWithRequestIDMethodPathStatus — proves
// RequestLogging is actually wired into the engine NewRouter builds (not
// just unit-tested in isolation), and that it still logs the final status
// of a request Recovery had to intervene on — the reason RequestLogging
// is registered before Recovery, not after (middleware.go's own doc
// comment).
func TestNewRouter_LogsEveryRequestWithRequestIDMethodPathStatus(t *testing.T) {
	logger, buf := testLoggerAndBuf()
	router := NewRouter(logger)
	router.GET("/panics", func(c *gin.Context) { panic("boom") })

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(RequestIDHeader, "req-abc")
	router.ServeHTTP(httptest.NewRecorder(), req)

	logged := buf.String()
	assert.Contains(t, logged, "http request")
	assert.Contains(t, logged, "request_id=req-abc")
	assert.Contains(t, logged, "method=GET")
	assert.Contains(t, logged, "path=/healthz")
	assert.Contains(t, logged, "status=200")

	buf.Reset()
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/panics", nil))
	assert.Contains(t, buf.String(), "status=500", "the request log line for a recovered panic must still report its final status")
}
