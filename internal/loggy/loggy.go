package loggy

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"
)

// LogCategory is the semantic event type embedded in every log entry as "type".
type LogCategory string

const (
	CategoryCritical              LogCategory = "CRITICAL"
	CategoryBusinessLogic         LogCategory = "BUSINESS_LOGIC"
	CategoryBusinessRuleViolation LogCategory = "BUSINESS_RULE_VIOLATION"
	CategoryMissingData           LogCategory = "MISSING_DATA"
	CategoryValidation            LogCategory = "VALIDATION"
	CategoryAPIRequest            LogCategory = "API_REQUEST"
	CategoryAPIExternalFailure    LogCategory = "API_EXTERNAL_FAILURE"
	CategoryAPIClientError        LogCategory = "API_CLIENT_ERROR"
	CategoryUncaughtException     LogCategory = "UNCAUGHT_EXCEPTION"
	CategorySecurity              LogCategory = "SECURITY"
	CategoryPerformance           LogCategory = "PERFORMANCE"
	CategoryInfo                  LogCategory = "INFO"
	CategoryUnstructured          LogCategory = "UNSTRUCTURED"
	CategoryDefault               LogCategory = "DEFAULT"
)

// Kv creates a structured log field.
func Kv(key string, value any) slog.Attr {
	return slog.Any(key, value)
}

// Alert creates an alert flag field.
func Alert(v bool) slog.Attr {
	return slog.Bool("alert", v)
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

func (l *Loggy) emit(ctx context.Context, level slog.Level, category LogCategory, msg string, err error, fields ...slog.Attr) {
	if l == nil {
		return
	}
	attrs := make([]slog.Attr, 0, 2+len(l.extra)+len(fields)+1)
	attrs = append(attrs, slog.String("logger", l.className))
	if category != "" {
		attrs = append(attrs, slog.String("type", string(category)))
	}
	attrs = append(attrs, l.extra...)
	attrs = append(attrs, fields...)
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
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

func (l *Loggy) Debug(msg string, fields ...slog.Attr) {
	l.emit(context.Background(), slog.LevelDebug, CategoryDefault, msg, nil, fields...)
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

func (l *Loggy) Debugc(ctx context.Context, category LogCategory, msg string, fields ...slog.Attr) {
	l.emit(ctx, slog.LevelDebug, category, msg, nil, fields...)
}

// --- Semantic event methods ---

// BusinessRuleViolation logs a business rule violation at WARN level.
func (l *Loggy) BusinessRuleViolation(ruleID, msg, ctx string) {
	l.emit(context.Background(), slog.LevelWarn, CategoryBusinessRuleViolation, msg, nil,
		slog.String("ruleId", ruleID),
		slog.String("context", ctx),
	)
}

// ValidationFailed logs a validation failure at WARN level.
func (l *Loggy) ValidationFailed(field, reason string, value ...string) {
	attrs := []slog.Attr{slog.String("field", field), slog.String("reason", reason)}
	if len(value) > 0 {
		attrs = append(attrs, slog.String("value", value[0]))
	}
	l.emit(context.Background(), slog.LevelWarn, CategoryValidation, "validation failed", nil, attrs...)
}

// MissingData logs missing expected data at WARN level.
func (l *Loggy) MissingData(field, ctx string) {
	l.emit(context.Background(), slog.LevelWarn, CategoryMissingData, "missing data", nil,
		slog.String("field", field),
		slog.String("context", ctx),
	)
}

// Critical logs a system-threatening error at ERROR level.
func (l *Loggy) Critical(msg string, err error, fields ...slog.Attr) {
	l.emit(context.Background(), slog.LevelError, CategoryCritical, msg, err, fields...)
}

// UncaughtException logs an exception caught at a system boundary at ERROR level.
func (l *Loggy) UncaughtException(err error, ctx string) {
	l.emit(context.Background(), slog.LevelError, CategoryUncaughtException, "uncaught exception", err,
		slog.String("context", ctx),
	)
}

// ServiceAccountExpired logs an expired or invalid service credential at ERROR level.
func (l *Loggy) ServiceAccountExpired(accountID, ctx string) {
	l.emit(context.Background(), slog.LevelError, CategorySecurity, "service account expired", nil,
		slog.String("accountId", accountID),
		slog.String("context", ctx),
	)
}

// UnstructuredLog logs a plain text message at INFO level.
func (l *Loggy) UnstructuredLog(msg string) {
	l.emit(context.Background(), slog.LevelInfo, CategoryUnstructured, msg, nil)
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
func (l *Loggy) ExternalApiSuccess(apiName string, httpStatus int) {
	var durationMs int64
	if v, ok := l.apiStart.LoadAndDelete(apiName); ok {
		durationMs = time.Since(v.(time.Time)).Milliseconds()
	}
	l.emit(context.Background(), slog.LevelInfo, CategoryAPIRequest, apiName+" success", nil,
		slog.Int("httpStatus", httpStatus),
		slog.Int64("durationMs", durationMs),
	)
}

// ExternalApiFailure logs a failed external API call (5xx / network) at ERROR level.
func (l *Loggy) ExternalApiFailure(apiName string, httpStatus int, err error) {
	var durationMs int64
	if v, ok := l.apiStart.LoadAndDelete(apiName); ok {
		durationMs = time.Since(v.(time.Time)).Milliseconds()
	}
	l.emit(context.Background(), slog.LevelError, CategoryAPIExternalFailure, apiName+" failure", err,
		slog.Int("httpStatus", httpStatus),
		slog.Int64("durationMs", durationMs),
	)
}

// ApiClientError logs a 4xx client error against an external API at WARN level.
func (l *Loggy) ApiClientError(apiName string, httpStatus int, reason string) {
	var durationMs int64
	if v, ok := l.apiStart.LoadAndDelete(apiName); ok {
		durationMs = time.Since(v.(time.Time)).Milliseconds()
	}
	l.emit(context.Background(), slog.LevelWarn, CategoryAPIClientError, apiName+" client error", nil,
		slog.Int("httpStatus", httpStatus),
		slog.Int64("durationMs", durationMs),
		slog.String("reason", reason),
	)
}
