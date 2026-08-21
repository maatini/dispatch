package loggy

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// MaskEmail masks an email address for safe logging: "user@domain.com" → "u***@domain.com".
func MaskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]
	if len(local) == 1 {
		return local + "***" + domain
	}
	return string(local[0]) + "***" + domain
}

// LogCategory is the semantic event type embedded in every log entry as "type".
type LogCategory string

const (
	CategoryCritical              LogCategory = "CRITICAL"
	CategoryBusinessLogic         LogCategory = "BUSINESS_LOGIC"
	CategoryBusinessRuleViolation LogCategory = "BUSINESS_RULE_VIOLATION"
	CategoryAPIRequest            LogCategory = "API_REQUEST"
	CategoryAPIExternalFailure    LogCategory = "API_EXTERNAL_FAILURE"
	CategoryAPIClientError        LogCategory = "API_CLIENT_ERROR"
	CategoryInfo                  LogCategory = "INFO"
	CategoryDefault               LogCategory = "DEFAULT"
)

// Kv creates a structured log field.
func Kv(key string, value any) slog.Attr {
	return slog.Any(key, value)
}

// Loggy wraps slog and emits structured JSON logs compatible with Loggy Core 1.3.0 semantics.
type Loggy struct {
	className string
	logger    *slog.Logger
	extra     []slog.Attr
	apiStart  sync.Map
}

