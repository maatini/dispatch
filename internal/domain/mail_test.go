package domain

import "testing"

func TestSanitizeTraceContext_Allowlist(t *testing.T) {
	parent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	got := SanitizeTraceContext(map[string]string{
		"traceparent":       parent,
		"tracestate":        "vendor=1",
		"Authorization":     "Bearer secret",
		"client-request-id": "inject-me",
	})
	if got[traceKeyParent] != parent {
		t.Errorf("traceparent: want %s, got %v", parent, got[traceKeyParent])
	}
	if got[traceKeyState] != "vendor=1" {
		t.Errorf("tracestate: want vendor=1, got %v", got[traceKeyState])
	}
	if _, ok := got["Authorization"]; ok {
		t.Error("Authorization must be dropped")
	}
	if _, ok := got["client-request-id"]; ok {
		t.Error("non-W3C keys must be dropped")
	}
}

func TestSanitizeTraceContext_InvalidAndEmpty(t *testing.T) {
	if got := SanitizeTraceContext(nil); got != nil {
		t.Errorf("nil in: want nil, got %v", got)
	}
	if got := SanitizeTraceContext(map[string]string{}); got != nil {
		t.Errorf("empty in: want nil, got %v", got)
	}
	if got := SanitizeTraceContext(map[string]string{
		"traceparent": "not-a-traceparent",
		"TRACEPARENT": "00-ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ-00f067aa0ba902b7-01",
		"tracestate":  "bad\nvalue",
	}); got != nil {
		t.Errorf("invalid values: want nil, got %v", got)
	}
}

func TestSanitizeTraceContext_CaseInsensitiveKeys(t *testing.T) {
	parent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	got := SanitizeTraceContext(map[string]string{"TraceParent": parent})
	if got[traceKeyParent] != parent {
		t.Errorf("TraceParent: want canonical traceparent, got %v", got)
	}
}
