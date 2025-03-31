package echo

import (
	"context"
	"log/slog"
	"sync"
)

// multiHandler routes logs to multiple underlying slog handlers.
// Kept unexported as it's an internal detail of the Init function.
type multiHandler struct {
	handlers []slog.Handler
	mu       sync.RWMutex
}

// newMultiHandler creates a handler that delegates to the provided handlers.
// Kept unexported.
func newMultiHandler(handlers ...slog.Handler) slog.Handler {
	// Defensive copy
	h := make([]slog.Handler, len(handlers))
	copy(h, handlers)
	return &multiHandler{
		handlers: h,
	}
}

// Enabled reports whether the handler handles records at the given level.
func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true // Enabled if any underlying handler is enabled
		}
	}
	return false
}

// Handle forwards the log record to all underlying handlers that are enabled for the record's level.
func (m *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var firstErr error
	// Create the record clone once outside the loop
	clonedRecord := record.Clone()
	for _, h := range m.handlers {
		// Crucial check: only Handle if the specific handler is Enabled for this level
		if h.Enabled(ctx, record.Level) {
			// Pass the cloned record to prevent potential issues if a handler modifies it
			if err := h.Handle(ctx, clonedRecord); err != nil && firstErr == nil {
				firstErr = err // Capture the first error encountered
			}
		}
	}
	return firstErr
}

// WithAttrs returns a new multiHandler whose underlying handlers are updated with the given attributes.
func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	m.mu.RLock()
	defer m.mu.RUnlock()
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	// Return a new multiHandler with the updated underlying handlers
	return &multiHandler{handlers: newHandlers}
}

// WithGroup returns a new multiHandler whose underlying handlers are updated with the given group name.
func (m *multiHandler) WithGroup(name string) slog.Handler {
	// Optimization: If the name is empty, slog handlers should return themselves.
	// If all handlers do this, we can return the original multiHandler.
	// However, creating a new one consistently is simpler and safer.
	m.mu.RLock()
	defer m.mu.RUnlock()
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	// Return a new multiHandler with the updated underlying handlers
	return &multiHandler{handlers: newHandlers}
}
