package logwriter

import (
	"io"
	"strings"

	"github.com/rs/zerolog"
)

// Create a writer that logs each line to zerolog
type zerologWriter struct {
	logger zerolog.Logger
	level  zerolog.Level
}

func NewZerologWriter(logger zerolog.Logger, level zerolog.Level) io.Writer {
	return &zerologWriter{
		level:  level,
		logger: logger,
	}
}

func (w *zerologWriter) Write(p []byte) (n int, err error) {
	lines := strings.Split(strings.TrimSuffix(string(p), "\n"), "\n")
	for _, line := range lines {
		if line != "" {
			w.logger.WithLevel(w.level).Msg(line)
		}
	}
	return len(p), nil
}
