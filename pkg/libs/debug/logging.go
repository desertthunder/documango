package debug

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type conf struct {
	timeFmt string
	prefix  string
	level   slog.Level
	offset  int
}

type Handler struct {
	h       slog.Handler
	b       *bytes.Buffer
	m       *sync.Mutex
	helpers *sync.Map
	conf    *conf
}

type color string

const (
	Reset         color = "\033[0m"
	BgBlack       color = "\x1b[40m"
	BgRed         color = "\x1b[41m"
	BgGreen       color = "\x1b[42m"
	BgYellow      color = "\x1b[43m"
	BgBlue        color = "\x1b[44m"
	BgMagenta     color = "\x1b[45m"
	BgCyan        color = "\x1b[46m"
	BgWhite       color = "\x1b[47m"
	FgWhite       color = "\x1b[37m"
	FgBrightGrey  color = "\x1b[90m"
	FgBrightWhite color = "\x1b[97m"
)

func (c color) block(s string) string {
	return fmt.Sprintf("%v %v %v", c, s, Reset)
}

func (c color) text(s string) string {
	return fmt.Sprintf("%v%v%v", c, s, Reset)
}

func (h Handler) callerHelper(skip int) {
	var pcs [1]uintptr
	n := runtime.Callers(skip+2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	frame, _ := frames.Next()

	h.helpers.LoadOrStore(frame.Function, struct{}{})
}

func (h Handler) getFrames(skip int) *runtime.Frames {
	// From testing.T
	const maxStackLen = 50
	var pc [maxStackLen]uintptr
	n := runtime.Callers(skip+2, pc[:])

	return runtime.CallersFrames(pc[:n])
}

func (h Handler) loc(frames []runtime.Frame) (file string, line int, fn string) {
	if len(frames) == 0 {
		return "", 0, ""
	}
	f := frames[0]
	return f.File, f.Line, f.Function
}

func (h Handler) getModRoot() (roots string) {
	wd, _ := os.Getwd()
	dir := filepath.Clean(wd)
	for {
		p := filepath.Join(dir, "go.mod")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return dir
		} else if err != nil {
			d := filepath.Dir(dir)
			dir = d
		} else {
			break
		}
	}

	return ""
}

func (h Handler) getCaller() string {
	frames := h.getFrames(h.conf.offset + 2)
	var frame runtime.Frame

	for {
		f, more := frames.Next()
		_, helper := h.helpers.Load(f.Function)
		if !helper || !more {
			frame = f
			break
		}
	}

	fr := []runtime.Frame{frame}

	if len(fr) > 0 && fr[0].PC != 0 {
		file, ln, _ := h.loc(fr)
		if file != "" {
			r := (h.getModRoot())
			p := strings.TrimPrefix(file, r)
			return fmt.Sprintf("%v:%v", p, ln)
		} else {
			return ""
		}
	} else {
		return ""
	}
}

func (h Handler) formatTime(t time.Time) string {
	return FgBrightGrey.text(t.Format(time.Kitchen))
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.h.Enabled(ctx, level)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{h: h.h.WithAttrs(attrs), b: h.b, m: h.m}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{h: h.h.WithGroup(name), b: h.b, m: h.m}
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	level := r.Level.String()[:4]
	message := r.Message

	switch r.Level {
	case slog.LevelDebug:
		level = BgCyan.block(level)
		message = FgBrightWhite.text(message)
	case slog.LevelInfo:
		level = BgBlue.block(level)
		message = FgBrightWhite.text(message)
	case slog.LevelWarn:
		level = BgYellow.block(level)
		message = FgBrightWhite.text(message)
	case slog.LevelError:
		level = BgRed.block(level)
		message = FgBrightWhite.text(message)
	}

	caller := h.getCaller()

	fmt.Println(
		level,
		h.formatTime(r.Time),
		caller,
		message,
	)

	return nil
}

func newHandler(c *conf) *Handler {
	b := &bytes.Buffer{}
	opts := slog.HandlerOptions{
		AddSource: true,
		Level:     c.level,
	}

	return &Handler{
		b:       b,
		h:       slog.NewJSONHandler(b, &opts),
		m:       &sync.Mutex{},
		helpers: &sync.Map{},
		conf:    c,
	}
}

func DefaultConf() conf {
	return conf{
		level:   slog.LevelInfo,
		offset:  5,
		prefix:  "🚀",
		timeFmt: time.RFC822Z,
	}
}

func DebugConf() conf {
	return conf{
		level:   slog.LevelDebug,
		offset:  5,
		prefix:  "🐞",
		timeFmt: time.RFC822Z,
	}
}

func Config(l slog.Level, offset int, p string, layout string) conf {
	return conf{
		level:   l,
		offset:  5,
		prefix:  p,
		timeFmt: layout,
	}
}

func NewLogger(c conf) *slog.Logger {
	return slog.New(newHandler(&c))
}

func DefaultLogger() *slog.Logger {
	c := DefaultConf()
	return NewLogger(c)
}
