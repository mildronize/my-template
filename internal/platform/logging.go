package platform

import (
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

// NewLogger returns a slog.Logger backed by tint, for human-readable,
// colorized dev output. Every service built from this template logs
// through log/slog — never fmt.Println/log.Printf — so log lines stay
// structured and consistently formatted.
func NewLogger() *slog.Logger {
	handler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.Kitchen,
	})
	return slog.New(handler)
}
