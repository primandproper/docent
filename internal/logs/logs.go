// Package logs provides the file-backed logger Docent runs with.
//
// primitives-go's own slog logger writes to stdout, which a CLI whose stdout
// is a machine-readable contract (`docent validate --json`) and a GUI app with
// no terminal can't use. This is the same logging.Logger interface over a
// slog handler that writes to ~/Library/Logs/docent instead.
package logs

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/primandproper/primitives-go/v2/observability/keys"
	"github.com/primandproper/primitives-go/v2/observability/logging"

	"go.opentelemetry.io/otel/trace"
)

const (
	fileName = "docent.log"
	// maxSize is the size past which the log is rotated at startup. One old
	// file is kept.
	maxSize = 5 << 20
)

type logger struct {
	_      struct{}
	logger *slog.Logger
}

var _ logging.Logger = (*logger)(nil)

// DefaultDir is where logs go on macOS: ~/Library/Logs/docent.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}

	return filepath.Join(home, "Library", "Logs", "docent"), nil
}

// Open creates dir if needed and returns a logger appending to docent.log in
// it, the slog.Logger underneath (for libraries that want one), and the file
// to close on exit.
func Open(dir string, level logging.Level) (log logging.Logger, system *slog.Logger, file io.Closer, err error) {
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return nil, nil, nil, fmt.Errorf("creating log directory: %w", err)
	}

	path := filepath.Join(dir, fileName)

	if info, statErr := os.Stat(path); statErr == nil && info.Size() > maxSize {
		if err = os.Rename(path, path+".1"); err != nil {
			return nil, nil, nil, fmt.Errorf("rotating log file: %w", err)
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("opening log file: %w", err)
	}

	l := New(f, level)

	return l, l.logger, f, nil
}

// New returns a logger writing JSON lines to w.
func New(w io.Writer, level logging.Level) *logger { //nolint:revive // the concrete type is only useful through its interface
	var slogLevel slog.Level

	switch level {
	case logging.DebugLevel:
		slogLevel = slog.LevelDebug
	case logging.WarnLevel:
		slogLevel = slog.LevelWarn
	case logging.ErrorLevel:
		slogLevel = slog.LevelError
	case logging.InfoLevel:
		slogLevel = slog.LevelInfo
	}

	return &logger{logger: slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slogLevel}))}
}

func (l *logger) Info(msg string)  { l.logger.Info(msg) }
func (l *logger) Debug(msg string) { l.logger.Debug(msg) }
func (l *logger) Warn(msg string)  { l.logger.Warn(msg) }

func (l *logger) Error(whatWasHappeningWhenErrorOccurred string, err error) {
	if err != nil {
		l.logger.Error(whatWasHappeningWhenErrorOccurred, slog.String("error", err.Error()))
		return
	}

	l.logger.Error(whatWasHappeningWhenErrorOccurred)
}

// SetRequestIDFunc is a no-op: Docent serves no HTTP requests.
func (l *logger) SetRequestIDFunc(logging.RequestIDFunc) {}

func (l *logger) Clone() logging.Logger {
	return &logger{logger: l.logger}
}

func (l *logger) WithName(name string) logging.Logger {
	return &logger{logger: l.logger.With(slog.String(logging.LoggerNameKey, name))}
}

func (l *logger) WithValue(key string, value any) logging.Logger {
	return &logger{logger: l.logger.With(slog.Any(key, value))}
}

func (l *logger) WithValues(values map[string]any) logging.Logger {
	attrs := make([]any, 0, len(values))
	for k, v := range values {
		attrs = append(attrs, slog.Any(k, v))
	}

	return &logger{logger: l.logger.With(attrs...)}
}

func (l *logger) WithError(err error) logging.Logger {
	if err == nil {
		return l
	}

	return &logger{logger: l.logger.With(slog.String("error", err.Error()))}
}

func (l *logger) WithSpan(span trace.Span) logging.Logger {
	spanCtx := span.SpanContext()
	if !spanCtx.IsValid() {
		return l
	}

	return &logger{logger: l.logger.With(
		slog.String(keys.SpanIDKey, spanCtx.SpanID().String()),
		slog.String(keys.TraceIDKey, spanCtx.TraceID().String()),
	)}
}

func (l *logger) WithRequest(req *http.Request) logging.Logger {
	if req == nil {
		return l
	}

	return &logger{logger: l.logger.With(slog.String("method", req.Method), slog.String("url", req.URL.String()))}
}

func (l *logger) WithResponse(res *http.Response) logging.Logger {
	if res == nil {
		return l
	}

	return &logger{logger: l.logger.With(slog.Int("status", res.StatusCode))}
}
