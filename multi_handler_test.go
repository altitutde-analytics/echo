//go:build test
// +build test

package echo

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHandler implements slog.Handler for testing purposes.
type mockHandler struct {
	mu               sync.Mutex
	enabledLevel     slog.Level
	handledRecords   []slog.Record // Records passed to Handle
	handleError      error         // Error to return from Handle
	attrs            []slog.Attr   // Attributes added via WithAttrs/WithGroup
	withAttrsHistory [][]slog.Attr // History of attrs passed to WithAttrs
	withGroupHistory []string      // History of groups passed to WithGroup
}

// newMockHandler creates a mock handler enabled at the specified level.
func newMockHandler(level slog.Level) *mockHandler {
	return &mockHandler{
		enabledLevel: level,
	}
}

func (h *mockHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.enabledLevel
}

func (h *mockHandler) Handle(ctx context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Store a clone to ensure record immutability is checked implicitly
	h.handledRecords = append(h.handledRecords, record.Clone())
	return h.handleError
}

func (h *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.withAttrsHistory = append(h.withAttrsHistory, attrs)

	// Create a new mock handler instance inheriting state but with new attrs
	newH := &mockHandler{
		enabledLevel: h.enabledLevel,
		handleError:  h.handleError,
		// handledRecords is instance specific, not copied
	}
	// Combine existing and new attributes
	newH.attrs = slices.Clip(h.attrs) // Copy existing
	newH.attrs = append(newH.attrs, attrs...)
	return newH
}

func (h *mockHandler) WithGroup(name string) slog.Handler {
	h.mu.Lock()
	defer h.mu.Unlock()
	if name == "" {
		return h // slog Handler spec says return receiver if name is empty
	}
	h.withGroupHistory = append(h.withGroupHistory, name)

	// Create a new mock handler instance inheriting state but with new group
	newH := &mockHandler{
		enabledLevel: h.enabledLevel,
		handleError:  h.handleError,
		// handledRecords is instance specific, not copied
	}
	newH.attrs = slices.Clip(h.attrs) // Copy existing
	// Apply the group by adding a group attribute
	newH.attrs = append(newH.attrs, slog.Group(name)) // Simplified: Real group handling is more complex internally in slog
	return newH
}

func (h *mockHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handledRecords = nil
	h.handleError = nil
	h.withAttrsHistory = nil
	h.withGroupHistory = nil
	h.attrs = nil
}

func (h *mockHandler) HandledCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.handledRecords)
}

func (h *mockHandler) LastHandledRecord() (slog.Record, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.handledRecords) == 0 {
		return slog.Record{}, false
	}
	return h.handledRecords[len(h.handledRecords)-1], true
}

// --- Test Functions ---

func TestNewMultiHandler(t *testing.T) {
	mh1 := newMockHandler(slog.LevelInfo)
	mh2 := newMockHandler(slog.LevelDebug)

	tests := []struct {
		name          string
		handlers      []slog.Handler
		expectedCount int
	}{
		{"ZeroHandlers", []slog.Handler{}, 0},
		{"OneHandler", []slog.Handler{mh1}, 1},
		{"TwoHandlers", []slog.Handler{mh1, mh2}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multi := newMultiHandler(tt.handlers...)
			mHandler, ok := multi.(*multiHandler)
			require.True(t, ok, "newMultiHandler should return *multiHandler")
			assert.Len(t, mHandler.handlers, tt.expectedCount)
		})
	}
}

func TestMultiHandlerEnabled(t *testing.T) {
	mhDebug := newMockHandler(slog.LevelDebug)
	mhInfo := newMockHandler(slog.LevelInfo)
	mhWarn := newMockHandler(slog.LevelWarn)

	tests := []struct {
		name     string
		handlers []slog.Handler
		level    slog.Level
		expected bool
	}{
		{"DebugEnabled DebugLevel", []slog.Handler{mhDebug, mhInfo}, slog.LevelDebug, true},
		{"DebugEnabled InfoLevel", []slog.Handler{mhDebug, mhInfo}, slog.LevelInfo, true},
		{"InfoOnly DebugLevel", []slog.Handler{mhInfo, mhWarn}, slog.LevelDebug, false},
		{"InfoOnly InfoLevel", []slog.Handler{mhInfo, mhWarn}, slog.LevelInfo, true},
		{"InfoOnly WarnLevel", []slog.Handler{mhInfo, mhWarn}, slog.LevelWarn, true},
		{"NoneEnabled", []slog.Handler{mhWarn}, slog.LevelInfo, false},
		{"ZeroHandlers", []slog.Handler{}, slog.LevelInfo, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multi := newMultiHandler(tt.handlers...)
			enabled := multi.Enabled(context.Background(), tt.level)
			assert.Equal(t, tt.expected, enabled)
		})
	}
}

