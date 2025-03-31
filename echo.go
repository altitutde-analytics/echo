// Package echo provides a configurable logging module built on Go's standard log/slog.
// It supports structured logging to console (text/json) and file (json/text).
package echo

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// LogLevel aliases slog.Level for configuration clarity.
type LogLevel = slog.Level

// Define common log levels for easy configuration using exported constants.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Config holds the configuration for the echo logger.
type Config struct {
	// Level is the minimum level to log. E.g., LevelInfo, LevelDebug. Defaults to LevelInfo.
	Level LogLevel
	// ConsoleOutput enables logging to standard output (stdout).
	// Defaults to true if nil. Set to new(bool) // false to disable explicitly.
	ConsoleOutput *bool
	// FileOutput enables logging to a file. Defaults to false.
	FileOutput bool
	// FilePath specifies the path for the log file. Required if FileOutput is true.
	FilePath string
	// FileFormat specifies the format for file logs ("json" or "text"). Defaults to "json".
	FileFormat string
	// ConsoleFormat specifies the format for console logs ("json" or "text"). Defaults to "text".
	ConsoleFormat string
	// AddSource includes the source code position (file:line) in logs. Useful for debugging.
	AddSource bool
}

// FileCloser is the interface returned by Init, allowing the caller to close the log file.
type FileCloser interface {
	Close() error
}

// noopCloser is used when no file needs to be closed.
type noopCloser struct{}

func (nc noopCloser) Close() error { return nil }

// Init initializes the logging system based on the provided configuration
// and sets the default slog logger. It returns a FileCloser for the log file
// (if opened) and an error if initialization fails. The caller is responsible
// for calling the Close() method on the returned FileCloser, typically using defer.
func Init(cfg Config) (FileCloser, error) {
	var handlers []slog.Handler
	var logFile *os.File
	var err error

	// --- Set Defaults ---
	if cfg.Level == 0 { // Check if level is the zero value for slog.Level
		cfg.Level = LevelInfo
	}
	if cfg.ConsoleOutput == nil {
		defaultTrue := true
		cfg.ConsoleOutput = &defaultTrue // Default to true if not specified
	}
	if cfg.FileFormat == "" {
		cfg.FileFormat = "json"
	}
	if cfg.ConsoleFormat == "" {
		cfg.ConsoleFormat = "text"
	}

	// --- Handler Options ---
	handlerOpts := &slog.HandlerOptions{
		AddSource: cfg.AddSource,
		Level:     cfg.Level,
	}

	// --- Console Handler ---
	if *cfg.ConsoleOutput {
		var consoleHandler slog.Handler
		switch cfg.ConsoleFormat {
		case "json":
			consoleHandler = slog.NewJSONHandler(os.Stdout, handlerOpts)
		case "text":
			fallthrough // Default to text
		default:
			consoleHandler = slog.NewTextHandler(os.Stdout, handlerOpts)
		}
		handlers = append(handlers, consoleHandler)
		// Use a temporary logger for init messages before default is set
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Debug(
			"Console logging enabled",
			"level", cfg.Level.String(),
			"format", cfg.ConsoleFormat,
			"addSource", cfg.AddSource,
		)
	}

	// --- File Handler ---
	var closer FileCloser = noopCloser{} // Default to a no-op closer

	if cfg.FileOutput {
		if cfg.FilePath == "" {
			return closer, fmt.Errorf("echo.Init: FilePath is required when FileOutput is true")
		}

		// Ensure directory exists
		logDir := filepath.Dir(cfg.FilePath)
		if logDir != "." && logDir != "/" { // Avoid MkdirAll on current dir or root
			if err := os.MkdirAll(logDir, 0750); err != nil {
				return closer, fmt.Errorf("echo.Init: failed to create log directory '%s': %w", logDir, err)
			}
		}

		// Open file for appending, create if it doesn't exist
		logFile, err = os.OpenFile(cfg.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		if err != nil {
			return closer, fmt.Errorf("echo.Init: failed to open log file '%s': %w", cfg.FilePath, err)
		}
		closer = logFile // Assign the actual file to be closed

		var fileHandler slog.Handler
		var fileWriter io.Writer = logFile

		switch cfg.FileFormat {
		case "text":
			fileHandler = slog.NewTextHandler(fileWriter, handlerOpts)
		case "json":
			fallthrough // Default to json
		default:
			fileHandler = slog.NewJSONHandler(fileWriter, handlerOpts)
		}
		handlers = append(handlers, fileHandler)
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Debug(
			"File logging enabled",
			"path", cfg.FilePath,
			"level", cfg.Level.String(),
			"format", cfg.FileFormat,
			"addSource", cfg.AddSource,
		)
	}

	// --- Combine Handlers ---
	var finalHandler slog.Handler
	if len(handlers) == 0 {
		// If absolutely no output is configured, perhaps default to a handler that discards everything?
		// Or stick with minimal console info as before. Let's discard to be truly silent if configured.
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Warn(
			"echo.Init: No log outputs configured. Logs will be discarded.",
		)
		finalHandler = slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}) // Effectively disable
	} else if len(handlers) == 1 {
		finalHandler = handlers[0]
	} else {
		// Use the unexported multiHandler defined in multi_handler.go
		finalHandler = newMultiHandler(handlers...)
	}

	// --- Create and Set Logger ---
	logger := slog.New(finalHandler)
	slog.SetDefault(logger) // Set as the global default logger

	slog.Info("Echo logger initialized") // Log confirmation using the new setup

	return closer, nil
}

// ErrAttr is a helper to create a slog.Attr for an error under the key "error".
// It returns an empty attribute if the error is nil, preventing noise in logs.
func ErrAttr(err error) slog.Attr {
	if err == nil {
		// Return an explicitly empty Attr. Depending on slog version/behavior,
		// an empty key might be omitted.
		return slog.Attr{}
	}
	return slog.Any("error", err)
}