// GetLogger returns a Loggy instance that logs as JSON to stdout.
func GetLogger(className string) *Loggy {
	return &Loggy{
		className: className,
		logger:    slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
}

// With returns a new Loggy with additional fields attached to every log entry.
func (l *Loggy) With(attrs ...slog.Attr) *Loggy {
	if l == nil {
		return GetLogger("")
	}
	extra := make([]slog.Attr, len(l.extra)+len(attrs))
	copy(extra, l.extra)
	copy(extra[len(l.extra):], attrs)
	return &Loggy{className: l.className, logger: l.logger, extra: extra}
}

type traceKey struct{}

// Trace is request-scoped correlation stored on context.
type Trace struct {
	TraceID string
	Fields  map[string]string
}

// Context returns ctx with TraceID and optional W3C fields for logs and Graph headers.
func Context(ctx context.Context, traceID string, fields map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	t := Trace{TraceID: traceID}
	if len(fields) > 0 {
		t.Fields = make(map[string]string, len(fields))
		for k, v := range fields {
			t.Fields[k] = v
		}
	}
	return context.WithValue(ctx, traceKey{}, t)
}

// TraceFrom returns the Trace stored by Context, if any.
func TraceFrom(ctx context.Context) (Trace, bool) {
	if ctx == nil {
		return Trace{}, false
	}
	t, ok := ctx.Value(traceKey{}).(Trace)
	return t, ok
}

// WithContext returns a logger with traceId and W3C fields from ctx attached.
func (l *Loggy) WithContext(ctx context.Context) *Loggy {
	t, ok := TraceFrom(ctx)
	if !ok {
		return l
	}
	attrs := make([]slog.Attr, 0, 1+len(t.Fields))
	if t.TraceID != "" {
		attrs = append(attrs, Kv("traceId", t.TraceID))
	}
	for k, v := range t.Fields {
		attrs = append(attrs, Kv(k, v))
	}
	if len(attrs) == 0 {
		return l
	}
	return l.With(attrs...)
}

func appendTraceAttrs(ctx context.Context, attrs []slog.Attr) []slog.Attr {
	t, ok := TraceFrom(ctx)
	if !ok {
		return attrs
	}
	if t.TraceID != "" && !attrHasKey(attrs, "traceId") {
		attrs = append(attrs, slog.String("traceId", t.TraceID))
	}
	for k, v := range t.Fields {
		if k != "" && !attrHasKey(attrs, k) {
			attrs = append(attrs, slog.String(k, v))
		}
	}
	return attrs
}

func attrHasKey(attrs []slog.Attr, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}

func (l *Loggy) emit(ctx context.Context, level slog.Level, category LogCategory, msg string, err error, fields ...slog.Attr) {
	if l == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	attrs := make([]slog.Attr, 0, 4+len(l.extra)+len(fields)+1)
	attrs = append(attrs, slog.String("logger", l.className))
	if category != "" {
		attrs = append(attrs, slog.String("type", string(category)))
	}
	attrs = append(attrs, l.extra...)
	attrs = append(attrs, fields...)
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	attrs = appendTraceAttrs(ctx, attrs)
	l.logger.LogAttrs(ctx, level, msg, attrs...)
}

// --- Standard log methods ---

func (l *Loggy) Info(msg string, fields ...slog.Attr) {
	l.emit(context.Background(), slog.LevelInfo, CategoryInfo, msg, nil, fields...)
}

func (l *Loggy) Warn(msg string, fields ...slog.Attr) {
	l.emit(context.Background(), slog.LevelWarn, CategoryDefault, msg, nil, fields...)
}

func (l *Loggy) Error(msg string, err error, fields ...slog.Attr) {
	l.emit(context.Background(), slog.LevelError, CategoryDefault, msg, err, fields...)
}

// --- Category-variant log methods ---

func (l *Loggy) Infoc(ctx context.Context, category LogCategory, msg string, fields ...slog.Attr) {
	l.emit(ctx, slog.LevelInfo, category, msg, nil, fields...)
}

func (l *Loggy) Warnc(ctx context.Context, category LogCategory, msg string, fields ...slog.Attr) {
	l.emit(ctx, slog.LevelWarn, category, msg, nil, fields...)
}

func (l *Loggy) Errorc(ctx context.Context, category LogCategory, msg string, err error, fields ...slog.Attr) {
	l.emit(ctx, slog.LevelError, category, msg, err, fields...)
}

// --- Semantic event methods ---

// Critical logs a system-threatening error at ERROR level.
func (l *Loggy) Critical(msg string, err error, fields ...slog.Attr) {
	l.emit(context.Background(), slog.LevelError, CategoryCritical, msg, err, fields...)
}

// --- API tracking methods ---

// RecordApiStart stores the start time for a named API call.
func (l *Loggy) RecordApiStart(apiName string) {
	if l == nil {
		return
	}
	l.apiStart.Store(apiName, time.Now())
}

// ExternalApiSuccess logs a successful external API call at INFO level,
// computing latency from the prior RecordApiStart.
func (l *Loggy) ExternalApiSuccess(ctx context.Context, apiName string, httpStatus int) {
	if l == nil {
		return
	}
	var durationMs int64
	if v, ok := l.apiStart.LoadAndDelete(apiName); ok {
		durationMs = time.Since(v.(time.Time)).Milliseconds()
	}
	l.emit(ctx, slog.LevelInfo, CategoryAPIRequest, apiName+" success", nil,
		slog.Int("httpStatus", httpStatus),
		slog.Int64("durationMs", durationMs),
	)
}

// ExternalApiFailure logs a failed external API call (5xx / network) at ERROR level.
func (l *Loggy) ExternalApiFailure(ctx context.Context, apiName string, httpStatus int, err error) {
	if l == nil {
		return
	}
	var durationMs int64
	if v, ok := l.apiStart.LoadAndDelete(apiName); ok {
		durationMs = time.Since(v.(time.Time)).Milliseconds()
	}
	l.emit(ctx, slog.LevelError, CategoryAPIExternalFailure, apiName+" failure", err,
		slog.Int("httpStatus", httpStatus),
		slog.Int64("durationMs", durationMs),
	)
}

// ApiClientError logs a 4xx client error against an external API at WARN level.
func (l *Loggy) ApiClientError(ctx context.Context, apiName string, httpStatus int, reason string) {
	if l == nil {
		return
	}
	var durationMs int64
	if v, ok := l.apiStart.LoadAndDelete(apiName); ok {
		durationMs = time.Since(v.(time.Time)).Milliseconds()
	}
	l.emit(ctx, slog.LevelWarn, CategoryAPIClientError, apiName+" client error", nil,
		slog.Int("httpStatus", httpStatus),
		slog.Int64("durationMs", durationMs),
		slog.String("reason", reason),
	)
}