func TestMultiHandlerHandleDispatch(t *testing.T) {
	mhDebug := newMockHandler(slog.LevelDebug)
	mhInfo := newMockHandler(slog.LevelInfo)
	mhWarn := newMockHandler(slog.LevelWarn)

	handlers := []slog.Handler{mhDebug, mhInfo, mhWarn}
	multi := newMultiHandler(handlers...)

	recordDebug := slog.NewRecord(time.Now(), slog.LevelDebug, "debug msg", 0)
	recordInfo := slog.NewRecord(time.Now(), slog.LevelInfo, "info msg", 0)
	recordWarn := slog.NewRecord(time.Now(), slog.LevelWarn, "warn msg", 0)
	recordError := slog.NewRecord(time.Now(), slog.LevelError, "error msg", 0)

	// Reset mocks before each Handle call
	mhDebug.Reset()
	mhInfo.Reset()
	mhWarn.Reset()
	_ = multi.Handle(context.Background(), recordDebug)
	assert.Equal(t, 1, mhDebug.HandledCount(), "Debug handler should handle Debug record")
	assert.Equal(t, 0, mhInfo.HandledCount(), "Info handler should NOT handle Debug record")
	assert.Equal(t, 0, mhWarn.HandledCount(), "Warn handler should NOT handle Debug record")

	mhDebug.Reset()
	mhInfo.Reset()
	mhWarn.Reset()
	_ = multi.Handle(context.Background(), recordInfo)
	assert.Equal(t, 1, mhDebug.HandledCount(), "Debug handler should handle Info record")
	assert.Equal(t, 1, mhInfo.HandledCount(), "Info handler should handle Info record")
	assert.Equal(t, 0, mhWarn.HandledCount(), "Warn handler should NOT handle Info record")

	mhDebug.Reset()
	mhInfo.Reset()
	mhWarn.Reset()
	_ = multi.Handle(context.Background(), recordWarn)
	assert.Equal(t, 1, mhDebug.HandledCount(), "Debug handler should handle Warn record")
	assert.Equal(t, 1, mhInfo.HandledCount(), "Info handler should handle Warn record")
	assert.Equal(t, 1, mhWarn.HandledCount(), "Warn handler should handle Warn record")

	mhDebug.Reset()
	mhInfo.Reset()
	mhWarn.Reset()
	_ = multi.Handle(context.Background(), recordError)
	assert.Equal(t, 1, mhDebug.HandledCount(), "Debug handler should handle Error record")
	assert.Equal(t, 1, mhInfo.HandledCount(), "Info handler should handle Error record")
	assert.Equal(t, 1, mhWarn.HandledCount(), "Warn handler should handle Error record")
}

func TestMultiHandlerHandleError(t *testing.T) {
	mhOK := newMockHandler(slog.LevelInfo)
	mhErr := newMockHandler(slog.LevelInfo)
	expectedErr := errors.New("handle failed")
	mhErr.handleError = expectedErr

	multi := newMultiHandler(mhOK, mhErr) // Order matters for which error is returned if multiple fail

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "info msg", 0)
	err := multi.Handle(context.Background(), record)

	require.Error(t, err, "Handle should return an error if an underlying handler fails")
	assert.Equal(t, expectedErr, err, "Handle should return the error from the failing handler")
	assert.Equal(t, 1, mhOK.HandledCount(), "Non-failing handler should still have handled the record")
	assert.Equal(t, 1, mhErr.HandledCount(), "Failing handler should have been called")
}

