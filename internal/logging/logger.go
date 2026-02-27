package logging

import (
	"log/slog"
	"os"
)

func New(service, version, commit, buildTime string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})
	return slog.New(handler).With(
		slog.String("service", service),
		slog.String("version", version),
		slog.String("commit", commit),
		slog.String("build_time", buildTime),
	)
}
