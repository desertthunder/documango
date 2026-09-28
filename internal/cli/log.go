package cli

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

// logHandler is a slog.Handler that writes one styled line per record:
// time, level, message, then key=value attributes.
type logHandler struct {
	w     io.Writer
	level slog.Leveler
	mu    *sync.Mutex // shared by handlers derived with WithAttrs and WithGroup

	attrs  string // preformatted " key=value" pairs
	prefix string // group prefix for attribute keys, e.g. "build."
}

var levelStyles = map[slog.Level]lipgloss.Style{
	slog.LevelDebug: lipgloss.NewStyle().Foreground(lipgloss.Blue),
	slog.LevelInfo:  lipgloss.NewStyle().Foreground(lipgloss.Green),
	slog.LevelWarn:  lipgloss.NewStyle().Foreground(lipgloss.Yellow),
	slog.LevelError: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Red),
}

var levelLabels = map[slog.Level]string{
	slog.LevelDebug: "DEBU",
	slog.LevelInfo:  "INFO",
	slog.LevelWarn:  "WARN",
	slog.LevelError: "ERRO",
}

// Enabled reports whether records at level are written.
func (h *logHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle writes r as a single line.
func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(dimStyle.Render(r.Time.Format("15:04:05")))
	b.WriteByte(' ')
	b.WriteString(levelStyles[r.Level].Render(levelLabels[r.Level]))
	b.WriteByte(' ')
	b.WriteString(r.Message)
	attrs := h.attrs
	r.Attrs(func(a slog.Attr) bool {
		attrs += formatAttr(h.prefix, a)
		return true
	})
	b.WriteString(dimStyle.Render(attrs))
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

// WithAttrs returns a handler that adds attrs to every record.
func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	for _, a := range attrs {
		h2.attrs += formatAttr(h.prefix, a)
	}
	return &h2
}

// WithGroup returns a handler that qualifies later attribute keys with name.
func (h *logHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.prefix += name + "."
	return &h2
}

// formatAttr renders a as " key=value", quoting values with spaces.
func formatAttr(prefix string, a slog.Attr) string {
	if a.Equal(slog.Attr{}) {
		return ""
	}
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		var s string
		for _, ga := range v.Group() {
			s += formatAttr(prefix+a.Key+".", ga)
		}
		return s
	}
	val := v.String()
	if strings.ContainsAny(val, " \t\n\"=") {
		val = strconv.Quote(val)
	}
	return " " + prefix + a.Key + "=" + val
}