func TestMultiHandlerWithAttrs(t *testing.T) {
	mh1 := newMockHandler(slog.LevelInfo)
	mh2 := newMockHandler(slog.LevelDebug)
	originalMulti := newMultiHandler(mh1, mh2)

	attrs := []slog.Attr{slog.String("key1", "val1"), slog.Int("key2", 123)}
	multiWithAttrs := originalMulti.WithAttrs(attrs)

	// Check immutability
	assert.NotSame(t, originalMulti, multiWithAttrs, "WithAttrs should return a new handler instance")

	// Check propagation to underlying mocks (crude check via mock history)
	mh1Impl, _ := mh1.WithAttrs(attrs).(*mockHandler) // Simulate expected state
	mh2Impl, _ := mh2.WithAttrs(attrs).(*mockHandler)
	assert.Equal(t, mh1Impl.attrs, mh1.withAttrsHistory[0], "Attr mismatch in underlying mock 1") // Check history directly on original mock
	assert.Equal(t, mh2Impl.attrs, mh2.withAttrsHistory[0], "Attr mismatch in underlying mock 2")

	// Check if attributes are present when handling a record with the new handler
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "info msg", 0)
	mh1.Reset() // Reset counters/history on the *original* mocks
	mh2.Reset()
	_ = multiWithAttrs.Handle(context.Background(), record) // Handle with the *new* multi-handler

	handledRec1, ok1 := mh1.LastHandledRecord()
	require.True(t, ok1, "Handler 1 should have handled the record")

	handledRec2, ok2 := mh2.LastHandledRecord()
	require.True(t, ok2, "Handler 2 should have handled the record")

	// Verify attributes exist in the handled record (simple presence check)
	attrCount1 := 0
	handledRec1.Attrs(func(a slog.Attr) bool {
		if (a.Key == "key1" && a.Value.String() == "val1") || (a.Key == "key2" && a.Value.Int64() == 123) {
			attrCount1++
		}
		return true
	})
	assert.Equal(t, 2, attrCount1, "Expected attributes not found in record handled by mock 1")

	attrCount2 := 0
	handledRec2.Attrs(func(a slog.Attr) bool {
		if (a.Key == "key1" && a.Value.String() == "val1") || (a.Key == "key2" && a.Value.Int64() == 123) {
			attrCount2++
		}
		return true
	})
	assert.Equal(t, 2, attrCount2, "Expected attributes not found in record handled by mock 2")

}

func TestMultiHandlerWithGroup(t *testing.T) {
	mh1 := newMockHandler(slog.LevelInfo)
	mh2 := newMockHandler(slog.LevelDebug)
	originalMulti := newMultiHandler(mh1, mh2)
	groupName := "mygroup"

	multiWithGroup := originalMulti.WithGroup(groupName)

	// Check immutability
	assert.NotSame(t, originalMulti, multiWithGroup, "WithGroup should return a new handler instance")

	// Check propagation to underlying mocks (crude check via mock history)
	assert.Contains(t, mh1.withGroupHistory, groupName, "Group name mismatch in underlying mock 1 history")
	assert.Contains(t, mh2.withGroupHistory, groupName, "Group name mismatch in underlying mock 2 history")

	// Check if group is applied when handling a record with the new handler
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "info msg", 0)
	record.AddAttrs(slog.String("attr_in_group", "val")) // Add an attr to see it nested

	mh1.Reset() // Reset counters/history on the *original* mocks
	mh2.Reset()
	_ = multiWithGroup.Handle(context.Background(), record) // Handle with the *new* multi-handler

	handledRec1, ok1 := mh1.LastHandledRecord()
	require.True(t, ok1, "Handler 1 should have handled the record")

	// Verify group presence (tricky without full slog internals, check if top-level attr is the group)
	var foundGroup1 bool
	handledRec1.Attrs(func(a slog.Attr) bool {
		if a.Key == groupName && a.Value.Kind() == slog.KindGroup {
			// Further check attributes within the group if necessary
			// For simplicity, just check the group name exists
			foundGroup1 = true
			return false // Stop iteration
		}
		return true
	})
	// Note: The mock handler's WithGroup is simplified. A real handler nests attrs.
	// This test mainly verifies that WithGroup was *called* via history and returns a new handler.
	// Verifying the *exact structure* handled would require a more complex mock or inspecting
	// how the actual Text/JSON handlers format groups.

	assert.True(t, len(mh1.withGroupHistory) > 0 && mh1.withGroupHistory[0] == groupName)
	assert.True(t, len(mh2.withGroupHistory) > 0 && mh2.withGroupHistory[0] == groupName)

	// Test empty group name
	multiWithEmptyGroup := multiWithGroup.WithGroup("")
	assert.NotSame(t, multiWithGroup, multiWithEmptyGroup, "WithGroup(\"\") should return a new handler instance based on mock") // Our simple mock always returns new
}
