package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	UserIDKey    contextKey = "user_id"
)

// LogHandler wraps slog.Handler to inject contextual fields like request_id and user_id.
type LogHandler struct {
	slog.Handler
}

// Handle appends context values to record attributes.
func (h *LogHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
			r.AddAttrs(slog.String("request_id", reqID))
		}
		if userID, ok := ctx.Value(UserIDKey).(string); ok && userID != "" {
			r.AddAttrs(slog.String("user_id", userID))
		}
	}
	return h.Handler.Handle(ctx, r)
}

// PrettyHandler renders structured, color-coded, visually distinct logs for development consoles.
type PrettyHandler struct {
	opts  slog.HandlerOptions
	out   io.Writer
	mu    *sync.Mutex
	attrs []slog.Attr
}

func NewPrettyHandler(out io.Writer, opts *slog.HandlerOptions) *PrettyHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{Level: slog.LevelInfo}
	}
	return &PrettyHandler{
		opts: *opts,
		out:  out,
		mu:   &sync.Mutex{},
	}
}

func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	timeStr := r.Time.Format("15:04:05")

	var levelBadge string
	switch r.Level {
	case slog.LevelDebug:
		levelBadge = "\033[90mDEBUG\033[0m"
	case slog.LevelInfo:
		levelBadge = "\033[36mINFO \033[0m"
	case slog.LevelWarn:
		levelBadge = "\033[33;1mWARN \033[0m"
	case slog.LevelError:
		levelBadge = "\033[31;1mERROR\033[0m"
	default:
		levelBadge = fmt.Sprintf("%-5s", r.Level.String())
	}

	var method, path, errStr string
	status := 0
	var latency time.Duration
	var extraAttrs []string

	collectAttr := func(a slog.Attr) {
		switch a.Key {
		case "method":
			method = a.Value.String()
		case "path":
			path = a.Value.String()
		case "status":
			status = int(a.Value.Int64())
		case "latency":
			latency = a.Value.Duration()
		case "error":
			errStr = a.Value.String()
			extraAttrs = append(extraAttrs, fmt.Sprintf("\033[31merror=%s\033[0m", errStr))
		case "request_id", "user_agent", "ip", "bytes":
			// Suppress secondary fields to keep logs concise
		default:
			if a.Key != "" && a.Value.String() != "" {
				extraAttrs = append(extraAttrs, fmt.Sprintf("%s=%s", a.Key, a.Value.String()))
			}
		}
	}

	for _, a := range h.attrs {
		collectAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		collectAttr(a)
		return true
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\033[90m[%s]\033[0m %s ", timeStr, levelBadge))

	// Format HTTP request events cleanly
	if method != "" && path != "" {
		var methodTag string
		switch method {
		case "GET":
			methodTag = "\033[34mGET   \033[0m"
		case "POST":
			methodTag = "\033[32mPOST  \033[0m"
		case "PUT":
			methodTag = "\033[33mPUT   \033[0m"
		case "PATCH":
			methodTag = "\033[35mPATCH \033[0m"
		case "DELETE":
			methodTag = "\033[31mDELETE\033[0m"
		default:
			methodTag = fmt.Sprintf("%-6s", method)
		}

		var statusTag string
		statusText := http.StatusText(status)
		if status >= 500 {
			statusTag = fmt.Sprintf("\033[31;1m%d %s\033[0m", status, statusText)
		} else if status >= 400 {
			statusTag = fmt.Sprintf("\033[33;1m%d %s\033[0m", status, statusText)
		} else {
			statusTag = fmt.Sprintf("\033[32m%d %s\033[0m", status, statusText)
		}

		lat := latency.Round(time.Millisecond).String()
		if latency < time.Millisecond {
			lat = latency.Round(time.Microsecond).String()
		}

		sb.WriteString(fmt.Sprintf("🌐 %s %-45s -> %s \033[90m(%s)\033[0m", methodTag, path, statusTag, lat))
		if errStr != "" {
			sb.WriteString(fmt.Sprintf(" \033[31m[error: %s]\033[0m", errStr))
		}
	} else {
		// Domain event formatting
		msg := r.Message
		if strings.HasPrefix(msg, "[Memory Persistence]") || strings.HasPrefix(msg, "[Memory Extractor]") {
			sb.WriteString("\033[35m🧠 [MEMORY]\033[0m ")
			msg = strings.TrimPrefix(msg, "[Memory Persistence] ")
			msg = strings.TrimPrefix(msg, "[Memory Extractor] ")
		} else if strings.HasPrefix(msg, "[Live Talk]") || strings.HasPrefix(msg, "[Live Talk Tool Call]") {
			sb.WriteString("\033[33m🎙️ [LIVE]  \033[0m ")
			msg = strings.TrimPrefix(msg, "[Live Talk] ")
			msg = strings.TrimPrefix(msg, "[Live Talk Tool Call] ")
		} else if strings.HasPrefix(msg, "[RAG]") {
			sb.WriteString("\033[34m📚 [RAG]   \033[0m ")
			msg = strings.TrimPrefix(msg, "[RAG] ")
		} else if strings.HasPrefix(msg, "[Function Calling]") || strings.HasPrefix(msg, "[Vertex AI]") {
			sb.WriteString("\033[36m🤖 [AI]    \033[0m ")
			msg = strings.TrimPrefix(msg, "[Function Calling] ")
			msg = strings.TrimPrefix(msg, "[Vertex AI] ")
		}
		sb.WriteString(msg)
	}

	if len(extraAttrs) > 0 {
		sb.WriteString(fmt.Sprintf(" \033[90m(%s)\033[0m", strings.Join(extraAttrs, " ")))
	}
	sb.WriteString("\n")

	_, err := h.out.Write([]byte(sb.String()))
	return err
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &PrettyHandler{
		opts:  h.opts,
		out:   h.out,
		mu:    h.mu,
		attrs: append(h.attrs, attrs...),
	}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	return h
}

// New initializes a production (JSON) or development (Pretty Structured) slog.Logger.
func New(env string) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = NewPrettyHandler(os.Stdout, opts)
	}

	customHandler := &LogHandler{Handler: handler}
	return slog.New(customHandler)
}
