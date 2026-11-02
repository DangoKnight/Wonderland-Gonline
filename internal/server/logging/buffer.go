// Package logging owns bounded administrative log snapshots and packet diagnostics.
// It has no dependency on server sessions or gameplay state.
package logging

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Capacity bounds the number of records retained for the administration API.
const Capacity = 500
const maxValueBytes = 4096

// LogEntry is a captured record with bounded values and redacted secret fields.
type LogEntry struct {
	At      time.Time         `json:"at"`
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields"`
}

// Buffer owns concurrent log capture and the runtime logging level. Its zero
// value is ready to use at the info level; snapshots do not share mutable maps.
type Buffer struct {
	mu      sync.Mutex
	entries []LogEntry
	level   slog.LevelVar
}

func ValidLevel(level string) bool {
	switch NormalizeLevel(level) {
	case "debug", "info", "warn", "error":
		return true
	}
	return false
}
func (l *Buffer) Level() string { return strings.ToLower(l.level.Level().String()) }
func (l *Buffer) SetLevel(level string) {
	switch NormalizeLevel(level) {
	case "debug":
		l.level.Set(slog.LevelDebug)
	case "info":
		l.level.Set(slog.LevelInfo)
	case "warn":
		l.level.Set(slog.LevelWarn)
	case "error":
		l.level.Set(slog.LevelError)
	}
}
func (l *Buffer) Snapshot() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]LogEntry{}, l.entries...)
	for i := range out {
		fields := map[string]string{}
		for k, v := range out[i].Fields {
			fields[k] = v
		}
		out[i].Fields = fields
	}
	return out
}

type adminLogHandler struct {
	base   slog.Handler
	buffer *Buffer
	attrs  []slog.Attr
	group  string
}

func (h *adminLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.buffer.level.Level()
}
func (h *adminLogHandler) Handle(ctx context.Context, r slog.Record) error {
	fields := map[string]string{}
	var add func(slog.Attr, string)
	add = func(a slog.Attr, prefix string) {
		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindGroup {
			if a.Key != "" {
				prefix += a.Key + "."
			}
			for _, attr := range a.Value.Group() {
				add(attr, prefix)
			}
			return
		}
		if a.Equal(slog.Attr{}) {
			return
		}
		key := prefix + a.Key
		value := a.Value.String()
		if len(value) > maxValueBytes {
			value = value[:maxValueBytes]
		}
		if strings.Contains(strings.ToLower(key), "password") || strings.Contains(strings.ToLower(key), "token") {
			value = "[redacted]"
		}
		fields[key] = value
	}
	for _, a := range h.attrs {
		add(a, h.group)
	}
	r.Attrs(func(a slog.Attr) bool { add(a, h.group); return true })
	h.buffer.mu.Lock()
	if len(h.buffer.entries) == Capacity {
		copy(h.buffer.entries, h.buffer.entries[1:])
		h.buffer.entries = h.buffer.entries[:Capacity-1]
	}
	h.buffer.entries = append(h.buffer.entries, LogEntry{At: r.Time, Level: r.Level.String(), Message: r.Message, Fields: fields})
	h.buffer.mu.Unlock()
	return h.base.Handle(ctx, r)
}
func (h *adminLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copy := *h
	copy.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	copy.base = h.base.WithAttrs(attrs)
	return &copy
}
func (h *adminLogHandler) WithGroup(group string) slog.Handler {
	copy := *h
	copy.group += group + "."
	copy.base = h.base.WithGroup(group)
	return &copy
}

// NewHandler records bounded, redacted snapshots alongside the output handler.
func NewHandler(base slog.Handler, buffer *Buffer) slog.Handler {
	return &adminLogHandler{base: base, buffer: buffer}
}
